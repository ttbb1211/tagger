package library

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ericwyn/tagger/internal/domain"
	"github.com/ericwyn/tagger/internal/scanner"
	"github.com/ericwyn/tagger/internal/tags"
)

// scriptedSearchEngine 让用例能改「文件里读到的标签」，用来验证重扫后搜索键会跟着更新。
type scriptedSearchEngine struct{ title string }

func (engine *scriptedSearchEngine) Read(_ context.Context, path string) (tags.Snapshot, error) {
	title := engine.title
	if title == "" {
		name := filepath.Base(path)
		title = name[:len(name)-len(filepath.Ext(name))]
	}
	return tags.Snapshot{
		Raw: map[string][]string{
			"TITLE":  {title},
			"ARTIST": {"测试歌手"},
		},
		DurationSeconds: 120,
	}, nil
}

func (*scriptedSearchEngine) Write(context.Context, string, map[string][]string) error { return nil }

func (*scriptedSearchEngine) Version() string { return "scripted-search-test" }

// baselineFieldMatch 复刻改造前的匹配逻辑（逐字段 ToLower + Contains）。
// 它只用于证明「原文精确匹配」这条老路径没有回归。
func baselineFieldMatch(track domain.Track, query string) bool {
	needle := strings.ToLower(query)
	for _, value := range trackSearchValues(track) {
		if strings.Contains(strings.ToLower(value), needle) {
			return true
		}
	}
	return false
}

func rawSearchKey(track domain.Track) string {
	return strings.ToLower(strings.Join(trackSearchValues(track), searchKeySeparator))
}

func traditionalSearchTrack() domain.Track {
	return domain.Track{
		ID:           "trk-trad",
		Title:        "絲路",
		FileName:     "齊秦 - 絲路.flac",
		RelativePath: "齊秦/絲路/01 - 沉默.flac",
		Album:        "絲路",
		Artists:      []string{"齊秦"},
		AlbumArtists: []string{"齊秦"},
		Genres:       []string{"國語流行"},
		TagHints: []domain.TagHint{{
			Title:   "不讓我的眼淚陪我過夜",
			Artists: []string{"齊秦"},
		}},
	}
}

func simplifiedSearchTrack() domain.Track {
	return domain.Track{
		ID:           "trk-simp",
		Title:        "大约在冬季",
		FileName:     "齐秦 - 大约在冬季.flac",
		RelativePath: "齐秦/纪念日/01 - 大约在冬季.flac",
		Album:        "纪念日",
		Artists:      []string{"齐秦"},
	}
}

func TestBuildSearchKeyKeepsOriginalAndSimplifiedCopies(t *testing.T) {
	key := buildSearchKey(traditionalSearchTrack())
	if !strings.Contains(key, "齊秦") {
		t.Fatalf("搜索键丢了原文（繁体）写法：%q", key)
	}
	if !strings.Contains(key, "齐秦") {
		t.Fatalf("搜索键缺简体归一化写法：%q", key)
	}
	if !strings.Contains(key, "丝路") {
		t.Fatalf("搜索键里的专辑名没被归一化：%q", key)
	}
}

// 无繁体文本时不该白白多存一份，否则纯西文库内存翻倍。
func TestBuildSearchKeySkipsRedundantCopyWhenNoTraditionalText(t *testing.T) {
	ascii := domain.Track{ID: "trk-ascii", Title: "Blue Train"}
	if key := buildSearchKey(ascii); key != rawSearchKey(ascii) {
		t.Fatalf("无繁体文本时搜索键应与原文小写一致：%q vs %q", key, rawSearchKey(ascii))
	}
	traditional := traditionalSearchTrack()
	if key := buildSearchKey(traditional); len(key) <= len(rawSearchKey(traditional)) {
		t.Fatalf("含繁体文本时应追加第二份搜索键：%q", key)
	}
}

