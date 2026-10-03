package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ericwyn/tagger/internal/domain"
	"github.com/ericwyn/tagger/internal/scanner"
)

const wholeTrackCue = `PERFORMER "测试歌手"
TITLE "整轨测试专辑"
FILE "Album.wav" WAVE
  TRACK 01 AUDIO
    TITLE "第一首"
    INDEX 01 00:00:00
  TRACK 02 AUDIO
    TITLE "第二首"
    INDEX 01 00:03:00
`

// buildWholeTrackAlbum 造一个整轨专辑：Album/Album.wav + 同名 cue。
// 扫描器会把父音频展开成 #cue:N 虚拟轨道，索引里没有父音频记录。
func buildWholeTrackAlbum(t *testing.T, root string) (audioPath, cuePath string) {
	t.Helper()
	album := filepath.Join(root, "Album")
	if err := os.MkdirAll(album, 0o755); err != nil {
		t.Fatal(err)
	}
	audioPath = filepath.Join(album, "Album.wav")
	if err := os.WriteFile(audioPath, make([]byte, 64), 0o644); err != nil {
		t.Fatal(err)
	}
	cuePath = filepath.Join(album, "Album.cue")
	if err := os.WriteFile(cuePath, []byte(wholeTrackCue), 0o644); err != nil {
		t.Fatal(err)
	}
	return audioPath, cuePath
}

