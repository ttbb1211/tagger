package watcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestManagerDebouncesFilesystemEvents(t *testing.T) {
	root := t.TempDir()
	changes := make(chan []string, 2)
	manager := New(40*time.Millisecond, func(_ context.Context, targets []string) error {
		changes <- targets
		return nil
	})
	if err := manager.Start(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	defer manager.Stop()
	path := filepath.Join(root, "song.mp3")
	if err := os.WriteFile(path, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case targets := <-changes:
		if len(targets) != 1 || targets[0] != "." {
			t.Fatalf("targets=%v, want root target", targets)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("watcher did not emit a debounced event")
	}
	select {
	case extra := <-changes:
		t.Fatalf("unexpected second debounced batch: %v", extra)
	case <-time.After(120 * time.Millisecond):
	}
}

func TestIgnoredPaths(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "music")
	for _, path := range []string{filepath.Join(root, ".hidden", "song.mp3"), filepath.Join(root, "@eaDir", "song.mp3"), filepath.Join(root, ".DS_Store")} {
		if !isIgnored(path, root) {
			t.Fatalf("path %q was not ignored", path)
		}
	}
	if isIgnored(filepath.Join(root, "Artist", "song.mp3"), root) {
		t.Fatal("regular audio path was ignored")
	}
}

func TestWatchedFilesIncludeOggAndOpus(t *testing.T) {
	for _, path := range []string{"song.ogg", "podcast.OPUS", "lyrics.lrc", "track.flac"} {
		if !isWatchedFile(path) {
			t.Errorf("watched file %q was ignored", path)
		}
	}
	for _, path := range []string{"cover.jpg", "track.aac", "notes.txt"} {
		if isWatchedFile(path) {
			t.Errorf("unwatched file %q was accepted", path)
		}
	}
}

func TestManagerRetriesBusyBatch(t *testing.T) {
	root := t.TempDir()
	changes := make(chan []string, 1)
	attempts := 0
	manager := New(10*time.Millisecond, func(_ context.Context, targets []string) error {
		attempts++
		if attempts == 1 {
			return ErrBusy
		}
		changes <- targets
		return nil
	})
	if err := manager.Start(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	defer manager.Stop()
	if err := os.WriteFile(filepath.Join(root, "song.mp3"), []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case targets := <-changes:
		if len(targets) != 1 || targets[0] != "." {
			t.Fatalf("targets=%v, want retried root target", targets)
		}
		if attempts != 2 {
			t.Fatalf("callback attempts=%d, want 2", attempts)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("busy watcher batch was not retried")
	}
}

func TestManagerWatchesPrepopulatedDirectoryMovedIntoRoot(t *testing.T) {
	root := t.TempDir()
	stagingRoot := t.TempDir()
	staging := filepath.Join(stagingRoot, "Album")
	if err := os.MkdirAll(filepath.Join(staging, "Disc 1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "Disc 1", "song.flac"), []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	changes := make(chan []string, 1)
	manager := New(20*time.Millisecond, func(_ context.Context, targets []string) error {
		changes <- targets
		return nil
	})
	if err := manager.Start(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	defer manager.Stop()
	if err := os.Rename(staging, filepath.Join(root, "Album")); err != nil {
		t.Fatal(err)
	}
	select {
	case targets := <-changes:
		found := false
		for _, target := range targets {
			if target == "Album" {
				found = true
			}
		}
		if !found {
			t.Fatalf("targets=%v, want Album", targets)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("watcher did not report moved directory")
	}
}

// TestStartSurvivesEventFloodDuringRegistration 复刻 Windows 上的「启动假死」。
//
// fsnotify v1.5.4 的 Windows 后端（windows.go 的 readEvents）在 Events（缓冲 50）
// 写满、或 Errors（无缓冲）无人读取时会永久阻塞；它一阻塞，之后每一个 Add 都停在
// <-in.reply 上不再返回。老实现「先把目录全部 Add 完、再起消费协程」正好落在这个
// 陷阱里：注册期间磁盘只要有后台活动（杀软 / 索引 / 网盘同步）攒够事件，注册就再也
// 回不来 —— 症状是控制台停在 goose 那行、黑窗不动、端口不监听。
//
// 本用例在注册期间持续制造事件洪峰，要求 Start 必须能返回。
// 仅 Windows 生效：其他平台的后端没有这个阻塞行为。
func TestStartSurvivesEventFloodDuringRegistration(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("fsnotify 的该阻塞行为只在 Windows 后端出现")
	}
	root := t.TempDir()
	const dirs = 400
	for i := 0; i < dirs; i++ {
		if err := os.Mkdir(filepath.Join(root, fmt.Sprintf("d%03d", i)), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	stop := make(chan struct{})
	var flooders sync.WaitGroup
	for i := 0; i < 12; i++ {
		dir := filepath.Join(root, fmt.Sprintf("d%03d", i))
		flooders.Add(1)
		go func(dir string) {
			defer flooders.Done()
			probe := filepath.Join(dir, "flood.tmp")
			for {
				select {
				case <-stop:
					return
				default:
				}
				if err := os.WriteFile(probe, []byte("x"), 0o600); err != nil {
					return
				}
				_ = os.Remove(probe)
			}
		}(dir)
	}

	manager := New(time.Second, nil)
	done := make(chan error, 1)
	go func() { done <- manager.Start(context.Background(), root) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
	case <-time.After(30 * time.Second):
		close(stop)
		flooders.Wait()
		t.Fatal("Start 未在 30s 内返回：目录注册被事件洪峰卡死（消费协程必须早于注册启动）")
	}

	close(stop)
	flooders.Wait()
	manager.Stop()
}
