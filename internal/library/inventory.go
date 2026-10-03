package library

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ericwyn/tagger/internal/domain"
	"github.com/ericwyn/tagger/internal/scanner"
)

type ReconcileResult struct {
	Generation uint64   `json:"generation"`
	Changed    bool     `json:"changed"`
	Pending    []string `json:"pending,omitempty"`
	Missing    int      `json:"missing"`
	// Restored 是本次对账纠正回来的「曾被误标缺失」的整轨虚拟轨道条数。
	Restored int      `json:"restored,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// ReconcileDirectory refreshes the current directory and its direct children.
// It never reads embedded tags. New or changed files are exposed immediately as
// drafts and returned in Pending for asynchronous targeted metadata scanning.
func (s *Service) ReconcileDirectory(ctx context.Context, folderPath string) (ReconcileResult, error) {
	return s.reconcileInventory(ctx, []string{normalizeFolderPath(folderPath)}, 1)
}

// ReconcileTargets is the fsnotify path: event targets are already narrow, so
// walking their complete subtree safely handles pre-populated directory moves.
func (s *Service) ReconcileTargets(ctx context.Context, targets []string) (ReconcileResult, error) {
	if len(targets) == 0 {
		targets = []string{"."}
	}
	for index := range targets {
		targets[index] = normalizeFolderPath(targets[index])
	}
	return s.reconcileInventory(ctx, targets, -1)
}

func (s *Service) reconcileInventory(ctx context.Context, targets []string, maxDepth int) (ReconcileResult, error) {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()

	currentScanner := s.currentScanner()
	files, warnings, err := currentScanner.DiscoverFiles(ctx, targets, maxDepth)
	if err != nil {
		return ReconcileResult{Warnings: warnings}, err
	}
	s.mu.RLock()
	existing := cloneTracks(s.tracks)
	librarySummary := cloneLibrary(s.library)
	report := s.report
	s.mu.RUnlock()

	byPath := make(map[string]int, len(existing))
	for index, track := range existing {
		byPath[track.RelativePath] = index
	}
	// 整轨母盘（wav + cue）在索引里只有 #cue:N 虚拟轨道，没有父音频记录。
	// 先按索引算出这批母盘，避免对每个普通音频都去探测一次 cue 文件。
	masters := cueMasterPaths(existing)
	seen := make(map[string]struct{}, len(files))
	changed := make([]domain.Track, 0)
	pendingSet := make(map[string]struct{})
	for _, file := range files {
		seen[file.RelativePath] = struct{}{}
		index, found := byPath[file.RelativePath]
		// 整轨母盘：磁盘上确实有它，但扫描器只会把它展开成 #cue:N 虚拟轨道，
		// 索引里不该有父音频记录。它既不是「新文件」（草稿化），也不能因为历史
		// 遗留的父音频记录而被「复活」——两条路都会入队一次目标扫描，扫描又把
		// 母盘展开回虚拟轨道，于是每次对账（窗口 focus / visibilitychange 都会
		// 触发）都空转一轮重扫。
		if _, isMaster := masters[file.RelativePath]; isMaster && currentScanner.CueBoundAudio(file.RelativePath) {
			if found && !existing[index].Missing {
				// 历史遗留的父音频记录（草稿或已索引）：它不该存在，压成缺失态，
				// 免得它以 0:00 的幽灵行留在列表里、或又被排队重扫。
				track := existing[index]
				track.Missing = true
				track.MissingSince = time.Now().UTC().Format(time.RFC3339)
				track.Health = domain.HealthMissing
				existing[index] = track
				changed = append(changed, track)
			}
			continue
		}
		if !found {
			track := currentScanner.DraftTrack(file)
			existing = append(existing, track)
			byPath[file.RelativePath] = len(existing) - 1
			changed = append(changed, track)
			pendingSet[file.RelativePath] = struct{}{}
			continue
		}
		track := existing[index]
		fingerprintChanged := track.FileFingerprint != file.FileFingerprint
		filesystemChanged := fingerprintChanged || track.Missing || track.SizeBytes != file.SizeBytes || track.Writable != file.Writable
		if filesystemChanged {
			track.FileName = file.FileName
			track.FolderID = file.FolderID
			track.Format = file.Format
			track.SizeBytes = file.SizeBytes
			track.ModifiedAt = file.ModifiedAt
			track.Writable = file.Writable
			track.FileFingerprint = file.FileFingerprint
			track.Missing = false
			track.MissingSince = ""
			track.SyncState = domain.SyncDraft
			track.ParseError = ""
			existing[index] = track
			changed = append(changed, track)
		}
		if track.SyncState == domain.SyncDraft {
			pendingSet[file.RelativePath] = struct{}{}
		}
	}

	missing := 0
	restored := 0
	// A partial read must never convert cached files into missing records.
	if len(warnings) == 0 {
		// 整轨虚拟轨道的存在性由父音频决定，而不是它自己的伪路径：索引里
		// 从来没有父音频记录，seen 里也只有父音频，所以伪路径永远匹配不上。
		// 不特判的话整张专辑会被判成缺失并落库（下次全扫才能补回）。
		parentPresent := make(map[string]bool, len(masters))
		for index, track := range existing {
			if !inInventoryScope(track.RelativePath, targets, maxDepth) {
				continue
			}
			if _, found := seen[track.RelativePath]; found {
				continue
			}
			if parent, _, err := domain.ParseCueVirtualPath(track.RelativePath); err == nil {
				present, known := parentPresent[parent]
				if !known {
					_, parentSeen := seen[parent]
					present = parentSeen && currentScanner.CueBoundAudio(parent)
					parentPresent[parent] = present
				}
				if present {
					// 父音频与 cue 都还在 → 这条虚拟轨道就是存在的。历史上被
					// 误标缺失的要顺手纠正回来，否则只能靠一次全量重扫恢复。
					// 整轨虚拟轨道的元数据来自 cue 而非内嵌标签，扫描器给出的
					// 健康度恒为 tag-compatibility，这里照此还原。
					if track.Missing {
						track.Missing = false
						track.MissingSince = ""
						track.Health = domain.HealthTagCompatibility
						existing[index] = track
						changed = append(changed, track)
						restored++
					}
					continue
				}
			}
			if track.Missing {
				continue
			}
			track.Missing = true
			track.MissingSince = time.Now().UTC().Format(time.RFC3339)
			track.Health = domain.HealthMissing
			existing[index] = track
			changed = append(changed, track)
			missing++
		}
	}

	pending := make([]string, 0, len(pendingSet))
	for path := range pendingSet {
		pending = append(pending, path)
	}
	sort.Strings(pending)
	if len(changed) == 0 {
		return ReconcileResult{Generation: s.EventGeneration(), Pending: pending, Missing: missing, Restored: restored, Warnings: warnings}, nil
	}
	sort.Slice(existing, func(i, j int) bool { return existing[i].RelativePath < existing[j].RelativePath })
	librarySummary.TrackCount = presentTrackCount(existing)
	librarySummary.Folders = buildFoldersForTracks(existing)
	librarySummary.FolderCount = len(librarySummary.Folders)
	result := scanner.Result{Library: librarySummary, Tracks: existing, Report: report}
	if s.repo != nil {
		if deltaRepo, ok := s.repo.(DeltaRepository); ok {
			if err := deltaRepo.SaveTrackUpdates(ctx, currentScanner.Root(), result, changed); err != nil {
				return ReconcileResult{}, fmt.Errorf("persist filesystem inventory: %w", err)
			}
		} else if err := s.repo.SaveScan(ctx, currentScanner.Root(), result); err != nil {
			return ReconcileResult{}, fmt.Errorf("persist filesystem inventory: %w", err)
		}
	}
	s.apply(result)
	paths := make([]string, 0, len(changed))
	for _, track := range changed {
		paths = append(paths, track.RelativePath)
	}
	s.publishEvent(Event{Kind: EventInventory, Paths: paths})
	return ReconcileResult{Generation: s.EventGeneration(), Changed: true, Pending: pending, Missing: missing, Restored: restored, Warnings: warnings}, nil
}

// cueMasterPaths collects the parent audio files of indexed cue virtual tracks.
// The scanner expands a whole-track album into virtual tracks only, so the
// parent audio never has a record of its own; this set identifies those masters
// from the index alone, without probing the filesystem.
func cueMasterPaths(tracks []domain.Track) map[string]struct{} {
	masters := make(map[string]struct{})
	for _, track := range tracks {
		if parent, _, err := domain.ParseCueVirtualPath(track.RelativePath); err == nil {
			masters[parent] = struct{}{}
		}
	}
	return masters
}

func normalizeFolderPath(value string) string {
	value = filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimSpace(value))))
	if value == "" || value == "/" {
		return "."
	}
	return value
}

func inInventoryScope(relativePath string, targets []string, maxDepth int) bool {
	path := filepath.ToSlash(filepath.Clean(filepath.FromSlash(relativePath)))
	for _, target := range targets {
		target = normalizeFolderPath(target)
		if target != "." && path != target && !strings.HasPrefix(path, target+"/") {
			continue
		}
		if maxDepth < 0 || path == target {
			return true
		}
		relative := path
		if target != "." {
			relative = strings.TrimPrefix(path, target+"/")
		}
		directory := filepath.ToSlash(filepath.Dir(relative))
		depth := 0
		if directory != "." && directory != "" {
			depth = len(strings.Split(directory, "/"))
		}
		if depth <= maxDepth {
			return true
		}
	}
	return false
}

func presentTrackCount(tracks []domain.Track) int {
	count := 0
	for _, track := range tracks {
		if !track.Missing {
			count++
		}
	}
	return count
}

func (s *Service) PendingPaths() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	paths := make([]string, 0)
	for _, track := range s.tracks {
		if !track.Missing && track.SyncState == domain.SyncDraft {
			paths = append(paths, track.RelativePath)
		}
	}
	sort.Strings(paths)
	return paths
}
