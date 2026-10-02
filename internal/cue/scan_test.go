package cue

import (
	"os"
	"path/filepath"
	"testing"
)

// 简体 cue 文件名 + 繁体 FILE 行 —— 老板库里 [費玉清東尼專輯] 的真实形态。
const simplifiedNameCueBody = `PERFORMER "費玉清"
TITLE "萬里長城"
FILE "費玉清 - 萬里長城.wav" WAVE
  TRACK 01 AUDIO
    TITLE "夜來香"
    INDEX 01 00:00:00
  TRACK 02 AUDIO
    TITLE "明月千里寄相思"
    INDEX 01 04:12:00
`

func writeCueFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSameNameSheetPathRequiresMatchingBaseName(t *testing.T) {
	dir := t.TempDir()
	writeCueFile(t, filepath.Join(dir, "费玉清 - 萬里長城.cue"), simplifiedNameCueBody)
	if got := SameNameSheetPath(dir, "費玉清 - 萬里長城.wav"); got != "" {
		t.Fatalf("simplified cue name matched a traditional audio name: %q", got)
	}
	writeCueFile(t, filepath.Join(dir, "費玉清 - 萬里長城.cue"), simplifiedNameCueBody)
	if got := SameNameSheetPath(dir, "費玉清 - 萬里長城.wav"); got == "" {
		t.Fatal("same-name cue was not found")
	}
}

func TestMatchesAudioIgnoresCaseAndExtension(t *testing.T) {
	sheet, err := ParseBytes([]byte(simplifiedNameCueBody))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"費玉清 - 萬里長城.wav",
		"費玉清 - 萬里長城.WAV",
		"費玉清 - 萬里長城.flac",
	} {
		if !MatchesAudio(sheet, name) {
			t.Errorf("MatchesAudio(%q) = false, want true", name)
		}
	}
	if MatchesAudio(sheet, "費玉清 - 另一個專輯.wav") {
		t.Error("unrelated audio matched the cue FILE line")
	}
	if MatchesAudio(&Sheet{}, "費玉清 - 萬里長城.wav") {
		t.Error("empty FILE line must not match")
	}
}

func TestResolveSheetFallsBackToFileFieldForRenamedCue(t *testing.T) {
	dir := t.TempDir()
	writeCueFile(t, filepath.Join(dir, "費玉清 - 萬里長城.wav"), "audio")
	writeCueFile(t, filepath.Join(dir, "费玉清 - 萬里長城.cue"), simplifiedNameCueBody)

	got, ok := ResolveSheet(dir, "費玉清 - 萬里長城.wav")
	if !ok {
		t.Fatal("FILE-line fallback did not bind the renamed cue")
	}
	if filepath.Base(got) != "费玉清 - 萬里長城.cue" {
		t.Fatalf("resolved %q", got)
	}
}

func TestResolveSheetKeepsSplitTrackRipOutOfCueExpansion(t *testing.T) {
	dir := t.TempDir()
	writeCueFile(t, filepath.Join(dir, "01 - 沈默.flac"), "audio")
	writeCueFile(t, filepath.Join(dir, "02 - 夜夜夜夜.flac"), "audio")
	writeCueFile(t, filepath.Join(dir, "noncompliant.cue"), `FILE "01 - 沈默.flac" WAVE
  TRACK 01 AUDIO
    INDEX 01 00:00:00
`)
	if got, ok := ResolveSheet(dir, "01 - 沈默.flac"); ok {
		t.Fatalf("split-track rip was expanded through a stray cue: %q", got)
	}
}

func TestResolveSheetPrefersSameNameCue(t *testing.T) {
	dir := t.TempDir()
	writeCueFile(t, filepath.Join(dir, "album.wav"), "audio")
	writeCueFile(t, filepath.Join(dir, "album.cue"), `FILE "album.wav" WAVE
  TRACK 01 AUDIO
    INDEX 01 00:00:00
`)
	writeCueFile(t, filepath.Join(dir, "zz-stray.cue"), `FILE "album.wav" WAVE
  TRACK 01 AUDIO
    INDEX 01 00:00:00
`)
	got, ok := ResolveSheet(dir, "album.wav")
	if !ok || filepath.Base(got) != "album.cue" {
		t.Fatalf("ResolveSheet = %q, %v; want album.cue", got, ok)
	}
}

func TestScanDirCountsBindableAudioAndReportsFailures(t *testing.T) {
	dir := t.TempDir()
	writeCueFile(t, filepath.Join(dir, "one.wav"), "audio")
	writeCueFile(t, filepath.Join(dir, "two.flac"), "audio")
	writeCueFile(t, filepath.Join(dir, "note.txt"), "not audio")
	writeCueFile(t, filepath.Join(dir, "good.cue"), simplifiedNameCueBody)
	writeCueFile(t, filepath.Join(dir, "empty.cue"), "TITLE \"没有 TRACK\"\n")

	scanned := ScanDir(dir)
	if scanned.AudioCount != 2 {
		t.Fatalf("AudioCount = %d, want 2", scanned.AudioCount)
	}
	if len(scanned.Sheets) != 1 || filepath.Base(scanned.Sheets[0].Path) != "good.cue" {
		t.Fatalf("Sheets = %#v", scanned.Sheets)
	}
	if len(scanned.Failures) != 1 || filepath.Base(scanned.Failures[0].Path) != "empty.cue" {
		t.Fatalf("Failures = %#v", scanned.Failures)
	}
}

func TestBindableAudioExtOnlyAcceptsWholeTrackFormats(t *testing.T) {
	for _, ext := range []string{".wav", ".WAV", ".flac", ".FLAC"} {
		if !BindableAudioExt(ext) {
			t.Errorf("BindableAudioExt(%q) = false, want true", ext)
		}
	}
	for _, ext := range []string{".mp3", ".m4a", ".ogg", ".opus", ".cue", ""} {
		if BindableAudioExt(ext) {
			t.Errorf("BindableAudioExt(%q) = true, want false", ext)
		}
	}
}
