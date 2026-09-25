// Package cue parses and edits CUE sheet files for whole-track albums.
//
// Tagger keeps the audio file untouched for cue albums: per-track metadata
// lives in the cue sheet itself, so editing a virtual track rewrites only
// the small text file. Non-UTF-8 cue files (GBK is common for Chinese
// rips) are decoded on parse and re-encoded with the same encoding on save.
package cue

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// Track is one TRACK entry of a cue sheet.
type Track struct {
	Number    int
	Title     string
	Performer string
	ISRC      string
	Index01   float64 // seconds from the start of the audio file
}

// Sheet is a parsed cue sheet. Line indexes anchor edits back onto the
// original text so unrelated lines are preserved byte for byte.
type Sheet struct {
	Rem        map[string]string // uppercase key -> value (DATE, GENRE, COMMENT, DISCID, ...)
	Performer  string
	Title      string
	File       string // audio filename from the FILE line
	Tracks     []Track
	Encoding   string // "utf8" or "gbk"; used to encode on save
	lines      []string
	titleLine      int // global TITLE line, -1 when absent
	performerLine  int // global PERFORMER line, -1 when absent
	remLines       map[string]int
	firstTrackLine int
}

// ParseBytes decodes raw cue bytes (UTF-8 or GBK) into a Sheet.
func ParseBytes(data []byte) (*Sheet, error) {
	encoding := "utf8"
	if !utf8.Valid(data) {
		decoded, err := simplifiedchinese.GBK.NewDecoder().Bytes(data)
		if err != nil {
			return nil, fmt.Errorf("decode cue: %w", err)
		}
		data = decoded
		encoding = "gbk"
	}
	text := strings.TrimPrefix(string(data), "\uFEFF")
	sheet := &Sheet{
		Rem:            map[string]string{},
		Encoding:       encoding,
		titleLine:      -1,
		performerLine:  -1,
		remLines:       map[string]int{},
		firstTrackLine: -1,
	}
	sheet.lines = strings.Split(text, "\n")

	var current *Track
	for i, raw := range sheet.lines {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" {
			continue
		}
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "REM "):
			key, value, found := strings.Cut(strings.TrimSpace(line[4:]), " ")
			if !found {
				continue
			}
			key = strings.ToUpper(strings.TrimSpace(key))
			value = unquote(strings.TrimSpace(value))
			if _, exists := sheet.Rem[key]; !exists {
				sheet.Rem[key] = value
				sheet.remLines[key] = i
			}
		case strings.HasPrefix(upper, "PERFORMER "):
			value := unquote(strings.TrimSpace(line[len("PERFORMER "):]))
			if current != nil {
				current.Performer = value
			} else if sheet.Performer == "" {
				sheet.Performer = value
				if sheet.performerLine < 0 {
					sheet.performerLine = i
				}
			}
		case strings.HasPrefix(upper, "TITLE "):
			value := unquote(strings.TrimSpace(line[len("TITLE "):]))
			if current != nil {
				current.Title = value
			} else {
				sheet.Title = value
				if sheet.titleLine < 0 {
					sheet.titleLine = i
				}
			}
		case strings.HasPrefix(upper, "FILE "):
			name, _, _ := strings.Cut(strings.TrimSpace(line[len("FILE "):]), " ")
			if sheet.File == "" {
				sheet.File = unquote(name)
			}
		case strings.HasPrefix(upper, "TRACK "):
			fields := strings.Fields(strings.TrimSpace(line[len("TRACK "):]))
			if len(fields) == 0 {
				continue
			}
			number, err := strconv.Atoi(strings.TrimLeft(fields[0], "0"))
			if err != nil {
				continue
			}
			sheet.Tracks = append(sheet.Tracks, Track{Number: number})
			current = &sheet.Tracks[len(sheet.Tracks)-1]
			if sheet.firstTrackLine < 0 {
				sheet.firstTrackLine = i
			}
		case strings.HasPrefix(upper, "ISRC "):
			if current != nil {
				current.ISRC = unquote(strings.TrimSpace(line[len("ISRC "):]))
			}
		case strings.HasPrefix(upper, "INDEX "):
			fields := strings.Fields(strings.TrimSpace(line[len("INDEX "):]))
			if len(fields) != 2 || fields[0] != "01" || current == nil {
				continue
			}
			current.Index01 = indexToSeconds(fields[1])
		}
	}
	if len(sheet.Tracks) == 0 {
		return nil, fmt.Errorf("cue 文件中没有 TRACK 条目")
	}
	return sheet, nil
}

