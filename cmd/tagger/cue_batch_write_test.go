package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ericwyn/tagger/internal/domain"
	"github.com/ericwyn/tagger/internal/filewrite"
	"github.com/ericwyn/tagger/internal/jobs"
	"github.com/ericwyn/tagger/internal/library"
	"github.com/ericwyn/tagger/internal/scanner"
	"github.com/ericwyn/tagger/internal/store"
	"github.com/ericwyn/tagger/internal/tags"
)

// cueArtworkEngine 是整轨批次回归测试用的假引擎。它如实模拟真实引擎里唯一
// 关键的那条副作用：写封面会改动音频文件本体（大小 + mtime），从而刷新
// scanner.FileRevision。整轨的 revision 是按父音频算的，所以这条副作用正是
// 「同专辑兄弟轨道 revision 集体失效」的根源 —— 保留它当绊线：一旦有人再把
// 封面写回父音频，文件就变大，用例立刻红。
type cueArtworkEngine struct {
	mu      sync.Mutex
	raw     map[string]map[string][]string
	artwork map[string][]byte
}

func newCueArtworkEngine() *cueArtworkEngine {
	return &cueArtworkEngine{raw: map[string]map[string][]string{}, artwork: map[string][]byte{}}
}

// cueEngineKey 把 filewrite 原子写的临时名归一化回原文件名。临时名形如
// .<原名>.tagger-<随机><扩展名>，写封面走的是「复制 → 改临时文件 → 改名」。
func cueEngineKey(path string) string {
	directory, base := filepath.Split(path)
	if trimmed, ok := strings.CutPrefix(base, "."); ok {
		if index := strings.Index(trimmed, ".tagger-"); index > 0 {
			base = trimmed[:index] + filepath.Ext(base)
		}
	}
	return filepath.Join(directory, base)
}

func (e *cueArtworkEngine) Read(_ context.Context, path string) (tags.Snapshot, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	key := cueEngineKey(path)
	raw := make(map[string][]string, len(e.raw[key]))
	for name, values := range e.raw[key] {
		raw[name] = append([]string(nil), values...)
	}
	count := 0
	if e.artwork[key] != nil {
		count = 1
	}
	return tags.Snapshot{Raw: raw, DurationSeconds: 120, ArtworkCount: count}, nil
}

func (e *cueArtworkEngine) Write(_ context.Context, path string, updates map[string][]string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	key := cueEngineKey(path)
	if e.raw[key] == nil {
		e.raw[key] = map[string][]string{}
	}
	for name, values := range updates {
		if len(values) == 0 {
			delete(e.raw[key], name)
			continue
		}
		e.raw[key][name] = append([]string(nil), values...)
	}
	return nil
}

func (*cueArtworkEngine) Version() string { return "cue-artwork-test" }

func (e *cueArtworkEngine) ReadArtwork(_ context.Context, path string, _ int) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]byte(nil), e.artwork[cueEngineKey(path)]...), nil
}

