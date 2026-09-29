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
