import type {ComponentProps} from 'react';
import {fireEvent, render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {describe, expect, it, vi} from 'vitest';
import {GlobalPlayer} from '@/components/GlobalPlayer';
import type {PlayerMode} from '@/lib/playerQueue';
import {seedTracks} from '@/mock/data';

const track = seedTracks[0];

function renderPlayer(overrides: Partial<Pick<ComponentProps<typeof GlobalPlayer>, 'track' | 'playing' | 'mode' | 'restartToken'>> = {}) {
  const onPlayingChange = vi.fn();
  const onTrackEnded = vi.fn();
  const onPrevious = vi.fn();
  const onNext = vi.fn();
  const onModeChange = vi.fn();
  const onClose = vi.fn();
  const view = render(
    <GlobalPlayer
      track={track}
      playing
      mode="order"
      onPlayingChange={onPlayingChange}
      onTrackEnded={onTrackEnded}
      onPrevious={onPrevious}
      onNext={onNext}
      onModeChange={onModeChange}
      onClose={onClose}
      {...overrides}
    />,
  );
  const audio = view.container.querySelector('audio');
  if (!audio) throw new Error('player audio element missing');
  return {audio, view, onPlayingChange, onTrackEnded, onPrevious, onNext, onModeChange, onClose};
}

describe('GlobalPlayer 播放模式', () => {
  it('asks the host to advance when the track reaches its end', () => {
    const {audio, onTrackEnded} = renderPlayer();
    fireEvent.ended(audio);
    expect(onTrackEnded).toHaveBeenCalledTimes(1);
  });

  it('replays the same track instead of advancing while single loop is on', () => {
    const {audio, onTrackEnded} = renderPlayer({mode: 'repeat-one'});
    fireEvent.ended(audio);
    expect(onTrackEnded).not.toHaveBeenCalled();
  });

  it('still hands the end over to the host in list-loop and shuffle mode', () => {
    for (const mode of ['repeat-all', 'shuffle'] as PlayerMode[]) {
      const {audio, onTrackEnded} = renderPlayer({mode});
      fireEvent.ended(audio);
      expect(onTrackEnded).toHaveBeenCalledTimes(1);
    }
  });

  it('asks the host for the next mode when the mode button is clicked', async () => {
    const user = userEvent.setup();
    const {onModeChange} = renderPlayer({mode: 'order'});
    await user.click(screen.getByTitle('顺序播放（点击切换为列表循环）'));
    expect(onModeChange).toHaveBeenCalledWith('repeat-all');
  });

  it('offers 单曲循环 → 随机播放 as the next mode step', async () => {
    const user = userEvent.setup();
    const {onModeChange} = renderPlayer({mode: 'repeat-one'});
    await user.click(screen.getByTitle('单曲循环（点击切换为随机播放）'));
    expect(onModeChange).toHaveBeenCalledWith('shuffle');
  });

  it('swallows the trailing pause event that follows a finished track', () => {
    const {audio, onPlayingChange} = renderPlayer();
    fireEvent.ended(audio);
    onPlayingChange.mockClear();
    fireEvent.pause(audio);
    expect(onPlayingChange).not.toHaveBeenCalledWith(false);
  });

  it('still reports a pause the listener asked for', () => {
    const {audio, onPlayingChange} = renderPlayer();
    onPlayingChange.mockClear();
    fireEvent.pause(audio);
    expect(onPlayingChange).toHaveBeenCalledWith(false);
  });

  it('releases the suppression once playback stops, so a later pause is honoured again', () => {
    const {audio, onPlayingChange, onTrackEnded, onPrevious, onNext, onModeChange, onClose, view} = renderPlayer();
    fireEvent.ended(audio);
    view.rerender(
      <GlobalPlayer track={track} playing={false} mode="order" onPlayingChange={onPlayingChange} onTrackEnded={onTrackEnded} onPrevious={onPrevious} onNext={onNext} onModeChange={onModeChange} onClose={onClose} />,
    );
    onPlayingChange.mockClear();
    fireEvent.pause(audio);
    expect(onPlayingChange).toHaveBeenCalledWith(false);
  });

  it('hands 上一首 / 下一首 over to the host', async () => {
    const user = userEvent.setup();
    const {onPrevious, onNext} = renderPlayer();
    await user.click(screen.getByTitle('上一首'));
    expect(onPrevious).toHaveBeenCalledTimes(1);
    await user.click(screen.getByTitle('下一首'));
    expect(onNext).toHaveBeenCalledTimes(1);
  });

  it('keeps 上一首 / 下一首 inert while nothing is loaded', () => {
    renderPlayer({track: null});
    expect(screen.getByTitle('上一首')).toBeDisabled();
    expect(screen.getByTitle('下一首')).toBeDisabled();
  });
});
