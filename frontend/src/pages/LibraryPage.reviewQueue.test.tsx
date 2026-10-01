import {render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {seedTracks} from '@/mock/data';
import type {Job, LibrarySummary, Track} from '@/types';

// 只在这个文件里把 apiReadMode 设成 real：队列查询只在真实模式下发生，
// 放在公共测试文件里会污染其他用例（这正是 2026-09-29 把 listJobs 去掉的原因）。
const api = vi.hoisted(() => ({
  apiReadMode: 'real' as const,
  listLibraries: vi.fn(),
  listTrackPage: vi.fn(),
  resolveTracks: vi.fn(),
  getSystem: vi.fn(),
  listJobs: vi.fn(),
}));

vi.mock('@/api', async () => ({
  ...(await vi.importActual<typeof import('@/api')>('@/api')),
  ...api,
}));

import {LibraryPage, isPendingMatchJob, pendingMatchJobsLabel} from '@/pages/LibraryPage';
import {clearLibraryViewSnapshots} from '@/pages/libraryViewCache';

const library: LibrarySummary = {
  id: 'lib-queue', name: 'QueueLib', rootLabel: 'QueueLib', rootPath: '/music/queue', active: true,
  trackCount: 2, folderCount: 1, writable: true, lastScanLabel: '刚刚', folders: [],
};

const first: Track = {...seedTracks[0], id: 'trk-a', relativePath: 'a.flac', fileName: 'a.flac', folderId: 'folder-root', title: '第一首'};
const second: Track = {...seedTracks[1], id: 'trk-b', relativePath: 'b.flac', fileName: 'b.flac', folderId: 'folder-root', title: '第二首'};

function job(overrides: Partial<Job>): Job {
  return {
    id: 'job-1', kind: 'match', title: '抓取元数据', detail: '进行中', state: 'running',
    processed: 0, total: 10, succeeded: 0, failed: 0, startedAt: '刚刚', ...overrides,
  };
}

function renderPage() {
  return render(
    <LibraryPage onOpenReview={vi.fn()} onOpenSettings={vi.fn()} onNotice={vi.fn()} playerPlaying={false} onPlayTrack={vi.fn()} onTogglePlayer={vi.fn()} />,
  );
}

describe('批量补全的重复任务提醒', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    clearLibraryViewSnapshots();
    api.getSystem.mockResolvedValue({batchTrackLimit: 2000});
    api.listLibraries.mockResolvedValue([library]);
    api.listTrackPage.mockResolvedValue({tracks: [first, second], total: 2, hasMore: false});
    api.resolveTracks.mockResolvedValue({tracks: [first, second], total: 2});
    api.listJobs.mockResolvedValue([]);
  });

  it('队列里已有未完成的抓取任务时，确认框给出提醒并写明数量构成', async () => {
    const user = userEvent.setup();
    api.listJobs.mockResolvedValue([
      job({id: 'job-run', state: 'running'}),
      job({id: 'job-wait', state: 'waiting'}),
      job({id: 'job-review', state: 'review'}),
      job({id: 'job-done', state: 'succeeded'}),
      job({id: 'job-write', state: 'running', kind: 'write'}),
    ]);
    renderPage();

    await screen.findByText('第一首');
    await user.click(screen.getAllByRole('button', {name: /批量补全/})[0]);

    // 3 条同类未终态任务（进行中/排队/待审核），已完成的与写入任务都不算
    expect(await screen.findByText(/队列里已有 3 个抓取任务（2 个排队\/进行中、1 个待审核）/)).toBeInTheDocument();
  });

  it('队列干净时不出现提醒', async () => {
    const user = userEvent.setup();
    api.listJobs.mockResolvedValue([job({id: 'job-done', state: 'succeeded'})]);
    renderPage();

    await screen.findByText('第一首');
    await user.click(screen.getAllByRole('button', {name: /批量补全/})[0]);

    await screen.findByText(/本次将对 2 首曲目发起元数据抓取/);
    await waitFor(() => expect(document.querySelector('.confirm-dialog-warning')).toBeNull());
  });

  it('队列查询失败不能挡住抓取（只是没有提醒）', async () => {
    const user = userEvent.setup();
    api.listJobs.mockRejectedValue(new Error('boom'));
    renderPage();

    await screen.findByText('第一首');
    await user.click(screen.getAllByRole('button', {name: /批量补全/})[0]);

    expect(await screen.findByText(/本次将对 2 首曲目发起元数据抓取/)).toBeInTheDocument();
    expect(document.querySelector('.confirm-dialog-warning')).toBeNull();
  });

  it('判定与文案：只有 match 的未终态任务算数', () => {
    expect(isPendingMatchJob({kind: 'match', state: 'running'})).toBe(true);
    expect(isPendingMatchJob({kind: 'match', state: 'waiting'})).toBe(true);
    expect(isPendingMatchJob({kind: 'match', state: 'review'})).toBe(true);
    expect(isPendingMatchJob({kind: 'match', state: 'succeeded'})).toBe(false);
    expect(isPendingMatchJob({kind: 'match', state: 'cancelled'})).toBe(false);
    expect(isPendingMatchJob({kind: 'write', state: 'running'})).toBe(false);

    expect(pendingMatchJobsLabel([{state: 'running'}, {state: 'waiting'}, {state: 'review'}])).toBe('2 个排队/进行中、1 个待审核');
    expect(pendingMatchJobsLabel([{state: 'review'}])).toBe('1 个待审核');
  });
});
