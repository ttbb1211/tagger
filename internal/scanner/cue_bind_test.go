package scanner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 简体 cue 名 + 繁体 FILE 行，与老板库里 [費玉清東尼專輯] 的形态一致。
const renamedCueBody = `PERFORMER "費玉清"
TITLE "萬里長城"
FILE "費玉清 - 萬里長城.wav" WAVE
  TRACK 01 AUDIO
    TITLE "夜來香"
    INDEX 01 00:00:00
  TRACK 02 AUDIO
    TITLE "明月千里寄相思"
    INDEX 01 04:12:00
`

func writeCueFixture(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// cue 文件名被改成简体、音频仍是繁体时，同名探测落空，须按 cue 内 FILE 行配对。
func TestBindCueSheetsMatchesRenamedCueThroughFileLine(t *testing.T) {
	root := t.TempDir()
	audio := filepath.Join(root, "費玉清 - 萬里長城.wav")
	writeCueFixture(t, audio, "audio")
	writeCueFixture(t, filepath.Join(root, "费玉清 - 萬里長城.cue"), renamedCueBody)

	musicScanner, err := New(fakeEngine{}, Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	binds, warnings := musicScanner.bindCueSheets([]string{audio})
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	bind, ok := binds[audio]
	if !ok {
		t.Fatal("renamed cue was not bound through the FILE line")
	}
	if bind.cueRel != "费玉清 - 萬里長城.cue" || len(bind.sheet.Tracks) != 2 {
		t.Fatalf("bind = %+v", bind)
	}
}

// 目录内有多条整轨音频时不得按 FILE 行配对：拆分单轨旁的残留整轨 cue
// （EAC 的 noncompliant.cue 指向第 1 轨）会把专辑重复展开。
func TestBindCueSheetsIgnoresStrayCueBesideSplitTracks(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "01 - 沈默.flac")
	second := filepath.Join(root, "02 - 夜夜夜夜.flac")
	writeCueFixture(t, first, "audio")
	writeCueFixture(t, second, "audio")
	writeCueFixture(t, filepath.Join(root, "noncompliant.cue"), `FILE "01 - 沈默.flac" WAVE
  TRACK 01 AUDIO
    INDEX 01 00:00:00
`)

	musicScanner, err := New(fakeEngine{}, Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	binds, warnings := musicScanner.bindCueSheets([]string{first, second})
	if len(binds) != 0 || len(warnings) != 0 {
		t.Fatalf("binds = %v warnings = %v", binds, warnings)
	}
}

// 同名 cue 保持原有行为，且只有整轨音频（wav/flac）参与配对。
func TestBindCueSheetsKeepsSameNameBehaviourAndSkipsOtherFormats(t *testing.T) {
	root := t.TempDir()
	album := filepath.Join(root, "album.flac")
	writeCueFixture(t, album, "audio")
	writeCueFixture(t, filepath.Join(root, "album.cue"), `FILE "album.flac" WAVE
  TRACK 01 AUDIO
    INDEX 01 00:00:00
`)
	song := filepath.Join(root, "song.mp3")
	writeCueFixture(t, song, "audio")
	writeCueFixture(t, filepath.Join(root, "song.cue"), `FILE "song.mp3" WAVE
  TRACK 01 AUDIO
    INDEX 01 00:00:00
`)

	musicScanner, err := New(fakeEngine{}, Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	binds, warnings := musicScanner.bindCueSheets([]string{album, song})
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if _, ok := binds[album]; !ok {
		t.Fatal("same-name flac cue was not bound")
	}
	if _, ok := binds[song]; ok {
		t.Fatal("mp3 must not participate in cue binding")
	}
}

// cue 存在但解析失败时退回普通单文件索引，并把原因记入 warnings。
func TestBindCueSheetsReportsUnparsableSameNameCue(t *testing.T) {
	root := t.TempDir()
	audio := filepath.Join(root, "album.wav")
	writeCueFixture(t, audio, "audio")
	writeCueFixture(t, filepath.Join(root, "album.cue"), "TITLE \"没有 TRACK 条目\"\n")

	musicScanner, err := New(fakeEngine{}, Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	binds, warnings := musicScanner.bindCueSheets([]string{audio})
	if len(binds) != 0 {
		t.Fatalf("binds = %v, want none", binds)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly one", warnings)
	}
}

// Result.Scanned / Indexed 是增量落库判定「多余的行」的唯一依据：
// 整轨专辑的父音频必须出现在 Scanned（扫描器走到过）里，但**不能**出现在
// Indexed（扫描器为它产出的记录）里 —— 它只会被展开成 #cue:N 虚拟轨道。
// 库里遗留的父音频行正是靠这个差集被判为多余行并删除。
func TestScanResultReportsScannedFilesAndProducedTracks(t *testing.T) {
	root := t.TempDir()
	writeCueFixture(t, filepath.Join(root, "Album", "Album.wav"), "audio")
	writeCueFixture(t, filepath.Join(root, "Album", "Album.cue"), `FILE "Album.wav" WAVE
  TRACK 01 AUDIO
    TITLE "第一首"
    INDEX 01 00:00:00
  TRACK 02 AUDIO
    TITLE "第二首"
    INDEX 01 04:00:00
`)
	writeCueFixture(t, filepath.Join(root, "single.flac"), "audio")

	musicScanner, err := New(fakeEngine{}, Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	result, err := musicScanner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(result.Scanned, ","); got != "Album/Album.wav,single.flac" {
		t.Fatalf("Scanned = %v, want the two audio files only", result.Scanned)
	}
	want := []string{"Album/Album.wav#cue:1", "Album/Album.wav#cue:2", "single.flac"}
	if got := strings.Join(result.Indexed, ","); got != strings.Join(want, ",") {
		t.Fatalf("Indexed = %v, want %v", result.Indexed, want)
	}
	if result.Report.WarningCount != 0 {
		t.Fatalf("warnings = %v, want none", result.Report.Warnings)
	}
}