func indexToSeconds(field string) float64 {
	parts := strings.Split(field, ":")
	if len(parts) != 3 {
		return 0
	}
	minutes, err1 := strconv.Atoi(parts[0])
	seconds, err2 := strconv.Atoi(parts[1])
	frames, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return 0
	}
	return float64(minutes*60+seconds) + float64(frames)/75
}

func unquote(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
		return value[1 : len(value)-1]
	}
	return value
}

// Quote renders a cue string value with the conventional double quotes.
func Quote(value string) string { return "\"" + strings.ReplaceAll(value, "\"", "'") + "\"" }

// RawTags converts one track (with album-level context) into the standard
// raw tag map used by the write pipeline. Keys follow the same vocabulary as
// embedded tags so compilePatch can be reused verbatim.
func (s *Sheet) RawTags(trackNumber int) map[string][]string {
	raw := map[string][]string{}
	if s.Title != "" {
		raw["ALBUM"] = []string{s.Title}
	}
	if s.Performer != "" {
		raw["ALBUMARTIST"] = []string{s.Performer}
	}
	if date := s.Rem["DATE"]; date != "" {
		raw["DATE"] = []string{date}
	}
	if genre := s.Rem["GENRE"]; genre != "" {
		raw["GENRE"] = []string{genre}
	}
	if comment := s.Rem["COMMENT"]; comment != "" {
		raw["COMMENT"] = []string{comment}
	}
	track := s.track(trackNumber)
	if track == nil {
		return raw
	}
	if track.Title != "" {
		raw["TITLE"] = []string{track.Title}
	}
	if track.Performer != "" {
		raw["ARTIST"] = []string{track.Performer}
	} else if s.Performer != "" {
		raw["ARTIST"] = []string{s.Performer}
	}
	if track.ISRC != "" {
		raw["ISRC"] = []string{track.ISRC}
	}
	raw["TRACKNUMBER"] = []string{fmt.Sprintf("%02d", track.Number)}
	return raw
}

func (s *Sheet) track(trackNumber int) *Track {
	for i := range s.Tracks {
		if s.Tracks[i].Number == trackNumber {
			return &s.Tracks[i]
		}
	}
	return nil
}

// cueWritableFields lists standard tag keys this package can persist into a
// cue sheet; anything else in an update map is reported back as unapplied.
var cueWritableFields = map[string]bool{
	"ALBUM": true, "ALBUMARTIST": true, "DATE": true, "GENRE": true,
	"COMMENT": true, "TITLE": true, "ARTIST": true, "ISRC": true,
}

// SupportedFields reports which update keys can be persisted into the cue.
func SupportedFields(keys []string) (applied, skipped []string) {
	for _, key := range keys {
		if cueWritableFields[strings.ToUpper(key)] {
			applied = append(applied, key)
		} else {
			skipped = append(skipped, key)
		}
	}
	return applied, skipped
}

