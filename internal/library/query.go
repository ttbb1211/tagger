package library

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ericwyn/tagger/internal/domain"
)

const (
	DefaultTrackPageSize    = 100
	MaxTrackPageSize        = 200
	DefaultTrackResolveSize = domain.DefaultBatchTrackLimit
)

type TrackSort string

const (
	TrackSortAlbum    TrackSort = "album"
	TrackSortTitle    TrackSort = "title"
	TrackSortModified TrackSort = "modified"
	TrackSortFormat   TrackSort = "format"
	// TrackSortPath 按物理目录排序。整轨 CUE 专辑的目录名就是专辑名，
	// 用它浏览脏库时能按「一个文件夹 = 一张专辑」的直觉走，不受专辑标签是否已刮削影响。
	TrackSortPath TrackSort = "path"
)

// TrackQuery is shared by paginated browsing and bounded batch selection.
// FolderPath uses the same display path as FolderNode ("Artist · Album").
type TrackQuery struct {
	Query             string             `json:"q,omitempty"`
	FolderID          string             `json:"folderId,omitempty"`
	FolderPath        string             `json:"folderPath,omitempty"`
	IncludeSubfolders bool               `json:"includeSubfolders,omitempty"`
	Health            domain.TrackHealth `json:"health,omitempty"`
	Format            domain.TrackFormat `json:"format,omitempty"`
	Sort              TrackSort          `json:"sort,omitempty"`
}

type TrackPage struct {
	Tracks     []domain.Track `json:"tracks"`
	Total      int            `json:"total"`
	NextCursor string         `json:"nextCursor,omitempty"`
	HasMore    bool           `json:"hasMore"`
}

var (
	ErrInvalidTrackQuery   = errors.New("invalid track query")
	ErrInvalidTrackCursor  = errors.New("invalid track cursor")
	ErrStaleTrackCursor    = errors.New("stale track cursor")
	ErrTrackSelectionLarge = errors.New("track selection exceeds limit")
)

type trackCursor struct {
	Generation uint64 `json:"generation"`
	QueryHash  string `json:"queryHash"`
	Offset     int    `json:"offset"`
}

func NormalizeTrackQuery(query TrackQuery) (TrackQuery, error) {
	query.Query = strings.TrimSpace(query.Query)
	query.FolderID = strings.TrimSpace(query.FolderID)
	query.FolderPath = strings.TrimSpace(strings.Trim(query.FolderPath, "· "))
	query.Health = domain.TrackHealth(strings.TrimSpace(string(query.Health)))
	query.Format = domain.TrackFormat(strings.TrimSpace(string(query.Format)))
	query.Sort = TrackSort(strings.TrimSpace(string(query.Sort)))
	if query.Sort == "" {
		query.Sort = TrackSortAlbum
	}
	switch query.Sort {
	case TrackSortAlbum, TrackSortTitle, TrackSortModified, TrackSortFormat, TrackSortPath:
	default:
		return TrackQuery{}, fmt.Errorf("%w: unsupported sort %q", ErrInvalidTrackQuery, query.Sort)
	}
	if query.Health != "" {
		switch query.Health {
		case domain.HealthComplete, domain.HealthTagCompatibility, domain.HealthMissingArtwork, domain.HealthMissingLyrics, domain.HealthNeedsReview, domain.HealthParseError, domain.HealthMissing:
		default:
			return TrackQuery{}, fmt.Errorf("%w: unsupported health %q", ErrInvalidTrackQuery, query.Health)
		}
	}
	if query.Format != "" {
		if !query.Format.IsSupported() {
			return TrackQuery{}, fmt.Errorf("%w: unsupported format %q", ErrInvalidTrackQuery, query.Format)
		}
	}
	return query, nil
}

func queryHash(query TrackQuery) string {
	encoded, _ := json.Marshal(query)
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:])
}

func encodeTrackCursor(cursor trackCursor) string {
	encoded, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func decodeTrackCursor(value string) (trackCursor, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return trackCursor{}, fmt.Errorf("%w: base64", ErrInvalidTrackCursor)
	}
	var cursor trackCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil || cursor.Generation == 0 || cursor.QueryHash == "" || cursor.Offset < 0 {
		return trackCursor{}, fmt.Errorf("%w: payload", ErrInvalidTrackCursor)
	}
	return cursor, nil
}

