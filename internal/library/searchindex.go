package library

import (
	"strings"

	"github.com/ericwyn/tagger/internal/domain"
	"github.com/ericwyn/tagger/internal/textconv"
)

// 曲库搜索的简繁互认。
//
// 背景：曲库里同一首歌的标签/文件名经常简繁混用（齐秦 vs 齊秦、张国荣 vs 張國榮），
// 而 Windows 文件名对大小写不敏感、对简繁敏感，所以两套写法长期共存。
// 只做 strings.Contains 时，「齐秦」查不到「齊秦」，结果集完全不相交。
//
// 做法：给每条曲目预计算一份「搜索键」，键里同时保留原文小写和简体归一化文本，
// 查询词同样出两种形态，任一形态命中即算命中。于是简体查繁体、繁体查简体都成立。
//
// 预计算放在曲库索引重建时（见 Service.apply / applyTrack），
// 实测 4618 条 ≈ 390ms，只在扫描/启动时发生，不进入每次请求的搜索路径。

// trackSearchValues 列出参与曲库搜索的文本字段。
// 这是搜索字段的唯一清单：搜索键与结构筛选都从这里取值，避免两边字段漂移。
func trackSearchValues(track domain.Track) []string {
	values := []string{track.Title, track.FileName, track.RelativePath, track.Album}
	values = append(values, track.Artists...)
	values = append(values, track.AlbumArtists...)
	values = append(values, track.Genres...)
	for _, hint := range track.TagHints {
		values = append(values, hint.Title, hint.Album)
		values = append(values, hint.Artists...)
		values = append(values, hint.AlbumArtists...)
	}
	return values
}

// searchKeySeparator 用于把各字段拼成一条 haystack。
// 查询词在 NormalizeTrackQuery 里已 trim，正常不含换行，
// 因此对拼接结果做 Contains 与「逐字段 Contains 取或」等价。
const searchKeySeparator = "\n"

// buildSearchKey 生成一条曲目的搜索键：小写原文，若含繁体再追加一段小写简体。
//
// 为什么不只留简体版本：OpenCC 是按词转换的（乾隆→乾隆、乾淨→干净），
// 只留归一化结果会让单字查询退化——用户输入「乾」时，原文里的「乾隆」
// 在简体文本里仍是「乾隆」，两边对不上。保留原文这一份可以保证
// 「原文精确匹配」这条老路径永远不回归。
func buildSearchKey(track domain.Track) string {
	raw := strings.ToLower(strings.Join(trackSearchValues(track), searchKeySeparator))
	simplified := strings.ToLower(textconv.Simplify(raw))
	if simplified == raw {
		return raw
	}
	return raw + searchKeySeparator + simplified
}

// searchNeedle 是查询词的两种形态：小写原文与小写简体，与搜索键对称。
type searchNeedle struct {
	raw        string
	simplified string
}

func newSearchNeedle(query string) searchNeedle {
	raw := strings.ToLower(query)
	return searchNeedle{raw: raw, simplified: textconv.Simplify(raw)}
}

func (n searchNeedle) empty() bool { return n.raw == "" }

func (n searchNeedle) matches(searchKey string) bool {
	if strings.Contains(searchKey, n.raw) {
		return true
	}
	if n.simplified == n.raw {
		return false
	}
	return strings.Contains(searchKey, n.simplified)
}