func newWholeTrackService(t *testing.T, root string) (*Service, *countingServiceEngine) {
	t.Helper()
	engine := &countingServiceEngine{}
	musicScanner, err := scanner.New(engine, scanner.Options{Root: root, Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	// New 在没有仓储时会立刻全扫一次，正好把整轨专辑展开入库。
	service, err := New(context.Background(), musicScanner)
	if err != nil {
		t.Fatal(err)
	}
	return service, engine
}

func assertCueAlbumIndexed(t *testing.T, service *Service, wantTracks int) {
	t.Helper()
	page, err := service.ListTrackPage(TrackQuery{}, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != wantTracks {
		t.Fatalf("整轨专辑应展开为 %d 条虚拟轨道，实际 %d 条：%#v", wantTracks, page.Total, page.Tracks)
	}
	for _, track := range page.Tracks {
		if !domain.IsCueVirtualPath(track.RelativePath) {
			t.Fatalf("整轨专辑不应出现父音频独立记录：%q", track.RelativePath)
		}
		if track.Missing {
			t.Fatalf("整轨虚拟轨道被标成缺失：%q", track.RelativePath)
		}
	}
}

// 整轨专辑的父音频在索引里没有记录，但它确实躺在磁盘上。目录对账必须把
// 「有文件、无记录」与「新文件」区分开：草稿化会立刻入队一次目标扫描，
// 扫描又把母盘展开回虚拟轨道，下一轮对账再次草稿化 —— 窗口一获得焦点
// （focus / visibilitychange 都会触发对账）就空转一次重扫。
func TestReconcileDirectoryKeepsWholeTrackAlbumIndexed(t *testing.T) {
	root := t.TempDir()
	buildWholeTrackAlbum(t, root)
	service, engine := newWholeTrackService(t, root)

	assertCueAlbumIndexed(t, service, 2)
	readsAfterScan := engine.reads.Load()

	// 对账两次：第一轮覆盖「首次对账」，第二轮覆盖「对账自身不再制造变更」。
	for round := 1; round <= 2; round++ {
		result, err := service.ReconcileDirectory(context.Background(), "Album")
		if err != nil {
			t.Fatalf("第 %d 次对账失败：%v", round, err)
		}
		if result.Changed {
			t.Fatalf("第 %d 次对账产生了变更（整轨母盘被当成新文件）：%#v", round, result)
		}
		if len(result.Pending) != 0 {
			t.Fatalf("第 %d 次对账入队了待扫描路径：%#v", round, result.Pending)
		}
		if result.Missing != 0 {
			t.Fatalf("第 %d 次对账把整轨虚拟轨道判成缺失：%#v", round, result)
		}
	}

	if engine.reads.Load() != readsAfterScan {
		t.Fatalf("对账读了标签：%d -> %d", readsAfterScan, engine.reads.Load())
	}
	assertCueAlbumIndexed(t, service, 2)
}

// 父音频真被删掉时，整轨虚拟轨道必须照旧判成缺失（本修复不能把缺失检测
// 一起废掉）。
func TestReconcileDirectoryMarksCueAlbumMissingWhenAudioRemoved(t *testing.T) {
	root := t.TempDir()
	audioPath, _ := buildWholeTrackAlbum(t, root)
	service, _ := newWholeTrackService(t, root)
	assertCueAlbumIndexed(t, service, 2)

	if err := os.Remove(audioPath); err != nil {
		t.Fatal(err)
	}
	result, err := service.ReconcileDirectory(context.Background(), "Album")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || result.Missing != 2 || len(result.Pending) != 0 {
		t.Fatalf("父音频删除后的对账结果异常：%#v", result)
	}
	visible, _ := service.ListTrackPage(TrackQuery{}, "", 100)
	missing, _ := service.ListTrackPage(TrackQuery{Health: domain.HealthMissing}, "", 100)
	if visible.Total != 0 || missing.Total != 2 {
		t.Fatalf("可见=%d 缺失=%d", visible.Total, missing.Total)
	}
}

// cue 被删掉、音频还在：母盘不再是整轨专辑，应退回普通单文件并草稿化；
// 残留的虚拟轨道随之判缺失。这条锁住 CueBoundAudio 守卫的语义 —— 否则
// 「cue 删除后音频永远不被索引」会被静默引入。
func TestReconcileDirectoryDraftsMasterAfterCueRemoved(t *testing.T) {
	root := t.TempDir()
	_, cuePath := buildWholeTrackAlbum(t, root)
	service, _ := newWholeTrackService(t, root)
	assertCueAlbumIndexed(t, service, 2)

	if err := os.Remove(cuePath); err != nil {
		t.Fatal(err)
	}
	result, err := service.ReconcileDirectory(context.Background(), "Album")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || len(result.Pending) != 1 || result.Pending[0] != "Album/Album.wav" {
		t.Fatalf("cue 删除后的对账结果异常：%#v", result)
	}
	if result.Missing != 2 {
		t.Fatalf("残留虚拟轨道应判缺失，实际 missing=%d：%#v", result.Missing, result)
	}
	drafts, err := service.ListTrackPage(TrackQuery{}, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if drafts.Total != 1 || drafts.Tracks[0].SyncState != domain.SyncDraft {
		t.Fatalf("母盘应退回普通单文件草稿：%#v", drafts.Tracks)
	}
}

// 复现 v1.6.10 及更早版本留下的损坏索引：整轨专辑的虚拟轨道被误标缺失、
// 母盘上还压着一条不该存在的草稿记录。对账必须既能止损（不再重扫空转），
// 也能自愈（把误标的缺失纠正回来），否则老板升级后只能靠一次全量重扫。
func TestReconcileDirectoryRepairsDamagedCueAlbumIndex(t *testing.T) {
	root := t.TempDir()
	buildWholeTrackAlbum(t, root)
	engine := &countingServiceEngine{}
	musicScanner, err := scanner.New(engine, scanner.Options{Root: root, Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	damaged := []domain.Track{
		{
			ID: "trk-cue-1", FileName: "Album.cue", RelativePath: "Album/Album.wav#cue:1",
			Format: domain.FormatWAV, Title: "第一首", SyncState: domain.SyncIndexed,
			Missing: true, MissingSince: "2026-10-03T02:00:00Z", Health: domain.HealthMissing,
		},
		{
			ID: "trk-cue-2", FileName: "Album.cue", RelativePath: "Album/Album.wav#cue:2",
			Format: domain.FormatWAV, Title: "第二首", SyncState: domain.SyncIndexed,
			Missing: true, MissingSince: "2026-10-03T02:00:00Z", Health: domain.HealthMissing,
		},
		{
			// 母盘幽灵记录：文件在磁盘上，但它不该被索引，历史上被草稿化了。
			ID: "trk-phantom", FileName: "Album.wav", RelativePath: "Album/Album.wav",
			Format: domain.FormatWAV, Title: "Album", SyncState: domain.SyncDraft,
		},
	}
	repo := &memoryServiceRepository{scans: map[string]scanner.Result{
		root: {
			Library: domain.LibrarySummary{ID: "lib-damaged", Name: "Damaged", RootPath: root, TrackCount: 1},
			Tracks:  damaged,
		},
	}}
	service, err := New(context.Background(), musicScanner, repo)
	if err != nil {
		t.Fatal(err)
	}
	// 预置索引直接加载，不该触发重扫。
	if engine.reads.Load() != 0 {
		t.Fatalf("加载预置索引时读了标签：%d", engine.reads.Load())
	}

	result, err := service.ReconcileDirectory(context.Background(), "Album")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pending) != 0 {
		t.Fatalf("损坏索引对账后仍入队重扫：%#v", result.Pending)
	}
	if result.Restored != 2 {
		t.Fatalf("应纠正 2 条误标缺失的虚拟轨道，实际 %d：%#v", result.Restored, result)
	}
	if result.Missing != 0 {
		t.Fatalf("对账不该新增缺失：%#v", result)
	}
	if engine.reads.Load() != 0 {
		t.Fatalf("对账读了标签：%d", engine.reads.Load())
	}

	visible, err := service.ListTrackPage(TrackQuery{}, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if visible.Total != 2 {
		t.Fatalf("纠正后应可见 2 条虚拟轨道，实际 %d：%#v", visible.Total, visible.Tracks)
	}
	for _, track := range visible.Tracks {
		if !domain.IsCueVirtualPath(track.RelativePath) {
			t.Fatalf("母盘幽灵记录不该可见：%q", track.RelativePath)
		}
		if track.Missing || track.Health != domain.HealthTagCompatibility || track.MissingSince != "" {
			t.Fatalf("虚拟轨道未正确还原：%#v", track)
		}
	}

	// 幂等：再对账一次不应产生任何变更，也不该再有可纠正项。
	again, err := service.ReconcileDirectory(context.Background(), "Album")
	if err != nil {
		t.Fatal(err)
	}
	if again.Changed || again.Restored != 0 || len(again.Pending) != 0 || again.Missing != 0 {
		t.Fatalf("第二次对账应无变更：%#v", again)
	}
}
