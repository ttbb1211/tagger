package library

import (
	"testing"

	"github.com/ericwyn/tagger/internal/domain"
)

// 「目录顺序」是给脏库用的浏览方式：一个文件夹 = 一张专辑。
// 注意事项都锁在这个用例里：目录优先；同目录内靠碟号/轨号，不能靠路径字符串。
// 目录名用 ASCII —— 后端 compareText 是逐字节比较（与「专辑顺序」一致），
// 拿中文目录名做断言会把「字节序 vs 拼音序」这个无关变量混进来。
func TestSortTracksByPathGroupsFoldersAndKeepsCueTracksInOrder(t *testing.T) {
	one, two, ten := 1, 2, 10
	tracks := []domain.Track{
		// 整轨 CUE 的虚拟路径带 `#cue:N` 后缀 —— 纯字符串比较会把第 10 首排到第 2 首前面。
		{ID: "b-10", RelativePath: "Album B/disc.wav#cue:10", TrackNumber: &ten, Title: "十"},
		{ID: "a-2", RelativePath: "Album A/02.flac", TrackNumber: &two, Title: "甲二"},
		{ID: "b-2", RelativePath: "Album B/disc.wav#cue:2", TrackNumber: &two, Title: "二"},
		{ID: "a-1", RelativePath: "Album A/01.flac", TrackNumber: &one, Title: "甲一"},
	}

	sortTracks(tracks, TrackSortPath)

	got := make([]string, 0, len(tracks))
	for _, track := range tracks {
		got = append(got, track.ID)
	}
	want := []string{"a-1", "a-2", "b-2", "b-10"}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("目录顺序排序结果 = %v，期望 %v", got, want)
		}
	}
}

func TestNormalizeTrackQueryAcceptsPathSort(t *testing.T) {
	normalized, err := NormalizeTrackQuery(TrackQuery{Sort: TrackSortPath})
	if err != nil {
		t.Fatalf("path 排序应被接受，却报错：%v", err)
	}
	if normalized.Sort != TrackSortPath {
		t.Fatalf("排序模式被改写为 %q", normalized.Sort)
	}
}
