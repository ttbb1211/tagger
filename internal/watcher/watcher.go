package watcher

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ericwyn/tagger/internal/domain"
	"github.com/fsnotify/fsnotify"
)

type Callback func(context.Context, []string) error

// ErrBusy tells the watcher that a callback's work is temporarily blocked by
// another durable task. The affected directory batch is retained and retried
// after a short delay instead of being silently dropped.
var ErrBusy = errors.New("watcher callback busy")

const (
	// registerTimeout 是「注册曲库目录」这一步的最长等待时间。超时不再阻断启动：
	// 目录注册留在后台继续，watcher 先按降级处理（UI、扫描、写入都不受影响）。
	registerTimeout = 20 * time.Second

	// eventBufferSize 是内部事件缓冲容量。fsnotify 自己的 Events 只有 50 格，
	// 而且它一旦写不进去就会阻塞内部读取协程、进而卡死每一个 Add；
	// 用一个大缓冲把「搬运事件」和「处理事件」解耦，处理慢一点也不会反压。
	eventBufferSize = 8192
)

type Manager struct {
	wait     time.Duration
	callback Callback

	mu       sync.Mutex
	cancel   context.CancelFunc
	fsw      *fsnotify.Watcher
	root     string
	watchErr chan error

	// registering 为 true 表示目录注册尚未结束。此时 loop 只消费事件、
	// 不派发回调，避免启动期的事件洪峰触发无意义的对账扫描。
	registering atomic.Bool
	// overflowed 记录「内部缓冲满、有事件被丢弃」。下一轮派发时补一次整库对账，
	// 免得丢掉的变化要等到下一次定时对账才被发现。
	overflowed atomic.Bool
}

func New(wait time.Duration, callback Callback) *Manager {
	if wait < 0 {
		wait = 5 * time.Second
	}
	return &Manager{wait: wait, callback: callback, watchErr: make(chan error, 1)}
}

func (m *Manager) Start(ctx context.Context, root string) error {
	m.Stop()
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create library watcher: %w", err)
	}
	root, err = filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		_ = fsw.Close()
		return err
	}
	watchCtx, cancel := context.WithCancel(ctx)
	events := make(chan fsnotify.Event, eventBufferSize)
	m.mu.Lock()
	m.cancel = cancel
	m.fsw = fsw
	m.root = root
	m.mu.Unlock()
	m.registering.Store(true)
	m.overflowed.Store(false)

	// ★ 顺序不能改：必须先起 drain + loop，再注册目录。
	//
	// fsnotify v1.5.4 的 Windows 后端（windows.go 的 readEvents）在
	// Events（缓冲仅 50）写满、或 Errors（无缓冲）无人读取时会永久阻塞；
	// 它一阻塞，之后每一个 Add 都停在 <-in.reply 上不再返回。
	// 曲库目录多、磁盘又有后台活动（杀软 / 索引 / 网盘同步都会触发
	// FILE_NOTIFY_CHANGE_ATTRIBUTES / LAST_ACCESS）时，注册阶段就能攒满
	// 50 条事件 —— 症状就是控制台停在 goose 那行、黑窗不动、8080 不监听。
	go m.drain(watchCtx, fsw, events)
	go m.loop(watchCtx, fsw, events, root)

	// 目录注册放后台 goroutine 并设上限：即使某个目录的打开操作真的卡住
	// （网络盘、休眠盘），也不能把整个进程的启动拖死。
	registered := make(chan error, 1)
	go func() {
		registerErr := addDirectories(watchCtx, fsw, root)
		m.registering.Store(false)
		registered <- registerErr
	}()

	timer := time.NewTimer(registerTimeout)
	defer timer.Stop()
	select {
	case err := <-registered:
		if err != nil {
			return fmt.Errorf("watch library directories: %w", err)
		}
		return nil
	case <-timer.C:
		m.registering.Store(false)
		return fmt.Errorf("watch library directories: 注册超过 %s 仍未完成（已降级，后台继续注册）", registerTimeout)
	case <-watchCtx.Done():
		return nil
	}
}

func (m *Manager) Stop() {
	m.mu.Lock()
	cancel := m.cancel
	fsw := m.fsw
	m.cancel = nil
	m.fsw = nil
	m.root = ""
	m.mu.Unlock()
	m.registering.Store(false)
	if cancel != nil {
		cancel()
	}
	if fsw != nil {
		_ = fsw.Close()
	}
}

func (m *Manager) Errors() <-chan error { return m.watchErr }

