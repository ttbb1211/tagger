import {describe, expect, it} from 'vitest';
import {candidatesFor, library, providerConfigs, seedTracks} from '@/mock/data';

describe('mock music archive', () => {
  it('mirrors the TestMusic sample directory', () => {
    expect(library.name).toBe('TestMusic');
    expect(seedTracks).toHaveLength(24);
    expect(seedTracks.some((track) => track.fileName === '再回首-姜育恒.flac')).toBe(true);
    expect(seedTracks.filter((track) => track.album === '安泊猜想')).toHaveLength(9);
    expect(seedTracks.filter((track) => track.album === '青年晚报')).toHaveLength(9);
  });

  it('keeps provider candidates ordered by confidence', () => {
    const candidates = candidatesFor(seedTracks[0]);
	  expect(candidates).toHaveLength(4);
	  expect(candidates[0].kind).toBe('smart');
	  expect(candidates[0].recommended).toBe(true);
    expect(candidates[0].score).toBeGreaterThan(candidates[1].score);
	  expect(candidates[3].scoreLabel).toContain('版本');
  });

  it('marks unofficial providers as experimental and enabled by default', () => {
    for (const id of ['netease', 'kuwo', 'kugou', 'lrcapi']) {
      const provider = providerConfigs.find((entry) => entry.id === id);
      expect(provider?.experimental, `${id} should stay experimental`).toBe(true);
      expect(provider?.enabled, `${id} should be enabled by default`).toBe(true);
    }
  });
});
