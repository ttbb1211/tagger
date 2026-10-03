import {memo, useEffect, useMemo, useRef} from 'react';
import {Virtuoso, type StateSnapshot, type VirtuosoHandle} from 'react-virtuoso';
import {AlertCircle, Check, ChevronDown, ListFilter, LoaderCircle, MoreHorizontal, Pause, Play} from 'lucide-react';
import {CoverArt} from '@/components/CoverArt';
import {artworkURL} from '@/api';
import {albumGroupCounts, albumGroupInfo, albumGroupKey} from '@/lib/trackGroups';
import {cn, formatDuration} from '@/lib/utils';
import type {Track} from '@/types';

interface TrackListProps {
  tracks: Track[];
  showGeneratedCovers?: boolean;
  activeTrackId?: string;
  // 播放态：列表原先完全不知道「正在播放」这件事，于是顶栏播 A、列表与右侧面板
  // 都指向 B，用户看不出在播哪首。现在把播放器状态接到列表上：
  //   · is-playing 行染色 + 左侧竖条 + 波形
  //   · 封面叠加播放/暂停键，点它同时把该行设为选中 ⇒ 面板与播放器永远一致
  playerTrackId?: string;
  playerPlaying?: boolean;
  onToggleTrackPlay?: (track: Track) => void;
  selectedIds: Set<string>;
  onSelectTrack: (track: Track) => void;
  onToggleTrack: (trackId: string) => void;
  onToggleAll: () => void;
  resultSelectionActive?: boolean;
  hasMore?: boolean;
  loadingMore?: boolean;
  onEndReached?: () => void;
  restoreStateFrom?: StateSnapshot;
  initialScrollTop?: number;
  onViewportState?: (state: StateSnapshot) => void;
  // 解析失败（syncState === 'error'）的曲目不能参与任何批量操作，行内给一个
  // 重试入口，否则用户只能看到一个红图标，不知道原因也无法自救。
  onRetryParse?: (track: Track) => void;
  // 底部选择浮层会盖住列表最后几行，选中时在列表尾部留出等高空白。
  bottomSpacer?: number;
  // 仅在「专辑顺序」排序下开启：选中母文件夹后，整库曲目是一条平铺长列表，
  // 首屏往往整屏都属于同一张专辑，极易被误读成「只显示了这一张专辑」。
  // 开启后按专辑插入分组标题，让范围和专辑边界一眼可见。
  albumGroups?: boolean;
}

const healthLabel: Record<Track['health'], string> = {
  complete: '完整',
  'tag-compatibility': '标签兼容问题',
  'missing-artwork': '无封面',
  'missing-lyrics': '无歌词',
  'needs-review': '需确认',
  'parse-error': '解析失败',
  missing: '文件缺失',
};

function unambiguousHint(track: Track): Track['tagHints'][number] | undefined {
  return track.tagHints.length === 1 ? track.tagHints[0] : undefined;
}

// 分组标题只做「划范围」，不参与选择：整批选取仍走表头全选与行内勾选，
// 避免给批量写入引入第二套选中语义。
const AlbumGroupHead = memo(function AlbumGroupHead({track, count}: {track: Track; count?: number}) {
  const {album, artists} = albumGroupInfo(track);
  return (
    <div className="track-group-head" role="row" aria-label={`专辑 ${album || '未标记专辑'}`}>
      <span className="track-group-album">{album || '未标记专辑'}</span>
      {artists.length > 0 && <span className="track-group-artist">{artists.join(' / ')}</span>}
      {typeof count === 'number' && count > 0 && <em>{count} 首</em>}
    </div>
  );
});

// 正在播放的跳动波形。暂停时静止但仍保留形状 —— 让「哪首在播」不依赖颜色。
const NowPlayingWave = memo(function NowPlayingWave({active}: {active: boolean}) {
  return (
    <span className={cn('track-playing-wave', active && 'is-playing')} aria-hidden="true">
      <i /><i /><i /><i />
    </span>
  );
});

