import {fireEvent, render, screen, waitFor, within} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {afterEach, beforeEach, describe, expect, it} from 'vitest';
import {App} from '@/App';
import {resetMockState} from '@/mock/api';
import {clearLibraryViewSnapshots} from '@/pages/libraryViewCache';

/** 播放器里显示的曲名（mock 模式没有真实音频地址，只能靠它判断换没换歌） */
const playerTitle = (player: HTMLElement) => player.querySelector('strong')?.textContent ?? '';
const flush = () => new Promise((resolve) => setTimeout(resolve, 0));

describe('Tagger app prototype', () => {
  beforeEach(() => {
    resetMockState();
    clearLibraryViewSnapshots();
    localStorage.clear();
    window.history.replaceState({}, '', '/');
  });

  afterEach(() => {
    resetMockState();
    clearLibraryViewSnapshots();
    localStorage.clear();
    window.history.replaceState({}, '', '/');
  });

  it('loads the TestMusic library and active track inspector', async () => {
    render(<App />);
    expect(screen.getByText('无播放歌曲')).toBeInTheDocument();
    expect(await screen.findByRole('heading', {name: '全部音乐'})).toBeInTheDocument();
    expect(await screen.findByRole('heading', {name: '愛上一個不回家的人'})).toBeInTheDocument();
    expect(screen.getByText('24')).toBeInTheDocument();
    expect(screen.queryByTitle('全局搜索快捷键')).not.toBeInTheDocument();
    expect(screen.queryByTitle('查看通知')).not.toBeInTheDocument();
    expect(screen.queryByTitle('账户与系统信息')).not.toBeInTheDocument();
  });

  it('keeps the navigation compact and leaves the far right for the player', async () => {
    render(<App />);
    const navigation = screen.getByRole('navigation', {name: '主导航'});
    const buttons = within(navigation).getAllByRole('button');
    expect(buttons[3]).toHaveTextContent('设置');
    expect(buttons).toHaveLength(4);
    expect(screen.queryByTitle('切换深色主题')).not.toBeInTheDocument();
    expect(screen.getByRole('region', {name: '全局播放器'})).toBeInTheDocument();
  });

  it('navigates between jobs, history and provider settings', async () => {
    const user = userEvent.setup();
    render(<App />);
    await screen.findByRole('heading', {name: '全部音乐'});

    await user.click(screen.getByRole('button', {name: /任务/}));
    expect(await screen.findByRole('heading', {name: '任务中心'})).toBeInTheDocument();

    await user.click(screen.getByRole('button', {name: '历史'}));
    expect(await screen.findByRole('heading', {name: '修改历史'})).toBeInTheDocument();

    await user.click(screen.getByRole('button', {name: '设置'}));
    expect(await screen.findByRole('heading', {name: '设置'})).toBeInTheDocument();
    await user.click(screen.getByRole('button', {name: /数据源/}));
    await waitFor(() => expect(screen.getByText('MusicBrainz')).toBeInTheDocument());
  });

  it('updates the browser URL and responds to browser back navigation', async () => {
    const user = userEvent.setup();
    render(<App />);
    await screen.findByRole('heading', {name: '全部音乐'});
    await user.click(screen.getByRole('button', {name: /任务/}));
    expect(window.location.pathname).toBe('/jobs');
    await user.click(screen.getByRole('button', {name: '历史'}));
    expect(window.location.pathname).toBe('/history');
    window.history.back();
    expect(await screen.findByRole('heading', {name: '任务中心'})).toBeInTheDocument();
  });

  it('keeps theme and font choices inside settings', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole('button', {name: '设置'}));
    await screen.findByRole('heading', {name: '设置'});
    await user.click(screen.getByRole('button', {name: /系统/}));
    await user.click(screen.getByRole('button', {name: /灰绿色/}));
    expect(document.documentElement.dataset.theme).toBe('sage');
    await user.selectOptions(screen.getByRole('combobox', {name: '界面字体'}), 'clean');
    expect(document.documentElement.dataset.font).toBe('clean');
  });

  it('keeps the global player mounted while changing pages', async () => {
    const user = userEvent.setup();
    render(<App />);
    await screen.findByRole('heading', {name: '愛上一個不回家的人'});

    await user.click(screen.getByTitle('试听'));
    expect(screen.getByRole('region', {name: '全局播放器'})).toBeInTheDocument();
    expect(screen.getByTitle('暂停播放')).toBeInTheDocument();
    // 模式按钮单键循环切换：顺序播放 → 列表循环 → 单曲循环 → 随机播放 → 顺序播放
    await user.click(screen.getByTitle('顺序播放（点击切换为列表循环）'));
    expect(screen.getByTitle('列表循环（点击切换为单曲循环）')).toBeInTheDocument();
    await user.click(screen.getByTitle('列表循环（点击切换为单曲循环）'));
    expect(screen.getByTitle('单曲循环（点击切换为随机播放）')).toBeInTheDocument();
    await user.click(screen.getByTitle('单曲循环（点击切换为随机播放）'));
    expect(screen.getByTitle('随机播放（点击切换为顺序播放）')).toBeInTheDocument();
    await user.click(screen.getByTitle('随机播放（点击切换为顺序播放）'));
    expect(screen.getByTitle('顺序播放（点击切换为列表循环）')).toBeInTheDocument();

    await user.click(screen.getByRole('button', {name: /任务/}));
    expect(await screen.findByRole('heading', {name: '任务中心'})).toBeInTheDocument();
    expect(screen.getByRole('region', {name: '全局播放器'})).toBeInTheDocument();
    expect(screen.getByTitle('暂停播放')).toBeInTheDocument();
  });

  it('auto-plays the next track in the visible list when the current one ends', async () => {
    const user = userEvent.setup();
    const {container} = render(<App />);
    await screen.findByRole('heading', {name: '愛上一個不回家的人'});

    await user.click(screen.getByTitle('试听'));
    const player = screen.getByRole('region', {name: '全局播放器'});
    expect(within(player).getByText('愛上一個不回家的人')).toBeInTheDocument();

    const audio = container.querySelector('audio');
    expect(audio).not.toBeNull();
    fireEvent.ended(audio!);

    await waitFor(() => expect(within(player).queryByText('愛上一個不回家的人')).not.toBeInTheDocument());
    expect(screen.getByTitle('暂停播放')).toBeInTheDocument();
  });

  it('wraps back to the first track after the last one ends in list-loop mode', async () => {
    const user = userEvent.setup();
    const {container} = render(<App />);
    await screen.findByRole('heading', {name: '愛上一個不回家的人'});

    await user.click(screen.getByTitle('试听'));
    const player = screen.getByRole('region', {name: '全局播放器'});
    const audio = container.querySelector('audio');
    expect(audio).not.toBeNull();

    // mock 模式不产生真实音频地址，只能按播放器里显示的曲名判断换没换歌
    const currentTitle = () => player.querySelector('strong')?.textContent ?? '';
    const first = currentTitle();
    expect(first).toBeTruthy();

    await user.click(screen.getByTitle('顺序播放（点击切换为列表循环）'));
    expect(screen.getByTitle('列表循环（点击切换为单曲循环）')).toBeInTheDocument();

    // 顺序往下放，直到绕回开头。能绕回来就说明队尾没有停下（顺序播放会停）。
    const seen = [first];
    for (let step = 0; step < 40; step += 1) {
      const before = currentTitle();
      fireEvent.ended(audio!);
      await waitFor(() => expect(currentTitle()).not.toBe(before));
      seen.push(currentTitle());
      if (currentTitle() === first) break;
    }

    expect(seen[seen.length - 1]).toBe(first);
    expect(new Set(seen.slice(0, -1)).size).toBe(seen.length - 1);
    expect(screen.getByTitle('暂停播放')).toBeInTheDocument();
  });

  it('stops at the end of the list when the mode is plain order playback', async () => {
    const user = userEvent.setup();
    const {container} = render(<App />);
    await screen.findByRole('heading', {name: '愛上一個不回家的人'});

    await user.click(screen.getByTitle('试听'));
    const audio = container.querySelector('audio');
    expect(audio).not.toBeNull();

    // 默认就是顺序播放：连放 30 次（> 列表长度）应已停在队尾，不会再绕回第一首
    for (let step = 0; step < 30; step += 1) {
      fireEvent.ended(audio!);
      await flush();
    }

    expect(screen.getByTitle('继续播放')).toBeInTheDocument();
  });

  it('walks the list back and forth with the 上一首 / 下一首 buttons', async () => {
    const user = userEvent.setup();
    render(<App />);
    await screen.findByRole('heading', {name: '愛上一個不回家的人'});

    await user.click(screen.getByTitle('试听'));
    const player = screen.getByRole('region', {name: '全局播放器'});
    const start = playerTitle(player);

    await user.click(screen.getByTitle('下一首'));
    await waitFor(() => expect(playerTitle(player)).not.toBe(start));

    await user.click(screen.getByTitle('上一首'));
    await waitFor(() => expect(playerTitle(player)).toBe(start));
    expect(screen.getByTitle('暂停播放')).toBeInTheDocument();
  });

  it('stays on the first track when 上一首 is pressed at the head of the list', async () => {
    const user = userEvent.setup();
    render(<App />);
    await screen.findByRole('heading', {name: '愛上一個不回家的人'});

    await user.click(screen.getByTitle('试听'));
    const player = screen.getByRole('region', {name: '全局播放器'});

    // 一直往前走到队首（顺序播放不环绕，标题不再变化即到顶）
    for (let step = 0; step < 40; step += 1) {
      const before = playerTitle(player);
      await user.click(screen.getByTitle('上一首'));
      await flush();
      if (playerTitle(player) === before) break;
    }

    const head = playerTitle(player);
    await user.click(screen.getByTitle('上一首'));
    await flush();
    expect(playerTitle(player)).toBe(head);
    expect(screen.getByTitle('暂停播放')).toBeInTheDocument();
  });

  it('jumps from the first track to the last one in list-loop mode', async () => {
    const user = userEvent.setup();
    render(<App />);
    await screen.findByRole('heading', {name: '愛上一個不回家的人'});

    await user.click(screen.getByTitle('试听'));
    const player = screen.getByRole('region', {name: '全局播放器'});

    // 先在顺序播放下走到队首（标题不再变化即到顶；列表循环下会一直往后绕，走不到）
    for (let step = 0; step < 40; step += 1) {
      const before = playerTitle(player);
      await user.click(screen.getByTitle('上一首'));
      await flush();
      if (playerTitle(player) === before) break;
    }
    const head = playerTitle(player);

    // 切到列表循环后再按上一首，应绕到队尾
    await user.click(screen.getByTitle('顺序播放（点击切换为列表循环）'));
    await user.click(screen.getByTitle('上一首'));
    await waitFor(() => expect(playerTitle(player)).not.toBe(head));
  });

  it('still moves on when 下一首 is pressed while single loop is on', async () => {
    const user = userEvent.setup();
    const {container} = render(<App />);
    await screen.findByRole('heading', {name: '愛上一個不回家的人'});

    await user.click(screen.getByTitle('试听'));
    const player = screen.getByRole('region', {name: '全局播放器'});
    const start = playerTitle(player);

    // 顺序播放 → 列表循环 → 单曲循环
    await user.click(screen.getByTitle('顺序播放（点击切换为列表循环）'));
    await user.click(screen.getByTitle('列表循环（点击切换为单曲循环）'));
    expect(screen.getByTitle('单曲循环（点击切换为随机播放）')).toBeInTheDocument();

    // 自动播完不切歌
    const audio = container.querySelector('audio');
    expect(audio).not.toBeNull();
    fireEvent.ended(audio!);
    await flush();
    expect(playerTitle(player)).toBe(start);

    // 手动点「下一首」照样切（手动操作不受单曲循环影响）
    await user.click(screen.getByTitle('下一首'));
    await waitFor(() => expect(playerTitle(player)).not.toBe(start));
  });

  it('searches metadata candidates for the active track', async () => {
    const user = userEvent.setup();
    render(<App />);
    await screen.findByRole('heading', {name: '愛上一個不回家的人'});

    await user.click(screen.getByRole('button', {name: /从数据源补全/}));
    expect(screen.getByText('正在查询已启用数据源')).toBeInTheDocument();
		expect(await screen.findByText('找到 4 个候选', {}, {timeout: 2000})).toBeInTheDocument();
		expect(screen.getAllByText(/智能选择/).length).toBeGreaterThan(0);
    expect(screen.getAllByText('MusicBrainz').length).toBeGreaterThan(0);
  });

  it('keeps the inspector aligned with the selected folder', async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(await screen.findByRole('button', {name: '许嵩 18'}));
    await user.click(await screen.findByRole('button', {name: '青年晚报 9'}));
    expect(await screen.findByRole('heading', {name: '奇谈'})).toBeInTheDocument();
  });

  it('filters the library by format and changes the track ordering', async () => {
    const user = userEvent.setup();
    render(<App />);
    await screen.findByRole('heading', {name: '全部音乐'});

    await user.selectOptions(screen.getByRole('combobox', {name: '曲目格式筛选'}), 'flac');
    expect(await screen.findByText('已加载 15 / 共 15 首')).toBeInTheDocument();
    await user.selectOptions(screen.getByRole('combobox', {name: '曲目排序'}), 'title');
    expect(screen.getByRole('combobox', {name: '曲目排序'})).toHaveValue('title');
  });
});
