package cue

import (
	"strings"
	"testing"
)

const sample = `REM DATE 1995
REM GENRE Pop
PERFORMER "辛晓琪"
TITLE "我也会爱上别人的 CD1"
FILE "辛晓琪 - 我也会爱上别人的CD1.wav" WAVE
  TRACK 01 AUDIO
    ISRC TWAAA9500012
    INDEX 01 00:00:00
  TRACK 02 AUDIO
    TITLE "我也會愛上別人"
    PERFORMER "辛曉琪"
    INDEX 01 00:05:30
  TRACK 03 AUDIO
    TITLE "告别"
    INDEX 01 00:11:00
`

func TestParse(t *testing.T) {
	sheet, err := ParseBytes([]byte(sample))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if sheet.Title != "我也会爱上别人的 CD1" || sheet.Performer != "辛晓琪" {
		t.Fatalf("global fields: %+v", sheet)
	}
	if sheet.Rem["DATE"] != "1995" || sheet.Rem["GENRE"] != "Pop" {
		t.Fatalf("rem: %+v", sheet.Rem)
	}
	if len(sheet.Tracks) != 3 {
		t.Fatalf("tracks: %d", len(sheet.Tracks))
	}
	if sheet.Tracks[1].Title != "我也會愛上別人" || sheet.Tracks[1].Index01 != 5.5*60-30+30 {
		// 00:05:30 => 5*60+30 = 330s
		if sheet.Tracks[1].Index01 != 330 {
			t.Fatalf("track2 index: %v", sheet.Tracks[1].Index01)
		}
	}
	if sheet.Tracks[0].ISRC != "TWAAA9500012" {
		t.Fatalf("isrc: %q", sheet.Tracks[0].ISRC)
	}
	if sheet.File != "辛晓琪 - 我也会爱上别人的CD1.wav" {
		t.Fatalf("file: %q", sheet.File)
	}
}

func TestRawTags(t *testing.T) {
	sheet, _ := ParseBytes([]byte(sample))
	raw := sheet.RawTags(2)
	if raw["TITLE"][0] != "我也會愛上別人" || raw["ARTIST"][0] != "辛曉琪" {
		t.Fatalf("track tags: %+v", raw)
	}
	if raw["ALBUM"][0] != "我也会爱上别人的 CD1" || raw["TRACKNUMBER"][0] != "02" {
		t.Fatalf("album tags: %+v", raw)
	}
}

func TestApplyUpdatesTrackScope(t *testing.T) {
	newText, applied, err := ApplyUpdates([]byte(sample), 3, map[string][]string{
		"TITLE":  {"告別"},
		"ARTIST": {"辛曉琪"},
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(applied) != 2 {
		t.Fatalf("applied: %v", applied)
	}
	sheet, err := ParseBytes(newText)
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if sheet.Tracks[2].Title != "告別" {
		t.Fatalf("track3 title: %q", sheet.Tracks[2].Title)
	}
	if sheet.Tracks[2].Performer != "辛曉琪" {
		t.Fatalf("track3 performer: %q", sheet.Tracks[2].Performer)
	}
	// 其他轨不受影响
	if sheet.Tracks[1].Title != "我也會愛上別人" {
		t.Fatalf("track2 title changed: %q", sheet.Tracks[1].Title)
	}
	// 全局字段保留
	if sheet.Title != "我也会爱上别人的 CD1" || sheet.Rem["DATE"] != "1995" {
		t.Fatalf("globals lost: %+v", sheet)
	}
}

func TestApplyUpdatesGlobalAndInsertion(t *testing.T) {
	// TRACK 01 没有 TITLE/PERFORMER 行 —— 需要插入
	newText, _, err := ApplyUpdates([]byte(sample), 1, map[string][]string{
		"TITLE":  {"我也會愛上別人的 (Intro)"},
		"ALBUM":  {"我也會愛上別人的"},
		"GENRE":  {"Mandopop"},
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	sheet, err := ParseBytes(newText)
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if sheet.Tracks[0].Title != "我也會愛上別人的 (Intro)" {
		t.Fatalf("track1 title: %q", sheet.Tracks[0].Title)
	}
	if sheet.Title != "我也會愛上別人的" {
		t.Fatalf("album: %q", sheet.Title)
	}
	if sheet.Rem["GENRE"] != "Mandopop" {
		t.Fatalf("genre: %q", sheet.Rem["GENRE"])
	}
	if !strings.Contains(newText, "FILE ") {
		t.Fatalf("FILE line lost")
	}
}

func TestGBKRoundTrip(t *testing.T) {
	gbk := encodeText(sample, "gbk")
	if utf8Valid(gbk) {
		t.Fatalf("expected non-utf8 gbk bytes")
	}
	sheet, err := ParseBytes(gbk)
	if err != nil {
		t.Fatalf("parse gbk: %v", err)
	}
	if sheet.Encoding != "gbk" || sheet.Title != "我也会爱上别人的 CD1" {
		t.Fatalf("gbk sheet: %q / %q", sheet.Encoding, sheet.Title)
	}
	newText, _, err := ApplyUpdates(gbk, 2, map[string][]string{"TITLE": {"我也會愛上別人 (修訂)"}})
	if err != nil {
		t.Fatalf("apply gbk: %v", err)
	}
	if utf8Valid(newText) {
		t.Fatalf("expected gbk output")
	}
	sheet, err = ParseBytes(newText)
	if err != nil {
		t.Fatalf("reparse gbk: %v", err)
	}
	if sheet.Tracks[1].Title != "我也會愛上別人 (修訂)" {
		t.Fatalf("gbk round trip: %q", sheet.Tracks[1].Title)
	}
}

func utf8Valid(b []byte) bool {
	for _, r := range string(b) {
		if r == '\uFFFD' {
			return false
		}
	}
	return true
}
