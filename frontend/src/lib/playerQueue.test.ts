import {describe, expect, it} from 'vitest';
import {nextInQueue, nextPlayerMode, previousInQueue, shuffleQueue, type PlayerMode} from '@/lib/playerQueue';
import {seedTracks} from '@/mock/data';

const [first, second, third, fourth] = seedTracks;
const ids = (tracks: {id: string}[]) => tracks.map((track) => track.id).sort();

describe('playerQueue / nextInQueue', () => {
  it('returns the following track in list order', () => {
    expect(nextInQueue([first, second, third], first)?.id).toBe(second.id);
    expect(nextInQueue([first, second, third], second)?.id).toBe(third.id);
  });

  it('stops at the end of the queue instead of wrapping around', () => {
    expect(nextInQueue([first, second, third], third)).toBeNull();
  });

  it('returns null when the current track is not in the queue', () => {
    expect(nextInQueue([second, third], first)).toBeNull();
    expect(nextInQueue([], first)).toBeNull();
    expect(nextInQueue([first, second], null)).toBeNull();
  });
});

describe('playerQueue / previousInQueue', () => {
  it('returns the preceding track in list order', () => {
    expect(previousInQueue([first, second, third], second)?.id).toBe(first.id);
    expect(previousInQueue([first, second, third], third)?.id).toBe(second.id);
  });

  it('stops at the head of the queue instead of wrapping around', () => {
    expect(previousInQueue([first, second, third], first)).toBeNull();
  });

  it('returns null when the current track is not in the queue', () => {
    expect(previousInQueue([second, third], first)).toBeNull();
    expect(previousInQueue([], first)).toBeNull();
    expect(previousInQueue([first, second], null)).toBeNull();
  });

  it('mirrors nextInQueue over the same queue', () => {
    const queue = [first, second, third, fourth];
    for (let i = 1; i < queue.length; i += 1) {
      expect(previousInQueue(queue, queue[i])?.id).toBe(nextInQueue([...queue].reverse(), queue[i])?.id);
    }
  });
});

describe('playerQueue / nextPlayerMode', () => {
  it('cycles 顺序 → 列表循环 → 单曲循环 → 随机 → 顺序', () => {
    expect(nextPlayerMode('order')).toBe('repeat-all');
    expect(nextPlayerMode('repeat-all')).toBe('repeat-one');
    expect(nextPlayerMode('repeat-one')).toBe('shuffle');
    expect(nextPlayerMode('shuffle')).toBe('order');
  });

  it('never gets stuck outside the cycle', () => {
    let mode: PlayerMode = 'order';
    const seen = new Set<PlayerMode>();
    for (let i = 0; i < 4; i += 1) {
      seen.add(mode);
      mode = nextPlayerMode(mode);
    }
    expect(mode).toBe('order');
    expect(seen.size).toBe(4);
  });
});

describe('playerQueue / shuffleQueue', () => {
  it('keeps the current track first so playback continues from it', () => {
    const order = shuffleQueue([first, second, third, fourth], second, () => 0);
    expect(order[0].id).toBe(second.id);
    expect(order).toHaveLength(4);
  });

  it('keeps every track exactly once', () => {
    const queue = [first, second, third, fourth];
    const order = shuffleQueue(queue, first, () => 0.42);
    expect(ids(order)).toEqual(ids(queue));
    expect(new Set(order.map((track) => track.id)).size).toBe(queue.length);
  });

  it('is deterministic for a given random source', () => {
    const queue = [first, second, third, fourth];
    expect(shuffleQueue(queue, first, () => 0).map((t) => t.id)).toEqual([first.id, third.id, fourth.id, second.id]);
    expect(shuffleQueue(queue, first, () => 0.999999).map((t) => t.id)).toEqual([first.id, second.id, third.id, fourth.id]);
  });

  it('does not invent a head when the current track is not in the queue', () => {
    const order = shuffleQueue([second, third], first, () => 0);
    expect(order).toHaveLength(2);
    expect(order.map((t) => t.id)).not.toContain(first.id);
  });

  it('handles an empty queue', () => {
    expect(shuffleQueue([], first)).toEqual([]);
  });
});
