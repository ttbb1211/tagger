package cue

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// BindableAudioExt reports whether ext (including the leading dot) names a
// whole-track audio format that may be paired with a cue sheet. It mirrors the
// scanner rule: only wav and flac participate in cue binding.
func BindableAudioExt(ext string) bool {
	return strings.EqualFold(ext, ".wav") || strings.EqualFold(ext, ".flac")
}

// SheetFile is one successfully parsed cue sheet together with its path.
type SheetFile struct {
	Path  string
	Sheet *Sheet
}

// SheetFailure records a cue file that exists but could not be read or parsed.
type SheetFailure struct {
	Path string
	Err  error
}

// DirSheets summarises the cue situation of a single directory.
type DirSheets struct {
	// Sheets holds every successfully parsed cue in the directory, sorted by path.
	Sheets []SheetFile
	// Failures holds every cue that could not be read or parsed, sorted by path.
	Failures []SheetFailure
	// AudioCount is how many bindable whole-track audio files (wav/flac) sit
	// next to the cue sheets.
	AudioCount int
}

// ScanDir reads and parses every .cue directly inside dir. Hidden files and
// sub-directories are skipped, matching the scanner's library rules. It also
// counts the bindable whole-track audio files in the same directory.
func ScanDir(dir string) DirSheets {
	result := DirSheets{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return result
	}
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if BindableAudioExt(ext) {
			result.AudioCount++
			continue
		}
		if !strings.EqualFold(ext, ".cue") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			result.Failures = append(result.Failures, SheetFailure{Path: path, Err: readErr})
			continue
		}
		sheet, parseErr := ParseBytes(data)
		if parseErr != nil {
			result.Failures = append(result.Failures, SheetFailure{Path: path, Err: parseErr})
			continue
		}
		result.Sheets = append(result.Sheets, SheetFile{Path: path, Sheet: sheet})
	}
	sort.Slice(result.Sheets, func(i, j int) bool { return result.Sheets[i].Path < result.Sheets[j].Path })
	sort.Slice(result.Failures, func(i, j int) bool { return result.Failures[i].Path < result.Failures[j].Path })
	return result
}

// SameNameSheetPath returns the path of the cue that shares the audio file
// name (only the extension differs), or "" when no such file exists.
func SameNameSheetPath(dir, audioName string) string {
	path := filepath.Join(dir, strings.TrimSuffix(audioName, filepath.Ext(audioName))+".cue")
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return ""
	}
	return path
}

// MatchesAudio reports whether the sheet's FILE line points at audioName.
// Only the base name and its stem are compared, case-insensitively, so a cue
// that spells the file with different capitalisation, a different extension,
// or in simplified Chinese still binds to its traditional-Chinese audio file.
func MatchesAudio(sheet *Sheet, audioName string) bool {
	if sheet == nil {
		return false
	}
	referenced := strings.TrimSpace(sheet.File)
	if referenced == "" {
		return false
	}
	referenced = filepath.Base(filepath.FromSlash(referenced))
	if strings.EqualFold(referenced, audioName) {
		return true
	}
	return strings.EqualFold(strings.TrimSuffix(referenced, filepath.Ext(referenced)),
		strings.TrimSuffix(audioName, filepath.Ext(audioName)))
}

// ResolveSheet returns the path of the cue sheet that belongs to the audio
// file audioName inside dir.
//
// A same-name cue (only the extension differs) always wins — that is the
// conventional whole-track layout. When it is missing the FILE line inside a
// neighbouring cue is consulted instead: Windows file names are
// case-insensitive but simplified/traditional-sensitive, so a rip whose cue
// was renamed to the simplified spelling of a traditional-Chinese audio file
// can never be found by name, while the cue's FILE line still names the real
// audio file.
//
// The FILE-based lookup only applies when dir holds exactly one bindable
// whole-track audio file. A split-track rip that keeps a stray whole-track cue
// (EAC writes "noncompliant.cue" pointing at track 1) must stay a plain album
// instead of being expanded into duplicated virtual tracks.
func ResolveSheet(dir, audioName string) (string, bool) {
	if samePath := SameNameSheetPath(dir, audioName); samePath != "" {
		return samePath, true
	}
	scanned := ScanDir(dir)
	if scanned.AudioCount != 1 {
		return "", false
	}
	for _, candidate := range scanned.Sheets {
		if MatchesAudio(candidate.Sheet, audioName) {
			return candidate.Path, true
		}
	}
	return "", false
}
