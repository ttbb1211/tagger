import {render, screen, within} from '@testing-library/react';
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

// 老板 2026-09-28 定的规则：整轨父音频只读。
//
// 整轨 CUE 虚拟轨道在磁盘上没有自己的音频文件，父整轨是用户的原件。旧实现
// 把封面嵌进父音频，代价是改动文件本体（size + mtime 都变），还让同专辑所有
// 兄弟轨道的 revision 集体失效 —— 实测 12 首整轨写入 11/11 全失败。现在封面
// 一律不写内嵌：标签写 cue、歌词写 .lrc。
const cueTrack: Track = {
  ...seedTracks[0],
  id: 'track-cue-01',
  title: '偿还',
  cuePath: '邓丽君 - 偿还.wav',
};

const plainTrack: Track = {...seedTracks[0], id: 'track-plain-01'};

function reviewItem(track: Track): MatchItem {
  const candidates = candidatesFor(track);
  return {
    id: `item-${track.id}`,
    jobId: 'job-1',
    trackId: track.id,
    state: 'review',
    candidates,
    selectedCandidateId: candidates[0]?.id,
  };
}

function renderWith(tracks: Track[]) {
  api.resolveTracks.mockResolvedValue({tracks, total: tracks.length});
  api.listMatchItems.mockResolvedValue(tracks.map(reviewItem));
  render(<ReviewPage trackIds={[]} matchJobId="job-1" onBack={vi.fn()} onComplete={vi.fn()} />);
}

describe('ReviewPage 整轨父音频只读', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.getJob.mockResolvedValue({
      id: 'job-1', kind: 'match', state: 'review', title: '批量抓取元数据', detail: '已完成',
      processed: 1, total: 1, succeeded: 1, failed: 0, startedAt: '刚刚',
    });
    api.updateMatchItem.mockImplementation(() => Promise.resolve({}));
  });

  it('整轨虚拟轨道的封面行渲染为禁用，并说明父音频只读', async () => {
    renderWith([cueTrack]);
    expect(await screen.findByRole('heading', {name: '审核抓取结果'})).toBeInTheDocument();

    // 行内直接说明「不改父音频」，用户不用去猜为什么封面没写
    expect(await screen.findByText('整轨父音频只读')).toBeInTheDocument();
    const row = screen.getByText('替换封面').closest('.review-diff-row') as HTMLElement;
    const art = within(row);
    expect(art.getByText('已跳过')).toBeInTheDocument();
    expect(art.getByText('Tagger 规则')).toBeInTheDocument();
    expect(art.getByText(/整轨不改动父音频/)).toBeInTheDocument();

    // 勾选框必须真的点不动，而不是「看着禁用、点了还会写」
    expect(art.getByRole('button', {name: '采用替换封面'})).toBeDisabled();
    // 尺寸控件对整轨没有意义，不该出现
    expect(screen.queryByText('封面写入尺寸')).not.toBeInTheDocument();
  });

  it('普通曲目的封面行照旧可选，尺寸控件也在', async () => {
    renderWith([plainTrack]);
    expect(await screen.findByRole('heading', {name: '审核抓取结果'})).toBeInTheDocument();

    expect(await screen.findByText('来源提供封面')).toBeInTheDocument();
    expect(screen.queryByText('整轨父音频只读')).not.toBeInTheDocument();
    expect(screen.getByRole('button', {name: /替换封面$/})).toBeEnabled();
    expect(screen.getByText('封面写入尺寸')).toBeInTheDocument();
  });
});