const TrackRow = memo(function TrackRow({
  track,
  showGeneratedCovers = false,
  active,
  selected,
  playing,
  playingNow,
  onSelect,
  onToggle,
  onTogglePlay,
  onRetryParse,
}: {
  track: Track;
  showGeneratedCovers?: boolean;
  active: boolean;
  selected: boolean;
  playing: boolean;
  playingNow: boolean;
  onSelect: () => void;
  onToggle: () => void;
  onTogglePlay?: () => void;
  onRetryParse?: (track: Track) => void;
}) {
  const hint = unambiguousHint(track);
  const displayTitle = track.title || hint?.title || track.fileName;
  const displayArtists = track.artists.length > 0 ? track.artists : hint?.artists ?? [];
  const inferredDisplay = !track.title && Boolean(hint?.title);
  return (
    <div
      className={cn('track-row', active && 'is-active', selected && 'is-selected', playing && 'is-playing', track.syncState === 'draft' && 'is-syncing')}
      onClick={onSelect}
      role="row"
      tabIndex={0}
      onKeyDown={(event) => {
        if (event.key === 'Enter') onSelect();
        if (event.key === ' ') {
          event.preventDefault();
          onToggle();
        }
      }}
    >
      <div className="track-check" onClick={(event) => event.stopPropagation()}>
        <button
          className={cn('square-check', selected && 'is-checked')}
          aria-label={selected ? `取消选择 ${displayTitle}` : `选择 ${displayTitle}`}
          onClick={onToggle}
        >
          {selected && <Check size={12} strokeWidth={3} />}
        </button>
      </div>
      <div className="track-cover-wrap">
        <CoverArt
          title={displayTitle}
          artist={displayArtists[0]}
          tone={track.coverTone}
          missing={!showGeneratedCovers && track.artworkCount === 0}
          imageUrl={artworkURL(track)}
          blankOnImageError={!showGeneratedCovers}
          size="xs"
        />
        {onTogglePlay && (
          <button
            className="track-play-overlay"
            title={track.syncState === 'draft' ? '索引完成后可试听' : playingNow ? '暂停' : '试听'}
            aria-label={`${playingNow ? '暂停' : '试听'} ${displayTitle}`}
            disabled={track.syncState === 'draft'}
            onClick={(event) => {
              event.stopPropagation();
              onTogglePlay();
            }}
          >
            {playingNow ? <Pause size={13} fill="currentColor" /> : <Play size={13} fill="currentColor" />}
          </button>
        )}
      </div>
      <div className="track-primary">
        <strong>{displayTitle}{inferredDisplay && <em className="inferred-tag">推断</em>}{playing && <NowPlayingWave active={playingNow} />}</strong>
        <span>{track.fileName}</span>
      </div>
      <div className="track-cell track-artist">{displayArtists.join(' / ') || (track.tagHints.length > 1 ? '文件名待确认' : '—')}</div>
      <div className="track-cell track-album">{track.album || '—'}</div>
      <div className="track-cell track-year">{track.year || '—'}</div>
      <div className="track-cell track-format"><span>{track.format.toUpperCase()}</span></div>
      <div className="track-cell track-time">{formatDuration(track.durationSeconds)}</div>
	  <div className="track-status">
		{track.syncState === 'draft' ? (
		  <span title="正在索引"><LoaderCircle size={14} className="spin" /></span>
		) : track.syncState === 'error' ? (
		  <button
		    type="button"
		    className="track-retry"
		    title={onRetryParse ? '标签解析失败，点击重新解析' : '标签解析失败'}
		    aria-label={`重新解析 ${displayTitle}`}
		    disabled={!onRetryParse}
		    onClick={(event) => { event.stopPropagation(); onRetryParse?.(track); }}
		  >
		    <AlertCircle size={14} />
		  </button>
		) : track.health !== 'complete' && (
		  <span title={healthLabel[track.health]}><AlertCircle size={14} /></span>
		)}
      </div>
      <button className="row-more" title="更多曲目操作" onClick={(event) => event.stopPropagation()}>
        <MoreHorizontal size={17} />
      </button>
    </div>
  );
});