func normalizePageSize(limit int) (int, error) {
	if limit == 0 {
		return DefaultTrackPageSize, nil
	}
	if limit < 1 || limit > MaxTrackPageSize {
		return 0, fmt.Errorf("%w: page size must be between 1 and %d", ErrInvalidTrackQuery, MaxTrackPageSize)
	}
	return limit, nil
}

// trackMatchesFilters 只做结构筛选（缺失/目录/健康度/格式）。
// 文本搜索由 searchNeedle.matches 基于预计算的搜索键完成，见 searchindex.go。
func trackMatchesFilters(track domain.Track, query TrackQuery) bool {
	if query.Health == domain.HealthMissing {
		if !track.Missing {
			return false
		}
	} else if track.Missing {
		return false
	}
	if query.FolderID != "" && track.FolderID != query.FolderID {
		return false
	}
	if query.FolderPath != "" {
		directory := filepath.ToSlash(filepath.Dir(track.RelativePath))
		folderPath := strings.ReplaceAll(query.FolderPath, " · ", "/")
		if directory != folderPath && (!query.IncludeSubfolders || !strings.HasPrefix(directory, folderPath+"/")) {
			return false
		}
	}
	if query.Health != "" && query.Health != domain.HealthMissing {
		if track.SyncState == domain.SyncDraft || track.Health != query.Health {
			return false
		}
	}
	if query.Format != "" && track.Format != query.Format {
		return false
	}
	return true
}

func compareTrack(left, right domain.Track, mode TrackSort) int {
	compareText := func(a, b string) int {
		a = strings.ToLower(strings.TrimSpace(a))
		b = strings.ToLower(strings.TrimSpace(b))
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
		return 0
	}
	comparePath := func() int { return compareText(left.RelativePath, right.RelativePath) }
	switch mode {
	case TrackSortTitle:
		if result := compareText(firstTrackText(left.Title, left.FileName), firstTrackText(right.Title, right.FileName)); result != 0 {
			return result
		}
	case TrackSortModified:
		if left.FileFingerprint.ModifiedUnixNano != right.FileFingerprint.ModifiedUnixNano {
			if left.FileFingerprint.ModifiedUnixNano > right.FileFingerprint.ModifiedUnixNano {
				return -1
			}
			return 1
		}
		if result := compareText(right.ModifiedAt, left.ModifiedAt); result != 0 {
			return result
		}
	case TrackSortFormat:
		if result := compareText(string(left.Format), string(right.Format)); result != 0 {
			return result
		}
		if result := compareText(firstTrackText(left.Title, left.FileName), firstTrackText(right.Title, right.FileName)); result != 0 {
			return result
		}
	case TrackSortPath:
		// 先按所在目录（= 专辑文件夹），再按碟号/轨号。
		// 轨号不能省：整轨 CUE 的虚拟路径是 `<音频>#cue:10`，纯字符串比较会把第 10 首排到第 2 首前面。
		if result := compareText(filepath.ToSlash(filepath.Dir(left.RelativePath)), filepath.ToSlash(filepath.Dir(right.RelativePath))); result != 0 {
			return result
		}
		if result := compareOptionalInt(left.DiscNumber, right.DiscNumber); result != 0 {
			return result
		}
		if result := compareOptionalInt(left.TrackNumber, right.TrackNumber); result != 0 {
			return result
		}
		if result := compareText(firstTrackText(left.Title, left.FileName), firstTrackText(right.Title, right.FileName)); result != 0 {
			return result
		}
	default:
		if result := compareText(left.Album, right.Album); result != 0 {
			return result
		}
		if result := compareOptionalInt(left.DiscNumber, right.DiscNumber); result != 0 {
			return result
		}
		if result := compareOptionalInt(left.TrackNumber, right.TrackNumber); result != 0 {
			return result
		}
		if result := compareText(firstTrackText(left.Title, left.FileName), firstTrackText(right.Title, right.FileName)); result != 0 {
			return result
		}
	}
	if result := comparePath(); result != 0 {
		return result
	}
	return compareText(left.ID, right.ID)
}

func firstTrackText(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return fallback
}

func compareOptionalInt(left, right *int) int {
	leftValue, rightValue := 0, 0
	if left != nil {
		leftValue = *left
	}
	if right != nil {
		rightValue = *right
	}
	if leftValue < rightValue {
		return -1
	}
	if leftValue > rightValue {
		return 1
	}
	return 0
}

func sortTracks(tracks []domain.Track, mode TrackSort) {
	sort.SliceStable(tracks, func(i, j int) bool { return compareTrack(tracks[i], tracks[j], mode) < 0 })
}