func (e *cueArtworkEngine) WriteArtwork(_ context.Context, path string, _ int, data []byte, mime string) error {
	key := cueEngineKey(path)
	e.mu.Lock()
	if len(data) == 0 {
		delete(e.artwork, key)
	} else {
		e.artwork[key] = append([]byte(nil), data...)
	}
	e.mu.Unlock()
	if len(data) == 0 {
		return nil
	}
	// 真实引擎把封面嵌进音频文件本体，文件大小与 mtime 都会变。
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// 回归：整轨父音频只读（老板 2026-09-28 定的规则）。
//
// 假引擎如实模拟真实引擎那条要命的副作用：写封面会改动音频文件本体（大小 +
// mtime），从而刷新 scanner.FileRevision。旧实现把整轨封面嵌进父音频，同专辑
// 所有兄弟轨道的 revision 集体失效 —— 实测 12 首整轨写入 11/11 全失败，父音频
// 被嵌了 76KB 的 ID3 块。
//
// 现在规则是：整轨只写 cue 与 .lrc，父音频一个字节都不动。这个用例同时守住
// 三件事 —— ① 批次照旧全部成功（含审核页带下来的过期 revision）；② 父音频
// size/mtime 不变、引擎一次都没被要求写封面；③ cue 照旧被两条轨道写入。
// 谁再把封面塞回父音频，文件就会变大，用例立刻红。
func TestBatchEditSkipsCueArtworkAndKeepsParentUntouched(t *testing.T) {
	root := t.TempDir()
	audioPath := filepath.Join(root, "album.wav")
	const placeholder = "RIFF....WAVEfmt "
	if err := os.WriteFile(audioPath, []byte(placeholder), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(audioPath)
	if err != nil {
		t.Fatal(err)
	}
	cueBody := "PERFORMER \"甲\"\nTITLE \"专辑\"\nFILE \"album.wav\" WAVE\n" +
		"  TRACK 01 AUDIO\n    TITLE \"一\"\n    INDEX 01 00:00:00\n" +
		"  TRACK 02 AUDIO\n    TITLE \"二\"\n    INDEX 01 00:03:00\n"
	if err := os.WriteFile(filepath.Join(root, "album.cue"), []byte(cueBody), 0o644); err != nil {
		t.Fatal(err)
	}

	engine := newCueArtworkEngine()
	musicScanner, err := scanner.New(engine, scanner.Options{Root: root, Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	repository, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "tagger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	service, err := library.New(context.Background(), musicScanner, repository)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := filewrite.New(root, engine)
	if err != nil {
		t.Fatal(err)
	}

	virtual := make([]domain.Track, 0, 2)
	for _, track := range service.ListTracks(library.TrackFilter{}) {
		if domain.IsCueVirtualPath(track.RelativePath) {
			virtual = append(virtual, track)
		}
	}
	if len(virtual) != 2 {
		t.Fatalf("cue virtual tracks = %d, want 2", len(virtual))
	}
	// ★ 故意塞一个必然对不上的 baseRevision，复刻老板的实际场景：审核页打开那一刻
	// 的曲库索引值，在点「确认并创建写入任务」时早就过期了（父音频被上一次写入改过）。
	// 整轨不该靠客户端 revision 兜底 —— cueRevisions.base() 会用重扫后的曲库当前值。
	items := make([]domain.BatchEditItem, 0, len(virtual))
	for _, track := range virtual {
		items = append(items, domain.BatchEditItem{TrackID: track.ID, BaseRevision: "rev-stale-from-review-page"})
	}
	payload := domain.BatchEditPayload{
		Items:      items,
		Operations: []domain.BatchEditOperation{{Field: "title", Mode: domain.BatchEditSet, Value: "改过的标题"}},
		Artwork:    &domain.BatchArtwork{Action: domain.BatchArtworkReplace, Data: batchArtworkData(t), MIME: "image/png", MaxSize: 500},
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	manager := jobs.New(repository)
	manager.Register(domain.JobBatchEdit, newBatchEditHandler(service, writer, repository))
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	created, err := manager.Enqueue(context.Background(), domain.Job{
		Kind: domain.JobBatchEdit, LibraryID: service.Library().ID, Title: "整轨批量写入", Total: len(items), Payload: string(payloadJSON),
	})
	if err != nil {
		t.Fatal(err)
	}
	job := waitBatchEditJob(t, manager, created.ID)
	if job.State != domain.JobSucceeded || job.Succeeded != len(items) || job.Failed != 0 {
		t.Fatalf("整轨批次写入结果 = %#v（兄弟轨道 revision 未同步）", job)
	}

	// ★ 核心断言：父音频一个字节都没动。
	after, err := os.Stat(audioPath)
	if err != nil {
		t.Fatal(err)
	}
	if after.Size() != before.Size() {
		t.Fatalf("父音频被改动了: size %d -> %d（封面不该写内嵌）", before.Size(), after.Size())
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("父音频 mtime 被刷新: %v -> %v", before.ModTime(), after.ModTime())
	}
	// 引擎一次都没被要求往父音频写封面。
	if len(engine.artwork) != 0 {
		t.Fatalf("封面被写入了整轨文件: %#v", engine.artwork)
	}

	// 该写的地方照旧写：两条虚拟轨道的标签都进了 cue。
	written, err := os.ReadFile(filepath.Join(root, "album.cue"))
	if err != nil || strings.Count(string(written), "改过的标题") != 2 {
		t.Fatalf("cue 未被两条虚拟轨道写入: %q err=%v", written, err)
	}
}