// 核心诉求：简体查询要能命中繁体曲目，繁体查询也要能命中简体曲目。
func TestSearchNeedleMatchesAcrossScripts(t *testing.T) {
	traditional := buildSearchKey(traditionalSearchTrack())
	simplified := buildSearchKey(simplifiedSearchTrack())
	ascii := buildSearchKey(domain.Track{ID: "trk-ascii", Title: "Blue Train", Album: "Blue Train"})

	cases := []struct {
		name      string
		key       string
		query     string
		wantMatch bool
	}{
		{"简体查繁体曲目", traditional, "齐秦", true},
		{"繁体查简体曲目", simplified, "齊秦", true},
		{"繁体查繁体曲目", traditional, "齊秦", true},
		{"简体查简体曲目", simplified, "齐秦", true},
		{"简体查繁体专辑", traditional, "丝路", true},
		{"繁体查繁体专辑", traditional, "絲路", true},
		{"简体查繁体目录名", traditional, "齐秦/丝路", true},
		{"繁体查繁体目录名", traditional, "齊秦/絲路", true},
		{"简体查繁体 TagHint", traditional, "不让我的眼泪", true},
		{"繁体查繁体 TagHint", traditional, "不讓我的眼淚", true},
		{"大小写不敏感仍成立", ascii, "blue train", true},
		{"大小写混写仍成立", ascii, "BLUE TRAIN", true},
		{"无关词不命中", traditional, "张学友", false},
		{"无关繁体词不命中", simplified, "張學友", false},
	}
	for _, testCase := range cases {
		got := newSearchNeedle(testCase.query).matches(testCase.key)
		if got != testCase.wantMatch {
			t.Errorf("%s：查询 %q 对搜索键 %q = %v，期望 %v", testCase.name, testCase.query, testCase.key, got, testCase.wantMatch)
		}
	}
}

// 改造不能丢掉老行为：任何以前能命中的（原文）查询，现在仍必须命中。
// 保留原文那一份搜索键就是为了这个——OpenCC 是词级转换（乾隆 不会被拆成 干隆）。
func TestSearchNeverRegressesBaselineFieldMatch(t *testing.T) {
	tracks := []domain.Track{
		traditionalSearchTrack(),
		simplifiedSearchTrack(),
		{ID: "trk-qianlong", Title: "乾隆王朝", Album: "乾隆王朝"},
		{ID: "trk-mixed", Title: "費玉清 - 萬里長城", Artists: []string{"費玉清", "Fei Yu-Ching"}},
		{ID: "trk-ascii", Title: "Blue Train", Album: "Blue Train"},
	}
	queries := []string{
		"齊秦", "齐秦", "絲路", "丝路", "不讓我的眼淚", "乾隆", "乾", "費玉清", "萬里長城",
		"blue", "BLUE", "Train", "Fei", "fei yu-ching", "齐", "齊",
	}
	for _, track := range tracks {
		key := buildSearchKey(track)
		for _, query := range queries {
			if !baselineFieldMatch(track, query) {
				continue
			}
			if !newSearchNeedle(query).matches(key) {
				t.Errorf("回归：曲目 %s 用 %q 以前能搜到，现在搜不到（key=%q）", track.ID, query, key)
			}
		}
	}
}

