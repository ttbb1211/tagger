import {useEffect, useState} from 'react';
import {AnimatePresence, motion} from 'motion/react';
import {CheckCircle2, KeyRound, LoaderCircle, X} from 'lucide-react';
import {TopBar} from '@/components/TopBar';
import {APIError, apiReadMode, getSystem, setAuthToken} from '@/api';
import {HistoryPage} from '@/pages/HistoryPage';
import {JobsPage} from '@/pages/JobsPage';
import {LibraryPage} from '@/pages/LibraryPage';
import {ReviewPage} from '@/pages/ReviewPage';
import {SettingsPage} from '@/pages/SettingsPage';
import {nextInQueue, previousInQueue, shuffleQueue, type PlayerMode} from '@/lib/playerQueue';
import {pageRoute, readRoute, routePath, type AppRoute} from '@/lib/router';
import {normalizeFont, normalizeTheme, type FontID, type ThemeID} from '@/theme';
import type {PageID, RestoreDraftRequest, Track} from '@/types';

export function App() {
  const [route, setRoute] = useState<AppRoute>(() => readRoute());
  const [theme, setTheme] = useState<ThemeID>(() => normalizeTheme(localStorage.getItem('tagger-theme-v2') ?? localStorage.getItem('tagger-theme')));
  const [font, setFont] = useState<FontID>(() => normalizeFont(localStorage.getItem('tagger-font-v1')));
  const [notice, setNotice] = useState<string | null>(null);
  const [playerTrack, setPlayerTrack] = useState<Track | null>(null);
  const [playerPlaying, setPlayerPlaying] = useState(false);
  // 播放队列 = 点「试听」那一刻曲库列表里可见的曲目（按显示顺序），用于「播完自动下一首」
  const [playerQueue, setPlayerQueue] = useState<Track[]>([]);
  // 播放模式：顺序播放 / 列表循环 / 单曲循环 / 随机播放，由播放器上一个按钮循环切换
  const [playerMode, setPlayerMode] = useState<PlayerMode>('order');
  // 随机播放用的固定顺序（开启随机时按当前队列洗一次），非随机模式下为空
  const [shuffleOrder, setShuffleOrder] = useState<Track[]>([]);
  // 递增即让播放器重新加载当前这首（单曲队列 + 列表循环绕回自己时用）
  const [playbackRestart, setPlaybackRestart] = useState(0);
  const [jobFocusId, setJobFocusId] = useState<string>();
  const [showGeneratedCovers, setShowGeneratedCovers] = useState(() => localStorage.getItem('tagger-generated-covers') === 'true');
  const [authState, setAuthState] = useState<'checking' | 'ready' | 'required'>(apiReadMode === 'mock' ? 'ready' : 'checking');
  const [version, setVersion] = useState('');
  const [authError, setAuthError] = useState('');
  const [restoreDraft, setRestoreDraft] = useState<RestoreDraftRequest>();

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    localStorage.setItem('tagger-theme-v2', theme);
  }, [theme]);

  useEffect(() => {
    document.documentElement.dataset.font = font;
    localStorage.setItem('tagger-font-v1', font);
  }, [font]);

  useEffect(() => {
    const handlePopState = () => setRoute(readRoute());
    window.addEventListener('popstate', handlePopState);
    const current = `${window.location.pathname}${window.location.search}`;
    const canonical = routePath(route);
    if (current !== canonical) window.history.replaceState({}, '', canonical);
    return () => window.removeEventListener('popstate', handlePopState);
    // Route is intentionally captured only on mount; browser navigation owns later updates.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    localStorage.setItem('tagger-generated-covers', String(showGeneratedCovers));
  }, [showGeneratedCovers]);

  useEffect(() => {
    if (apiReadMode === 'mock') return;
    void getSystem().then((info) => {
      if (info.version && info.version !== 'mock') setVersion(info.version);
      setAuthState('ready');
    }).catch((error) => {
      setAuthError(error instanceof APIError && error.status === 401 ? '当前服务已启用访问令牌保护' : (error instanceof Error ? error.message : '后台连接失败'));
      setAuthState('required');
    });
  }, []);

  useEffect(() => {
    if (!notice) return;
    const timer = window.setTimeout(() => setNotice(null), 3600);
    return () => window.clearTimeout(timer);
  }, [notice]);

  useEffect(() => {
    if (route.page !== 'jobs') setJobFocusId(undefined);
  }, [route.page]);

  const navigate = (nextRoute: AppRoute) => {
    const nextPath = routePath(nextRoute);
    const currentPath = `${window.location.pathname}${window.location.search}`;
    if (currentPath === nextPath) {
      setRoute(nextRoute);
      return;
    }
    window.history.pushState({}, '', nextPath);
    setRoute(nextRoute);
  };

  const navigatePage = (nextPage: PageID) => navigate(pageRoute(nextPage));

  const openReview = (ids: string[]) => navigate({page: 'review', batchIds: ids});

  const openReviewJob = (jobId: string) => navigate({page: 'review', batchIds: [], reviewJobId: jobId});

  const openJobs = (focusJobId?: string) => {
    setJobFocusId(focusJobId);
    navigatePage('jobs');
  };

  // queue 由曲库页按「当前可见列表顺序」传入：普通单曲目录、整轨 CUE 专辑都走同一条队列
  const playTrack = (track: Track, queue?: Track[]) => {
    const nextQueue = queue ?? playerQueue;
    if (queue) setPlayerQueue(queue);
    // 随机模式下换一首要重洗（当前曲目仍放首位），否则会接着用上一次的随机顺序
    if (playerMode === 'shuffle') setShuffleOrder(shuffleQueue(nextQueue, track));
    setPlayerTrack(track);
    setPlayerPlaying(true);
  };

  const changePlayerMode = (mode: PlayerMode) => {
    setPlayerMode(mode);
    setShuffleOrder(mode === 'shuffle' ? shuffleQueue(playerQueue, playerTrack) : []);
  };

  const startTrack = (next: Track) => {
    setPlayerTrack(next);
    setPlayerPlaying(true);
  };

  // 一首放完 → 按当前模式取下一首。单曲循环由播放器自己回段首，不会走到这里。
  const playNextTrack = () => {
    if (playerMode === 'repeat-one') return;

    if (playerMode === 'shuffle') {
      const next = nextInQueue(shuffleOrder, playerTrack);
      if (next) {
        startTrack(next);
        return;
      }
      // 一轮放完 → 重新洗牌接着放（当前曲目仍放首位，避免立刻重复）
      const reshuffled = shuffleQueue(playerQueue, playerTrack);
      setShuffleOrder(reshuffled);
      const restarted = nextInQueue(reshuffled, playerTrack);
      if (restarted) startTrack(restarted);
      else setPlayerPlaying(false);
      return;
    }

    const next = nextInQueue(playerQueue, playerTrack);
    if (next) {
      startTrack(next);
      return;
    }
    // 到队尾：列表循环绕回第一首，顺序播放停下
    const first = playerMode === 'repeat-all' ? playerQueue[0] : undefined;
    if (!first) {
      setPlayerPlaying(false);
      return;
    }
    if (first.id === playerTrack?.id) {
      // 队列只有一个曲目 → 换 track 不会触发重载，用 restartToken 让播放器重新加载
      setPlaybackRestart((value) => value + 1);
    }
    startTrack(first);
  };

  // 手动「下一首」：不受单曲循环影响；到队尾**原地不动**（不把播放停掉，只是没得可切）
  const skipNext = () => {
    const next = playerMode === 'shuffle'
      ? nextInQueue(shuffleOrder, playerTrack)
      : nextInQueue(playerQueue, playerTrack);
    if (next) {
      startTrack(next);
      return;
    }
    if (playerMode === 'shuffle') {
      // 随机播放走到一轮末尾 → 重新洗牌接着放
      const reshuffled = shuffleQueue(playerQueue, playerTrack);
      setShuffleOrder(reshuffled);
      const restarted = nextInQueue(reshuffled, playerTrack);
      if (restarted) startTrack(restarted);
      return;
    }
    if (playerMode === 'repeat-all' && playerQueue[0]) startTrack(playerQueue[0]);
  };

  // 手动「上一首」：到队首时列表循环绕到队尾，其余情况重播当前曲（多数播放器的习惯）
  const skipPrevious = () => {
    const queue = playerMode === 'shuffle' ? shuffleOrder : playerQueue;
    const previous = previousInQueue(queue, playerTrack);
    if (previous) {
      startTrack(previous);
      return;
    }
    const last = playerMode === 'repeat-all' ? queue[queue.length - 1] : undefined;
    if (last) {
      startTrack(last);
      return;
    }
    setPlaybackRestart((value) => value + 1);
  };

  const authenticate = async (token: string) => {
    const normalized = token.trim();
    if (!normalized) {
      setAuthError('请输入访问令牌');
      return;
    }
    setAuthError('正在验证访问令牌…');
    setAuthToken(normalized);
    try {
      await getSystem();
      setAuthError('');
      setAuthState('ready');
    } catch (error) {
      setAuthError(error instanceof APIError && error.status === 401 ? '访问令牌不正确' : (error instanceof Error ? error.message : '令牌验证失败'));
      setAuthState('required');
    }
  };

  if (authState !== 'ready') {
    return <AuthGate checking={authState === 'checking'} error={authError} onSubmit={authenticate} />;
  }

  return (
    <div className="app-shell">
      <TopBar
        page={route.page}
        version={version}
        onNavigate={navigatePage}
        playerTrack={playerTrack}
        playerPlaying={playerPlaying}
        onPlayerPlayingChange={setPlayerPlaying}
        onPlayerTrackEnded={playNextTrack}
        onPlayerPrevious={skipPrevious}
        onPlayerNext={skipNext}
        playerMode={playerMode}
        onPlayerModeChange={changePlayerMode}
        playerRestartToken={playbackRestart}
        onPlayerClose={() => { setPlayerPlaying(false); setPlayerTrack(null); setPlayerQueue([]); setShuffleOrder([]); }}
      />
      <main className="app-main">
        <AnimatePresence mode="wait">
          <motion.div
            key={route.page}
            className="page-motion"
            initial={{opacity: 0, y: 8}}
            animate={{opacity: 1, y: 0}}
            exit={{opacity: 0, y: -5}}
            transition={{duration: 0.22, ease: [0.22, 1, 0.36, 1]}}
          >
            {route.page === 'library' && <LibraryPage onOpenReview={openReview} onOpenSettings={() => navigatePage('settings')} onNotice={setNotice} playerTrackId={playerTrack?.id} playerTrack={playerTrack} playerPlaying={playerPlaying} onPlayTrack={playTrack} onTogglePlayer={() => setPlayerPlaying((value) => !value)} showGeneratedCovers={showGeneratedCovers} restoreDraft={restoreDraft} onRestoreDraftConsumed={() => setRestoreDraft(undefined)} onDiscardRestoreDraft={() => setRestoreDraft(undefined)} />}
            {route.page === 'review' && (
              <ReviewPage
                trackIds={route.batchIds}
                matchJobId={route.reviewJobId}
                showGeneratedCovers={showGeneratedCovers}
				onBack={() => route.reviewJobId ? openJobs(route.reviewJobId) : navigatePage('library')}
                onComplete={() => {
                  setNotice('批量写入任务已创建，正在等待安全写入');
                  openJobs();
                }}
                onJobQueued={(jobId) => openJobs(jobId)}
              />
            )}
            {route.page === 'jobs' && <JobsPage onOpenReview={openReviewJob} focusJobId={jobFocusId} />}
            {route.page === 'history' && <HistoryPage onNotice={setNotice} onLoadSnapshot={(request) => { setRestoreDraft(request); navigatePage('library'); }} showGeneratedCovers={showGeneratedCovers} />}
            {route.page === 'settings' && <SettingsPage onNotice={setNotice} showGeneratedCovers={showGeneratedCovers} onShowGeneratedCoversChange={setShowGeneratedCovers} theme={theme} onThemeChange={setTheme} font={font} onFontChange={setFont} />}
          </motion.div>
        </AnimatePresence>
      </main>

      <AnimatePresence>
        {notice && (
          <motion.div
            className="toast"
            initial={{opacity: 0, y: 18, scale: 0.98}}
            animate={{opacity: 1, y: 0, scale: 1}}
            exit={{opacity: 0, y: 8}}
          >
            <CheckCircle2 size={18} />
            <span>{notice}</span>
            <button title="关闭通知" onClick={() => setNotice(null)}><X size={15} /></button>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}

function AuthGate({checking, error, onSubmit}: {checking: boolean; error: string; onSubmit: (token: string) => Promise<void>}) {
  const [token, setToken] = useState('');
  return (
    <main className="auth-gate">
      <section className="auth-gate-card">
        <div className="auth-gate-icon"><KeyRound size={23} /></div>
        <div className="eyebrow">SINGLE USER ACCESS</div>
        <h1>输入访问令牌</h1>
        <p>此 Tagger 实例启用了单用户鉴权。未配置令牌时服务不会要求登录。</p>
        {checking ? (
          <div className="auth-gate-status"><LoaderCircle className="spin" size={16} /> 正在连接后台…</div>
        ) : (
          <form onSubmit={(event) => { event.preventDefault(); void onSubmit(token); }}>
            <label><span>访问令牌</span><input aria-label="访问令牌" type="password" value={token} onChange={(event) => setToken(event.target.value)} placeholder="TAGGER_AUTH_TOKEN" autoFocus /></label>
            <button className="primary-button" type="submit">验证并进入</button>
          </form>
        )}
        {error && <small className="auth-gate-error">{error}</small>}
      </section>
    </main>
  );
}
