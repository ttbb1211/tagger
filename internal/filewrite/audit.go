package filewrite

import "github.com/ericwyn/tagger/internal/domain"

// SidecarOperation 描述一次 sidecar 写入的语义：目标存在即「写入」，否则「删除」。
func SidecarOperation(result SidecarResult) domain.Operation {
	if result.After != nil && result.After.Exists {
		return domain.OperationSet
	}
	return domain.OperationDelete
}

// SidecarDiff 把 sidecar 写入结果转成一条可放进「历史」页的差异记录。
func SidecarDiff(result SidecarResult) domain.RevisionDiff {
	return domain.RevisionDiff{
		Field:     "lyricsSidecar",
		Operation: SidecarOperation(result),
		Before:    SidecarAuditValue(result.Before),
		After:     SidecarAuditValue(result.After),
	}
}

// SidecarAuditValue 输出适合放进差异记录的 sidecar 摘要（不含歌词正文）。
func SidecarAuditValue(info *domain.SidecarInfo) any {
	if info == nil || !info.Exists {
		return nil
	}
	return map[string]any{
		"exists":     info.Exists,
		"revision":   info.Revision,
		"sizeBytes":  info.SizeBytes,
		"modifiedAt": info.ModifiedAt,
	}
}

// SidecarAuditSnapshot 捕获 sidecar 快照（含正文），供历史修订恢复使用。
func SidecarAuditSnapshot(info *domain.SidecarInfo, content string) *domain.SidecarSnapshot {
	if info == nil || !info.Exists {
		return nil
	}
	return &domain.SidecarSnapshot{
		Exists: info.Exists, Revision: info.Revision, SizeBytes: info.SizeBytes,
		ModifiedAt: info.ModifiedAt, Content: content,
	}
}

// SidecarResultSnapshot 从写入结果里取 before/after 快照。
func SidecarResultSnapshot(result *SidecarResult, before bool) *domain.SidecarSnapshot {
	if result == nil {
		return nil
	}
	if before {
		return SidecarAuditSnapshot(result.Before, result.BeforeContent)
	}
	return SidecarAuditSnapshot(result.After, result.AfterContent)
}
