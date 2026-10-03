import {useEffect, useRef, useState} from 'react';
import {ListOrdered, Pause, Play, Repeat, Repeat1, Shuffle, SkipBack, SkipForward, X} from 'lucide-react';
import {audioURL} from '@/api';
import {nextPlayerMode, type PlayerMode} from '@/lib/playerQueue';
import {cn, formatDuration} from '@/lib/utils';
import type {Track} from '@/types';

interface GlobalPlayerProps {
  track: Track | null;
  playing: boolean;
  onPlayingChange: (playing: boolean) => void;
  /** 一首放完（非单曲循环）时回调，由上层切到播放队列的下一首 */
  onTrackEnded: () => void;
  /**
   * 手动「上一首 / 下一首」。与 `onTrackEnded` 的区别：手动切换**不受单曲循环影响**，
   * 且到队首/队尾不会把播放停掉（由上层决定绕回还是重播当前曲）。
   */
  onPrevious: () => void;
  onNext: () => void;
  /** 播放模式由上层持有（取下一首要用），这里只负责展示与切换 */
  mode: PlayerMode;
  onModeChange: (mode: PlayerMode) => void;
  /**
   * 递增即「重新加载并播放当前这首」。
   * 只有一个曲目的队列在「列表循环」下会绕回自己，光换 track 不会触发重载，靠它兜住。
   */
  restartToken?: number;
  onClose: () => void;
}

const MODE_UI: Record<PlayerMode, {icon: typeof Shuffle; title: string}> = {
  order: {icon: ListOrdered, title: '顺序播放（点击切换为列表循环）'},
  'repeat-all': {icon: Repeat, title: '列表循环（点击切换为单曲循环）'},
  'repeat-one': {icon: Repeat1, title: '单曲循环（点击切换为随机播放）'},
  shuffle: {icon: Shuffle, title: '随机播放（点击切换为顺序播放）'},
};


/**
 * 统一的起播入口。jsdom（单测环境）的 `HTMLMediaElement.play()` 不返回 Promise，
 * 直接 `.catch/.then` 会抛「Cannot read properties of undefined」；真实浏览器恒返回 Promise。
 * 这里两种都兜住：没有 Promise 就按「已开始」处理。
 */
function startPlayback(audio: HTMLAudioElement, onFailed: () => void, onStarted?: () => void) {
  let result: Promise<void> | undefined;
  try {
    result = audio.play() as Promise<void> | undefined;
  } catch {
    onFailed();
    return;
  }
  if (result && typeof result.then === 'function') {
    void result.then(() => onStarted?.(), onFailed);
  } else {
    onStarted?.();
  }
}

