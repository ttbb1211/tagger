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

describe('TrackList 正在播放标识', () => {
  // 整库平铺长列表里「播放器在播 A、面板和列表高亮都是 B」最容易让人误判 ——
  // 这里把「只有真正在播的那一行染色」与「暂停时不跑动画」分别锁住。
  it('只给真正在播的那一行加 is-playing，且不影响选中行', () => {
    const {container} = renderList([albumTrack('a1', '甲专辑', '甲一'), albumTrack('a2', '甲专辑', '甲二')], {
      playerTrackId: 'a2',
      playerPlaying: true,
      activeTrackId: 'a1',
    });

    const rows = [...container.querySelectorAll('.track-row')];
    expect(rows[0].className).not.toContain('is-playing');
    expect(rows[0].className).toContain('is-active');
    expect(rows[1].className).toContain('is-playing');
  });

  it('播放中波形跑动画，暂停时波形停住但整行仍是播放态', () => {
    const playing = renderList([albumTrack('a1', '甲专辑', '甲一')], {playerTrackId: 'a1', playerPlaying: true});
    expect(playing.container.querySelector('.track-playing-wave')?.className).toContain('is-playing');
    playing.unmount();

    const paused = renderList([albumTrack('a1', '甲专辑', '甲一')], {playerTrackId: 'a1', playerPlaying: false});
    expect(paused.container.querySelector('.track-playing-wave')?.className).not.toContain('is-playing');
    expect(paused.container.querySelector('.track-row')?.className).toContain('is-playing');
  });

  it('没在播任何曲目时没有任何播放态痕迹', () => {
    const {container} = renderList([albumTrack('a1', '甲专辑', '甲一')], {playerTrackId: undefined});
    expect(container.querySelector('.track-row')?.className).not.toContain('is-playing');
    expect(container.querySelector('.track-playing-wave')).toBeNull();
  });

  it('点封面上的播放键会把这一首交给上层，而不是静默改状态', async () => {
    const user = userEvent.setup();
    const onToggleTrackPlay = vi.fn();
    render(<TrackList tracks={[albumTrack('a1', '甲专辑', '甲一')]} selectedIds={new Set()} onSelectTrack={vi.fn()} onToggleTrack={vi.fn()} onToggleAll={vi.fn()} playerTrackId="a1" playerPlaying onToggleTrackPlay={onToggleTrackPlay} />);

    await user.click(screen.getByRole('button', {name: '暂停 甲一'}));
    expect(onToggleTrackPlay).toHaveBeenCalledWith(expect.objectContaining({id: 'a1'}));
  });

  it('未接播放回调时行内不出现播放键，避免点了没反应', () => {
    renderList([albumTrack('a1', '甲专辑', '甲一')], {playerTrackId: 'a1', playerPlaying: true});
    expect(screen.queryByRole('button', {name: /试听|暂停/})).not.toBeInTheDocument();
  });

  it('索引中的曲目不给播放键', () => {
    renderList([{...albumTrack('a1', '甲专辑', '甲一'), syncState: 'draft'}], {
      playerTrackId: 'a1',
      playerPlaying: true,
      onToggleTrackPlay: vi.fn(),
    });
    expect(screen.getByRole('button', {name: '暂停 甲一'})).toBeDisabled();
  });
});