export function TrackList({
  tracks,
  showGeneratedCovers = false,
  activeTrackId,
  playerTrackId,
  playerPlaying = false,
  onToggleTrackPlay,
  selectedIds,
  onSelectTrack,
  onToggleTrack,
  onToggleAll,
  resultSelectionActive = false,
  hasMore = false,
  loadingMore = false,
  onEndReached,
  restoreStateFrom,
  initialScrollTop,
  onViewportState,
  onRetryParse,
  bottomSpacer = 0,
  albumGroups = false,
}: TrackListProps) {
  const virtuosoRef = useRef<VirtuosoHandle>(null);
  const viewportStateRef = useRef(onViewportState);
  viewportStateRef.current = onViewportState;
  useEffect(() => () => {
    virtuosoRef.current?.getState((state) => viewportStateRef.current?.(state));
  }, []);
  const groupCounts = useMemo(() => (albumGroups ? albumGroupCounts(tracks) : undefined), [albumGroups, tracks]);
  const allSelected = resultSelectionActive || (tracks.length > 0 && tracks.every((track) => selectedIds.has(track.id)));

  return (
    <div className="track-list" role="table" aria-label="音乐文件列表">
      <div className="track-head" role="row">
        <button
          className={cn('square-check', allSelected && 'is-checked')}
          aria-label={allSelected ? '取消全选' : '全选当前结果集'}
          onClick={onToggleAll}
        >
          {allSelected && <Check size={12} strokeWidth={3} />}
        </button>
        <span className="head-cover">封面</span>
        <button>标题 / 文件名 <ChevronDown size={12} /></button>
        <span>艺术家</span>
        <span>专辑</span>
        <span>年份</span>
        <span>格式</span>
        <span>时长</span>
        <span />
        <button className="column-settings" title="设置显示列"><ListFilter size={15} /></button>
      </div>

      {tracks.length > 0 ? (
        <Virtuoso
          ref={virtuosoRef}
          className="track-virtuoso"
          data={tracks}
          restoreStateFrom={restoreStateFrom}
          initialScrollTop={restoreStateFrom ? undefined : initialScrollTop}
          endReached={() => {
            if (hasMore && !loadingMore) onEndReached?.();
          }}
          components={{Footer: () => (
            <>
              {loadingMore ? <div className="track-list-footer">正在加载更多曲目…</div> : hasMore ? <div className="track-list-footer">继续滚动加载更多</div> : null}
              {bottomSpacer > 0 && <div style={{height: bottomSpacer}} aria-hidden="true" />}
            </>
          )}}
          itemContent={(index, track) => {
            // 分组标题挂在「该专辑第一首」这一项内部渲染，而不是往 data 里插伪元素：
            // 索引与曲目一一对应，虚拟滚动的状态恢复、endReached 分页和全选计数都不受影响。
            const previous = index > 0 ? tracks[index - 1] : undefined;
            const startsAlbum = albumGroups && (!previous || albumGroupKey(previous) !== albumGroupKey(track));
            return (
              <>
                {startsAlbum && <AlbumGroupHead track={track} count={groupCounts?.get(albumGroupKey(track))} />}
                <TrackRow
                  track={track}
                  showGeneratedCovers={showGeneratedCovers}
                  active={track.id === activeTrackId}
                  selected={selectedIds.has(track.id)}
                  playing={Boolean(playerTrackId) && track.id === playerTrackId}
                  playingNow={Boolean(playerTrackId) && track.id === playerTrackId && playerPlaying}
                  onSelect={() => onSelectTrack(track)}
                  onToggle={() => onToggleTrack(track.id)}
                  onTogglePlay={onToggleTrackPlay ? () => onToggleTrackPlay(track) : undefined}
                  onRetryParse={onRetryParse}
                />
              </>
            );
          }}
        />
      ) : (
        <div className="empty-state">
          <span>0</span>
          <strong>没有符合当前条件的曲目</strong>
          <p>调整筛选或搜索关键词后再试。</p>
        </div>
      )}
    </div>
  );
}