// drain 是 fsnotify 事件通道的唯一消费者，只做「尽快搬走」这一件事：
// 不碰文件系统、不派发回调。它是防止 readEvents 被反压卡死的兜底 ——
// loop 处理单个事件要做 1~3 次 os.Stat，事件洪峰下跟不上，
// 若让它直接读 fsw.Events，fsnotify 的 50 格缓冲很快写满并永久阻塞。
func (m *Manager) drain(ctx context.Context, fsw *fsnotify.Watcher, events chan fsnotify.Event) {
	defer close(events)
	for {
		select {
		case event, ok := <-fsw.Events:
			if !ok {
				return
			}
			select {
			case events <- event:
			default:
				// 内部缓冲也满了：丢事件并置位，让下一轮派发补一次整库对账。
				m.overflowed.Store(true)
			}
		case err, ok := <-fsw.Errors:
			if !ok {
				continue
			}
			m.reportErr(err)
		case <-ctx.Done():
			return
		}
	}
}

func (m *Manager) reportErr(err error) {
	if err == nil {
		return
	}
	select {
	case m.watchErr <- err:
	default:
	}
}

func (m *Manager) loop(ctx context.Context, fsw *fsnotify.Watcher, events <-chan fsnotify.Event, root string) {
	var timer *time.Timer
	var timerC <-chan time.Time
	targets := make(map[string]struct{})
	for {
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			if isIgnored(event.Name, root) {
				continue
			}
			if event.Op&fsnotify.Create != 0 {
				if info, err := filepath.Abs(event.Name); err == nil {
					if stat, statErr := fsStat(info); statErr == nil && stat.IsDir() {
						if addErr := addDirectories(ctx, fsw, info); addErr != nil {
							m.reportErr(fmt.Errorf("watch new library directory: %w", addErr))
						}
					}
				}
			}
			if info, statErr := fsStat(event.Name); statErr == nil && !info.IsDir() && !isWatchedFile(event.Name) {
				continue
			}
			if _, statErr := fsStat(event.Name); statErr != nil && event.Op&(fsnotify.Remove|fsnotify.Rename) == 0 && !isWatchedFile(event.Name) {
				continue
			}
			target := eventFolder(event.Name, root)
			if target == "" {
				continue
			}
			targets[target] = struct{}{}
			if timer == nil {
				timer = time.NewTimer(m.wait)
			} else {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(m.wait)
			}
			timerC = timer.C
		case <-timerC:
			timerC = nil
			// 目录注册还没结束：先把批次攒着，注册完再派发，
			// 避免启动期的事件洪峰直接触发一次对账扫描。
			if m.registering.Load() {
				timer.Reset(m.wait)
				timerC = timer.C
				continue
			}
			if m.overflowed.Swap(false) {
				// 内部缓冲曾满、丢过事件：补一次整库对账，别让变化漏到下次定时扫描。
				targets["."] = struct{}{}
			}
			batch := make([]string, 0, len(targets))
			for target := range targets {
				batch = append(batch, target)
			}
			targets = make(map[string]struct{})
			if len(batch) > 0 && m.callback != nil {
				if err := m.callback(ctx, batch); err != nil {
					if errors.Is(err, ErrBusy) {
						for _, target := range batch {
							targets[target] = struct{}{}
						}
						if timer == nil {
							timer = time.NewTimer(retryDelay(m.wait))
						} else {
							timer.Reset(retryDelay(m.wait))
						}
						timerC = timer.C
					} else {
						m.reportErr(err)
					}
				}
			}
		}
	}
}

func retryDelay(wait time.Duration) time.Duration {
	const minimum = 100 * time.Millisecond
	if wait < minimum {
		return minimum
	}
	return wait
}

func addDirectories(ctx context.Context, fsw *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		// watcher 已被 Stop / 重启：别再往下走（SkipAll 让 WalkDir 干净返回）。
		if ctx != nil && ctx.Err() != nil {
			return fs.SkipAll
		}
		if walkErr != nil {
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if path != root && ignoredDirectory(entry.Name()) {
				return fs.SkipDir
			}
			return fsw.Add(path)
		}
		return nil
	})
}

func fsStat(path string) (fs.FileInfo, error) { return os.Stat(path) }

func eventFolder(path, root string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == "." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "."
	}
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		return filepath.ToSlash(relative)
	}
	return filepath.ToSlash(filepath.Dir(relative))
}

func isIgnored(path, root string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return true
	}
	parts := strings.Split(filepath.ToSlash(relative), "/")
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		if ignoredDirectory(part) || strings.HasPrefix(part, ".") {
			return true
		}
	}
	base := filepath.Base(path)
	return base == ".DS_Store" || strings.HasPrefix(base, ".")
}

func ignoredDirectory(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	switch strings.ToLower(name) {
	case "@eadir", "$recycle.bin", "system volume information":
		return true
	default:
		return false
	}
}

func isWatchedFile(path string) bool {
	extension := filepath.Ext(path)
	if strings.EqualFold(extension, ".lrc") {
		return true
	}
	_, supported := domain.TrackFormatFromExtension(extension)
	return supported
}
