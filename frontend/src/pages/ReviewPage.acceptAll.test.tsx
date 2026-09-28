import {render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {candidatesFor, seedTracks} from '@/mock/data';
import type {MatchItem, Track} from '@/types';

const api = vi.hoisted(() => ({
  resolveTracks: vi.fn(),
  getJob: vi.fn(),
  listMatchItems: vi.fn(),
  updateMatchItem: vi.fn(),
}));

vi.mock('@/api', async () => ({
  ...(await vi.importActual<typeof import('@/api')>('@/api')),
  apiReadMode: 'real',
  ...api,
}));

import {ReviewPage} from '@/pages/ReviewPage';

// 老板 2026-09-28 的场景：智能推荐候选的 autoAccept=false（中文老歌很少同时满足
// 「≥2 个独立来源 + 领先第二名 0.08 分 + 无冲突」），后端因此把状态留成 review。
// 旧实现按 autoAccept 筛，集合恒为空 → 按钮点下去毫无反应。
function reviewItem(track: Track, state: MatchItem['state']): MatchItem {
  const candidates = candidatesFor(track).map((candidate) => candidate.kind === 'smart' ? {...candidate, autoAccept: false} : candidate);
  return {
    id: `item-${track.id}`, jobId: 'job-1', trackId: track.id, state, candidates,
    selectedCandidateId: candidates[0]?.id,
  };
}

const tracks = [seedTracks[0], seedTracks[1], seedTracks[2]];

describe('ReviewPage 接受所有自动推荐', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.resolveTracks.mockResolvedValue({tracks, total: tracks.length});
    api.getJob.mockResolvedValue({
      id: 'job-1', kind: 'match', state: 'review', title: '批量抓取元数据', detail: '已完成',
      processed: tracks.length, total: tracks.length, succeeded: tracks.length, failed: 0, startedAt: '刚刚',
    });
    api.listMatchItems.mockResolvedValue([
      reviewItem(tracks[0], 'review'),
      reviewItem(tracks[1], 'accepted'),
      reviewItem(tracks[2], 'review'),
    ]);
    api.updateMatchItem.mockImplementation(() => Promise.resolve({}));
  });

  it('accepts every pending recommendation in one click, not just the autoAccept subset', async () => {
    const user = userEvent.setup();
    render(<ReviewPage trackIds={[]} matchJobId="job-1" onBack={vi.fn()} onComplete={vi.fn()} />);

    expect(await screen.findByRole('heading', {name: '审核抓取结果'})).toBeInTheDocument();
    // 三首里两首待确认 —— 按钮必须报出真实数量，否则看起来像「点了没反应」
    const button = await screen.findByRole('button', {name: /接受所有自动推荐（2）/});
    expect(button).toBeEnabled();

    await user.click(button);

    await waitFor(() => expect(api.updateMatchItem).toHaveBeenCalledTimes(2));
    expect(api.updateMatchItem).toHaveBeenCalledWith('job-1', tracks[0].id, 'accepted', expect.any(String), expect.any(Array), expect.any(Boolean), expect.any(Number));
    expect(api.updateMatchItem).toHaveBeenCalledWith('job-1', tracks[2].id, 'accepted', expect.any(String), expect.any(Array), expect.any(Boolean), expect.any(Number));
    // 已接受的那首不该被重复提交
    expect(api.updateMatchItem).not.toHaveBeenCalledWith('job-1', tracks[1].id, expect.anything(), expect.anything(), expect.anything(), expect.anything(), expect.anything());
    // 没有可接受的了 → 按钮自锁，用户一眼能看出已完成
    expect(await screen.findByRole('button', {name: '接受所有自动推荐'})).toBeDisabled();
  });

  it('leaves skipped tracks alone so an explicit skip is not silently undone', async () => {
    const user = userEvent.setup();
    api.listMatchItems.mockResolvedValue([
      reviewItem(tracks[0], 'review'),
      reviewItem(tracks[1], 'skipped'),
    ]);
    render(<ReviewPage trackIds={[]} matchJobId="job-1" onBack={vi.fn()} onComplete={vi.fn()} />);

    const button = await screen.findByRole('button', {name: /接受所有自动推荐（1）/});
    await user.click(button);

    await waitFor(() => expect(api.updateMatchItem).toHaveBeenCalledTimes(1));
    expect(api.updateMatchItem).toHaveBeenCalledWith('job-1', tracks[0].id, 'accepted', expect.any(String), expect.any(Array), expect.any(Boolean), expect.any(Number));
  });

  it('stays disabled when every recommendation is already accepted', async () => {
    api.listMatchItems.mockResolvedValue([
      reviewItem(tracks[0], 'accepted'),
      reviewItem(tracks[1], 'accepted'),
    ]);
    render(<ReviewPage trackIds={[]} matchJobId="job-1" onBack={vi.fn()} onComplete={vi.fn()} />);

    expect(await screen.findByRole('button', {name: '接受所有自动推荐'})).toBeDisabled();
  });
});
