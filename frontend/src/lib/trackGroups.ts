import type {Track} from '@/types';

// 「专辑顺序」是后端的默认排序（internal/library/query.go 按 Album → 碟号 → 轨号），
// 因此同一张专辑的曲目在列表里必然连续。前端只依赖这个前提做视觉分组，
// 不再额外请求后端 —— 换成年份/标题等其它排序时分组必须关闭（相邻关系不再成立）。
export function albumGroupKey(track: Track): string {
  const album = track.album.trim();
  const artists = track.albumArtists.map((value) => value.trim()).filter(Boolean).join('\u0001');
  return `${album}\u0000${artists}`;
}

export interface AlbumGroupInfo {
  album: string;
  artists: string[];
}

export function albumGroupInfo(track: Track): AlbumGroupInfo {
  return {
    album: track.album.trim(),
    artists: (track.albumArtists.length > 0 ? track.albumArtists : track.artists).filter((value) => value.trim() !== ''),
  };
}

// 整轨 CUE 的虚拟轨道共用同一个专辑名，会自然落进同一组。
export function albumGroupCounts(tracks: Track[]): Map<string, number> {
  const counts = new Map<string, number>();
  tracks.forEach((track) => {
    const key = albumGroupKey(track);
    counts.set(key, (counts.get(key) ?? 0) + 1);
  });
  return counts;
}
