import {
  History,
  LibraryBig,
  ListTodo,
  Settings2,
} from 'lucide-react';
import {cn} from '@/lib/utils';
import {GlobalPlayer} from '@/components/GlobalPlayer';
import {TaggerMark} from '@/components/TaggerMark';
import type {PlayerMode} from '@/lib/playerQueue';
import type {PageID, Track} from '@/types';

interface TopBarProps {
  page: PageID;
  version?: string;
  onNavigate: (page: PageID) => void;
  playerTrack: Track | null;
  playerPlaying: boolean;
  onPlayerPlayingChange: (playing: boolean) => void;
  onPlayerTrackEnded: () => void;
  onPlayerPrevious: () => void;
  onPlayerNext: () => void;
  playerMode: PlayerMode;
  onPlayerModeChange: (mode: PlayerMode) => void;
  onNotice?: (message: string) => void;
  playerRestartToken?: number;
  onPlayerClose: () => void;
}

const navItems: Array<{id: PageID; label: string; icon: typeof LibraryBig}> = [
  {id: 'library', label: '曲库', icon: LibraryBig},
  {id: 'jobs', label: '任务', icon: ListTodo},
  {id: 'history', label: '历史', icon: History},
  {id: 'settings', label: '设置', icon: Settings2},
];

export function TopBar({page, version, onNavigate, playerTrack, playerPlaying, onPlayerPlayingChange, onPlayerTrackEnded, onPlayerPrevious, onPlayerNext, playerMode, onPlayerModeChange, onNotice, playerRestartToken, onPlayerClose}: TopBarProps) {
  return (
    <header className="top-bar">
      <button className="brand-block" onClick={() => onNavigate('library')} aria-label="返回曲库">
        <TaggerMark className="brand-mark" />
        <span>
          <strong>TAGGER</strong>
          <small title={version ? `Tagger ${version}` : undefined}>{version ? `v${version}` : 'MUSIC ARCHIVE / 01'}</small>
        </span>
      </button>

      <nav className="primary-nav" aria-label="主导航">
        {navItems.map(({id, label, icon: Icon}) => (
          <button
            key={id}
            className={cn('nav-item', page === id && 'is-active')}
            onClick={() => onNavigate(id)}
          >
            <Icon size={16} aria-hidden="true" />
            <span>{label}</span>
          </button>
        ))}
      </nav>

      <GlobalPlayer track={playerTrack} playing={playerPlaying} onPlayingChange={onPlayerPlayingChange} onTrackEnded={onPlayerTrackEnded} onPrevious={onPlayerPrevious} onNext={onPlayerNext} mode={playerMode} onModeChange={onPlayerModeChange} onNotice={onNotice} restartToken={playerRestartToken} onClose={onPlayerClose} />
    </header>
  );
}
