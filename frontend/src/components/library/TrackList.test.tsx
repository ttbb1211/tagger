import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type {ReactNode} from 'react';
import {describe, expect, it, vi} from 'vitest';
import {seedTracks} from '@/mock/data';
import type {Track} from '@/types';

// jsdom 没有布局，react-virtuoso 不会渲染任何行。这里换成一个「把 data 全部渲染
// 出来」的替身，让行内交互与列表尾部的占位可以被断言。
vi.mock('react-virtuoso', () => ({
  Virtuoso: ({data, itemContent, components}: {
    data: Track[];
    itemContent: (index: number, item: Track) => ReactNode;
    components?: {Footer?: () => ReactNode};
  }) => (
    <div>
      {data.map((track, index) => <div key={track.id}>{itemContent(index, track)}</div>)}
      {components?.Footer ? components.Footer() : null}
    </div>
  ),
}));

vi.mock('@/api', async () => ({
  ...(await vi.importActual<typeof import('@/api')>('@/api')),
  artworkURL: () => undefined,
}));

import {TrackList} from '@/components/library/TrackList';

const indexedTrack: Track = {...seedTracks[0], id: 'trk-ok', fileName: 'ok.flac', title: '正常曲目', syncState: 'indexed'};
const brokenTrack: Track = {...seedTracks[1], id: 'trk-broken', fileName: 'broken.wav', title: '坏文件', syncState: 'error'};

function renderList(tracks: Track[], overrides: Partial<Parameters<typeof TrackList>[0]> = {}) {
  return render(
    <TrackList
      tracks={tracks}
      selectedIds={new Set()}
      onSelectTrack={vi.fn()}
      onToggleTrack={vi.fn()}
      onToggleAll={vi.fn()}
      {...overrides}
    />,
  );
}

describe('TrackList 未索引曲目', () => {
  it('offers a retry action only for tracks whose tags failed to parse', async () => {
    const user = userEvent.setup();
    const onRetryParse = vi.fn();
    renderList([indexedTrack, brokenTrack], {onRetryParse});

    expect(screen.queryByRole('button', {name: '重新解析 正常曲目'})).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', {name: '重新解析 坏文件'}));
    expect(onRetryParse).toHaveBeenCalledWith(expect.objectContaining({id: 'trk-broken'}));
  });

  it('keeps the retry icon inert when no handler is wired up', () => {
    renderList([brokenTrack]);
    expect(screen.getByRole('button', {name: '重新解析 坏文件'})).toBeDisabled();
  });

  it('reserves room at the end of the list so the selection bar stops covering rows', () => {
    const {container} = renderList([indexedTrack], {bottomSpacer: 84});
    expect(container.querySelector('div[aria-hidden="true"]')).toHaveStyle({height: '84px'});
  });
});

function albumTrack(id: string, album: string, title: string, albumArtists: string[] = ['测试歌手']): Track {
  return {...indexedTrack, id, album, albumArtists, title, fileName: `${id}.flac`};
}

describe('TrackList 专辑分组', () => {
  // 选中母文件夹时右侧是整库平铺长列表，首屏常被一张专辑占满 —— 分组标题是
  // 区分「这是整库」与「这只是一张专辑」的唯一视觉线索，故单独锁住。
  it('在专辑顺序下为每张专辑插入分组标题并带上曲目数', () => {
    const {container} = renderList([
      albumTrack('a1', '甲专辑', '甲一'),
      albumTrack('a2', '甲专辑', '甲二'),
      albumTrack('b1', '乙专辑', '乙一'),
    ], {albumGroups: true});

    const heads = [...container.querySelectorAll('.track-group-head')];
    expect(heads).toHaveLength(2);
    expect(heads[0]).toHaveTextContent('甲专辑');
    expect(heads[0]).toHaveTextContent('测试歌手');
    expect(heads[0]).toHaveTextContent('2 首');
    expect(heads[1]).toHaveTextContent('乙专辑');
    expect(heads[1]).toHaveTextContent('1 首');
  });

  it('非专辑排序（未开启分组）时不插入任何分组标题', () => {
    const {container} = renderList([albumTrack('a1', '甲专辑', '甲一'), albumTrack('b1', '乙专辑', '乙一')]);
    expect(container.querySelectorAll('.track-group-head')).toHaveLength(0);
  });

  it('专辑同名但专辑艺术家不同则各成一组，避免标题张冠李戴', () => {
    const {container} = renderList([
      albumTrack('c1', '合辑', '一', ['歌手甲']),
      albumTrack('c2', '合辑', '二', ['歌手乙']),
    ], {albumGroups: true});

    const heads = [...container.querySelectorAll('.track-group-head')];
    expect(heads).toHaveLength(2);
    expect(heads[0]).toHaveTextContent('歌手甲');
    expect(heads[1]).toHaveTextContent('歌手乙');
  });

  it('缺专辑名时给出「未标记专辑」占位，不显示空白标题', () => {
    const {container} = renderList([albumTrack('d1', '', '无专辑曲目')], {albumGroups: true});
    expect(container.querySelector('.track-group-head')).toHaveTextContent('未标记专辑');
  });
});