export function GlobalPlayer({track, playing, onPlayingChange, onTrackEnded, onPrevious, onNext, mode, onModeChange, restartToken = 0, onClose}: GlobalPlayerProps) {
  const audioRef = useRef<HTMLAudioElement>(null);
  // 「播完切下一首」期间抑制 <audio> 的 pause 事件：整轨非 WAV 的段尾是我们自己调 pause() 停的，
  // 浏览器随后补发的 pause 会把上层刚置上的「正在播放」又打回暂停。
  const advancingRef = useRef(false);
  const [currentTime, setCurrentTime] = useState(0);
  const source = track ? audioURL(track) : undefined;
  const repeatOne = mode === 'repeat-one';

  // 整轨 CUE 虚拟轨道：WAV 由后端切成独立段（audioURL 返回的就是该段），
  // 前端不能再叠加偏移；其余格式（FLAC/MP3 等）后端整文件下发，
  // 由前端 seek 到 [cueStart, cueEnd) 区间并在段尾停止。
  const isCue = Boolean(track?.cuePath) && track?.format !== 'wav';
  const cueStart = isCue ? Math.max(track?.startOffsetSeconds ?? 0, 0) : 0;
  const cueEnd = isCue
    ? (track?.endOffsetSeconds && track.endOffsetSeconds > cueStart ? track.endOffsetSeconds : cueStart + (track?.durationSeconds || 0))
    : (track?.durationSeconds || 0);
  // 播放器进度条 / 时长一律按「当前曲目段」显示
  const segmentDuration = Math.max(cueEnd - cueStart, 0);

  useEffect(() => {
    const audio = audioRef.current;
    if (!audio) return;
    setCurrentTime(cueStart);
    if (source) {
      audio.pause();
      audio.src = source;
      audio.load();
      // readyState 为 HAVE_NOTHING 时设置 currentTime 会被记为默认起播位置，加载后生效
      if (cueStart > 0) {
        try {
          audio.currentTime = cueStart;
        } catch {
          /* 忽略：元数据未就绪时由 onLoadedMetadata 兜底 */
        }
      }
    } else {
      audio.removeAttribute('src');
    }
    if (playing && source) {
      startPlayback(audio, () => onPlayingChange(false));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [source, track?.id, restartToken]);

  useEffect(() => {
    // 整轨段循环由 onTimeUpdate 手动处理，避免原生 loop 循环整个文件
    if (audioRef.current) audioRef.current.loop = repeatOne && !isCue;
  }, [repeatOne, isCue]);

  useEffect(() => {
    const audio = audioRef.current;
    if (!audio || !source) return;
    if (playing && audio.paused) {
      startPlayback(audio, () => onPlayingChange(false));
    } else if (!playing && !audio.paused) {
      audio.pause();
    }
  }, [playing, source, onPlayingChange]);

  useEffect(() => {
    // 队列到尽头 / 播放失败时 playing 归 false：解除抑制，别把用户之后的暂停也吞掉
    if (!playing) advancingRef.current = false;
  }, [playing]);

  const toggle = () => {
    const audio = audioRef.current;
    if (!track || !source || !audio) {
      onPlayingChange(!playing);
      return;
    }
    if (audio.paused) {
      // 段尾暂停后再次播放：回到段首重新开始
      if (isCue && audio.currentTime >= cueEnd - 0.15) {
        audio.currentTime = cueStart;
        setCurrentTime(cueStart);
      }
      startPlayback(audio, () => onPlayingChange(false), () => onPlayingChange(true));
    } else {
      advancingRef.current = false;
      audio.pause();
      onPlayingChange(false);
    }
  };

  // value 为相对当前段的秒数
  const seek = (value: number) => {
    const absolute = Math.min(Math.max(cueStart + value, cueStart), cueEnd);
    setCurrentTime(absolute);
    if (audioRef.current && track) audioRef.current.currentTime = absolute;
  };

  // 一首放完：单曲循环回到段首重播，否则交给上层切播放队列的下一首（顺序 / 随机）。
  // 普通曲目与整轨 WAV（后端已按 cue 切段）走原生 ended；
  // 整轨 FLAC 等（前端按 cue 偏移 seek）到不了文件末尾，由 onTimeUpdate 在段尾触发。
  const finishTrack = () => {
    if (repeatOne) {
      const audio = audioRef.current;
      if (audio) {
        try {
          audio.currentTime = cueStart;
        } catch {
          /* 元数据未就绪时忽略 */
        }
        setCurrentTime(cueStart);
        startPlayback(audio, () => onPlayingChange(false));
      }
      return;
    }
    advancingRef.current = true;
    onTrackEnded();
  };

  const handleTimeUpdate = (audio: HTMLAudioElement) => {
    if (isCue && cueEnd > cueStart && audio.currentTime >= cueEnd - 0.15) {
      if (repeatOne) {
        audio.currentTime = cueStart;
        setCurrentTime(cueStart);
        return;
      }
      audio.pause();
      audio.currentTime = cueEnd;
      setCurrentTime(cueEnd);
      finishTrack();
      return;
    }
    setCurrentTime(audio.currentTime);
  };

  const ModeIcon = MODE_UI[mode].icon;

  return (
    <div className="global-player" role="region" aria-label="全局播放器">
      <audio
        ref={audioRef}
        preload="metadata"
        onLoadedMetadata={(event) => {
          const audio = event.currentTarget;
          if (isCue && cueStart > 0 && Math.abs(audio.currentTime - cueStart) > 0.05) {
            audio.currentTime = cueStart;
          }
        }}
        onTimeUpdate={(event) => handleTimeUpdate(event.currentTarget)}
        onPlay={() => { advancingRef.current = false; onPlayingChange(true); }}
        onPause={(event) => {
          // 播完自然停止（ended 已置位）不算「用户暂停」；整轨非 WAV 的段尾暂停由 advancingRef 兜住
          if (advancingRef.current || event.currentTarget.ended) return;
          onPlayingChange(false);
        }}
        onEnded={finishTrack}
        onError={() => { advancingRef.current = false; onPlayingChange(false); }}
      />
      <button className={cn('icon-button', 'global-player-skip')} disabled={!track} title="上一首" aria-label="上一首" onClick={onPrevious}><SkipBack size={15} /></button>
      <button className="global-player-toggle" disabled={!track} title={track ? (playing ? '暂停播放' : '继续播放') : '暂无播放歌曲'} onClick={toggle}>
        {playing ? <Pause size={15} fill="currentColor" /> : <Play size={15} fill="currentColor" />}
      </button>
      <button className={cn('icon-button', 'global-player-skip')} disabled={!track} title="下一首" aria-label="下一首" onClick={onNext}><SkipForward size={15} /></button>
      <div className="global-player-copy">
        <strong>{track ? (track.title || track.fileName) : '无播放歌曲'}</strong>
        <span>{track ? `${track.artists.join(' / ') || '未知艺术家'} · ${track.album || '未知专辑'}` : '从曲库选择一首歌曲开始播放'}</span>
      </div>
      <input
        className="global-player-progress"
        type="range"
        min={0}
        max={segmentDuration}
        step={0.1}
        value={track ? Math.min(Math.max(currentTime - cueStart, 0), segmentDuration) : 0}
        aria-label="播放进度"
        disabled={!track || segmentDuration <= 0}
        onChange={(event) => seek(Number(event.target.value))}
      />
      <span className="global-player-time">{track ? `${formatDuration(Math.round(Math.max(currentTime - cueStart, 0)))} / ${formatDuration(segmentDuration)}` : '— / —'}</span>
      <button
        className={cn('icon-button', 'global-player-loop', mode !== 'order' && 'is-active')}
        disabled={!track}
        aria-pressed={mode !== 'order'}
        aria-label={MODE_UI[mode].title}
        title={MODE_UI[mode].title}
        onClick={() => onModeChange(nextPlayerMode(mode))}
      >
        <ModeIcon size={16} />
      </button>
      <button className={cn('icon-button', 'global-player-close')} disabled={!track} title="关闭播放器" onClick={onClose}><X size={15} /></button>
    </div>
  );
}
