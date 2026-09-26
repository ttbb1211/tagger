import {useEffect, useRef, useState} from 'react';
import {Pause, Play, Repeat1, X} from 'lucide-react';
import {audioURL} from '@/api';
import {cn, formatDuration} from '@/lib/utils';
import type {Track} from '@/types';

interface GlobalPlayerProps {
  track: Track | null;
  playing: boolean;
  onPlayingChange: (playing: boolean) => void;
  onClose: () => void;
}

export function GlobalPlayer({track, playing, onPlayingChange, onClose}: GlobalPlayerProps) {
  const audioRef = useRef<HTMLAudioElement>(null);
  const [currentTime, setCurrentTime] = useState(0);
  const [singleLoop, setSingleLoop] = useState(false);
  const source = track ? audioURL(track) : undefined;

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
      void audio.play().catch(() => onPlayingChange(false));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [source, track?.id]);

  useEffect(() => {
    // 整轨段循环由 onTimeUpdate 手动处理，避免原生 loop 循环整个文件
    if (audioRef.current) audioRef.current.loop = singleLoop && !isCue;
  }, [singleLoop, isCue]);

  useEffect(() => {
    const audio = audioRef.current;
    if (!audio || !source) return;
    if (playing && audio.paused) {
      void audio.play().catch(() => onPlayingChange(false));
    } else if (!playing && !audio.paused) {
      audio.pause();
    }
  }, [playing, source, onPlayingChange]);

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
      void audio.play().then(() => onPlayingChange(true)).catch(() => onPlayingChange(false));
    } else {
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

  const handleTimeUpdate = (audio: HTMLAudioElement) => {
    if (isCue && cueEnd > cueStart && audio.currentTime >= cueEnd - 0.15) {
      if (singleLoop) {
        audio.currentTime = cueStart;
        setCurrentTime(cueStart);
        return;
      }
      audio.pause();
      audio.currentTime = cueEnd;
      setCurrentTime(cueEnd);
      onPlayingChange(false);
      return;
    }
    setCurrentTime(audio.currentTime);
  };

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
        onPlay={() => onPlayingChange(true)}
        onPause={() => onPlayingChange(false)}
        onEnded={() => onPlayingChange(singleLoop ? true : false)}
        onError={() => onPlayingChange(false)}
      />
      <button className="global-player-toggle" disabled={!track} title={track ? (playing ? '暂停播放' : '继续播放') : '暂无播放歌曲'} onClick={toggle}>
        {playing ? <Pause size={15} fill="currentColor" /> : <Play size={15} fill="currentColor" />}
      </button>
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
      <button className={cn('icon-button', 'global-player-loop', singleLoop && 'is-active')} disabled={!track} aria-pressed={singleLoop} title={singleLoop ? '关闭单曲循环' : '开启单曲循环'} onClick={() => setSingleLoop((value) => !value)}><Repeat1 size={16} /></button>
      <button className={cn('icon-button', 'global-player-close')} disabled={!track} title="关闭播放器" onClick={onClose}><X size={15} /></button>
    </div>
  );
}