// ApplyUpdates rewrites the cue text, applying standard-tag updates to the
// given track (and album-level fields globally). It returns the new text and
// the uppercase keys that were actually applied.
func ApplyUpdates(data []byte, trackNumber int, updates map[string][]string) (newData []byte, applied []string, err error) {
	sheet, err := ParseBytes(data)
	if err != nil {
		return nil, nil, err
	}
	text := strings.TrimPrefix(string(data), "\uFEFF")
	for _, key := range sortedKeys(updates) {
		if !cueWritableFields[key] {
			continue
		}
		values := updates[key]
		if len(values) == 0 || strings.TrimSpace(values[0]) == "" {
			continue // cue 不支持清空字段，跳过
		}
		switch key {
		case "ALBUM":
			text = setText(text, "TITLE", Quote(values[0]), true)
		case "ALBUMARTIST":
			text = setText(text, "PERFORMER", Quote(values[0]), true)
		case "DATE":
			text = setRem(text, "DATE", values[0])
		case "GENRE":
			text = setRem(text, "GENRE", values[0])
		case "COMMENT":
			text = setRem(text, "COMMENT", values[0])
		case "TITLE":
			text = setText(text, "TITLE", Quote(values[0]), false, trackNumber)
		case "ARTIST":
			text = setText(text, "PERFORMER", Quote(strings.Join(values, "; ")), false, trackNumber)
		case "ISRC":
			text = setText(text, "ISRC", values[0], false, trackNumber)
		}
		applied = append(applied, key)
	}
	return encodeText(text, sheet.Encoding), applied, nil
}

func sortedKeys(updates map[string][]string) []string {
	keys := make([]string, 0, len(updates))
	for key := range updates {
		keys = append(keys, strings.ToUpper(key))
	}
	// 稳定顺序，便于测试与幂等
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

// setText rewrites (or inserts) a `KEY "value"` line. Global scope edits the
// first top-level occurrence; track scope edits the given TRACK block.
func setText(text, key, value string, global bool, trackNumber ...int) string {
	lines := strings.Split(text, "\n")
	number := 0
	if len(trackNumber) > 0 {
		number = trackNumber[0]
	}
	blockStart, blockEnd := trackBlockBounds(lines, number)
	scopeStart, scopeEnd := 0, len(lines)
	if !global {
		scopeStart, scopeEnd = blockStart, blockEnd
	}
	prefix := key + " "
	for i := scopeStart; i < scopeEnd; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(strings.ToUpper(trimmed), prefix) {
			lines[i] = key + " " + value
			return strings.Join(lines, "\n")
		}
	}
	if global {
		// 插到第一条 TRACK 之前（或文件末尾）
		at := len(lines)
		for i, line := range lines {
			if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(line)), "TRACK ") {
				at = i
				break
			}
		}
		return strings.Join(append(lines[:at:at], key+" "+value, joinRest(lines[at:])), "\n")
	}
	// 轨道块内：插到 TRACK 行之后
	at := blockEnd
	for i := blockStart; i < blockEnd; i++ {
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(lines[i])), "TRACK ") {
			at = i + 1
			break
		}
	}
	rest := append([]string{key + " " + value}, lines[at:]...)
	return strings.Join(append(lines[:at:at], rest...), "\n")
}

func joinRest(lines []string) string { return strings.Join(lines, "\n") }

func setRem(text, key, value string) string {
	lines := strings.Split(text, "\n")
	prefix := "REM " + key + " "
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(trimmed), prefix) {
			lines[i] = "REM " + key + " " + value
			return strings.Join(lines, "\n")
		}
	}
	at := len(lines)
	for i, line := range lines {
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(line)), "TRACK ") {
			at = i
			break
		}
	}
	rest := append([]string{"REM " + key + " " + value}, lines[at:]...)
	return strings.Join(append(lines[:at:at], rest...), "\n")
}

func trackBlockBounds(lines []string, number int) (start, end int) {
	startIdx := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToUpper(trimmed), "TRACK ") {
			continue
		}
		fields := strings.Fields(strings.TrimSpace(trimmed[len("TRACK "):]))
		if len(fields) == 0 {
			continue
		}
		value, err := strconv.Atoi(strings.TrimLeft(fields[0], "0"))
		if err != nil || value != number {
			continue
		}
		startIdx = i
		break
	}
	if startIdx < 0 {
		return 0, len(lines)
	}
	end = len(lines)
	for i := startIdx + 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(lines[i])), "TRACK ") ||
			strings.HasPrefix(strings.ToUpper(strings.TrimSpace(lines[i])), "FILE ") {
			end = i
			break
		}
	}
	return startIdx, end
}

func encodeText(text, encoding string) []byte {
	if encoding == "gbk" {
		encoded, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(text))
		if err == nil {
			return encoded
		}
	}
	return []byte(text)
}