func newSearchTestService(t *testing.T, names ...string) (*Service, *scriptedSearchEngine) {
	t.Helper()
	root := t.TempDir()
	for _, name := range names {
		target := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	engine := &scriptedSearchEngine{}
	musicScanner, err := scanner.New(engine, scanner.Options{Root: root, Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(context.Background(), musicScanner)
	if err != nil {
		t.Fatal(err)
	}
	return service, engine
}

// 搜索键包含相对路径，所以目录名里的繁体同样会被简体查询关联上。
func TestListTrackPageMatchesSimplifiedQueryAgainstTraditionalLibrary(t *testing.T) {
	service, _ := newSearchTestService(t,
		"齊秦/絲路/01.flac",
		"齊秦/絲路/02.flac",
		"張學友/吻別/01.flac",
		"Blue Train/01.flac",
	)

	page, err := service.ListTrackPage(TrackQuery{Query: "齐秦"}, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 {
		t.Fatalf("简体「齐秦」命中 %d 条，期望 2（繁体目录名应被关联）", page.Total)
	}
	traditionalPage, err := service.ListTrackPage(TrackQuery{Query: "齊秦"}, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if traditionalPage.Total != page.Total {
		t.Fatalf("繁体「齊秦」命中 %d 条，与简体「齐秦」的 %d 条不一致", traditionalPage.Total, page.Total)
	}
	traditionalOnly, err := service.ListTrackPage(TrackQuery{Query: "張學友"}, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	simplifiedOnly, err := service.ListTrackPage(TrackQuery{Query: "张学友"}, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if traditionalOnly.Total != 1 || simplifiedOnly.Total != 1 {
		t.Fatalf("「張學友」/「张学友」命中 %d/%d 条，期望各 1 条", traditionalOnly.Total, simplifiedOnly.Total)
	}
	ascii, err := service.ListTrackPage(TrackQuery{Query: "blue"}, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if ascii.Total != 1 {
		t.Fatalf("ASCII 搜索命中 %d 条，期望 1 条", ascii.Total)
	}
}

func TestListTracksLegacySearchAlsoMatchesAcrossScripts(t *testing.T) {
	service, _ := newSearchTestService(t, "齊秦/絲路/01.flac")
	if got := service.ListTracks(TrackFilter{Query: "齐秦"}); len(got) != 1 {
		t.Fatalf("ListTracks 简体查繁体命中 %d 条，期望 1 条", len(got))
	}
	if got := service.ListTracks(TrackFilter{Query: "絲路"}); len(got) != 1 {
		t.Fatalf("ListTracks 繁体查繁体命中 %d 条，期望 1 条", len(got))
	}
}

// 重扫单曲后搜索键必须跟着更新，否则刚改完标签就搜不到。
func TestRescanTrackRefreshesSearchKey(t *testing.T) {
	service, engine := newSearchTestService(t, "song.flac")
	tracks := service.ListTracks(TrackFilter{})
	if len(tracks) != 1 {
		t.Fatalf("初始曲目数 = %d", len(tracks))
	}
	engine.title = "齊秦 - 絲路"
	if _, err := service.RescanTrack(context.Background(), tracks[0].ID); err != nil {
		t.Fatal(err)
	}
	if got := service.ListTracks(TrackFilter{Query: "齐秦"}); len(got) != 1 {
		t.Fatalf("重扫后简体查繁体命中 %d 条，期望 1 条（搜索键未刷新）", len(got))
	}
	if got := service.ListTracks(TrackFilter{Query: "齐秦 - 丝路"}); len(got) != 1 {
		t.Fatalf("重扫后整串简体查询命中 %d 条，期望 1 条", len(got))
	}
}

func TestRemoveMissingDropsSearchKeys(t *testing.T) {
	service, _ := newSearchTestService(t,
		"齊秦/絲路/01.flac",
		"張學友/吻別/01.flac",
	)
	if err := os.Remove(filepath.Join(service.Root(), "張學友", "吻別", "01.flac")); err != nil {
		t.Fatal(err)
	}
	if err := service.Rescan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if removed := service.RemoveMissing(); removed != 1 {
		t.Fatalf("RemoveMissing 移除 %d 条，期望 1 条", removed)
	}
	if got := service.ListTracks(TrackFilter{Query: "张学友"}); len(got) != 0 {
		t.Fatalf("已移除曲目仍能被搜到：%#v", got)
	}
	if got := service.ListTracks(TrackFilter{Query: "齐秦"}); len(got) != 1 {
		t.Fatalf("保留曲目搜索异常，命中 %d 条", len(got))
	}
	service.mu.RLock()
	keyCount := len(service.searchKeys)
	service.mu.RUnlock()
	if keyCount != 1 {
		t.Fatalf("RemoveMissing 后搜索键剩 %d 条，期望 1 条", keyCount)
	}
}
