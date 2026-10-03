import {fireEvent, render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {describe, expect, it, vi} from 'vitest';
import {TrackInspector} from '@/components/library/TrackInspector';
import {seedTracks} from '@/mock/data';
import type {Track, TrackPatch} from '@/types';

describe('TrackInspector', () => {
  it('keeps path hints out of real fields until the user explicitly applies one', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn().mockResolvedValue(undefined);
    const hintedTrack: Track = {
      ...seedTracks[0],
      title: '', artists: [], album: '', albumArtists: [],
      health: 'tag-compatibility',
      tagIssues: ['missing-embedded-title', 'missing-embedded-artist'],
      tagHints: [
        {title: '宮崎歩', artists: ['brave heart'], albumArtists: ['brave heart'], source: 'filename', pattern: 'title-artist'},
        {title: 'brave heart', artists: ['宮崎歩'], albumArtists: ['宮崎歩'], source: 'filename', pattern: 'artist-title'},
      ],
    };
    render(
      <TrackInspector
        track={hintedTrack}
        saving={false}
        mobileOpen
        onCloseMobile={() => {}}
        onSearch={() => {}}
        onSave={onSave}
        onArtworkChange={vi.fn().mockResolvedValue(undefined)}
      />,
    );

    expect(screen.getByLabelText('标题')).toHaveValue('');
    expect(screen.getByLabelText(/^艺术家/)).toHaveValue('');
    expect(screen.getByRole('button', {name: '保存修改'})).toBeDisabled();

    await user.click(screen.getByRole('button', {name: /brave heart · 宮崎歩.*艺术家 - 标题/}));
    expect(screen.getByLabelText('标题')).toHaveValue('brave heart');
    expect(screen.getByLabelText(/^艺术家/)).toHaveValue('宮崎歩');
    expect(screen.getByLabelText('专辑艺术家')).toHaveValue('宮崎歩');

    await user.click(screen.getByRole('button', {name: '保存修改'}));
    await user.click(screen.getByRole('button', {name: '确认写入'}));
    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({
      title: 'brave heart', artists: ['宮崎歩'], albumArtists: ['宮崎歩'],
    }), {writeTag: true, exportLrc: false});
  });

  it('keeps the historical snapshot notice compact and dismissible', async () => {
    const user = userEvent.setup();
    const onDiscard = vi.fn();
    const patch = {
      title: '历史标题', artists: [], album: '', albumArtists: [], genres: [], lyrics: '', comment: '',
      composers: [], conductor: '', lyricists: [], copyright: '', isrc: '', musicbrainzTrackId: '',
      musicbrainzReleaseId: '', musicbrainzArtistIds: [], acoustidId: '', acoustidFingerprint: '',
    } as TrackPatch;
    const {container} = render(
      <TrackInspector
        track={seedTracks[0]}
        saving={false}
        mobileOpen
        onCloseMobile={() => {}}
        onSearch={() => {}}
        onSave={vi.fn().mockResolvedValue(undefined)}
        onArtworkChange={vi.fn().mockResolvedValue(undefined)}
        restoreDraft={{key: 'rev-1', revisionId: 'rev-1', trackId: seedTracks[0].id, patch, label: '历史修订 rev-1'}}
        onDiscardRestoreDraft={onDiscard}
      />,
    );

    expect(container.querySelector('.inspector-banners .restore-draft-banner')).toBeInTheDocument();
    expect(screen.getByText('已加载历史快照')).toBeInTheDocument();
    await user.click(screen.getByRole('button', {name: '取消加载'}));
    expect(onDiscard).toHaveBeenCalledTimes(1);
  });

  it('previews and submits field-level edits', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(
      <TrackInspector
        track={seedTracks[0]}
        saving={false}
        mobileOpen
        onCloseMobile={() => {}}
        onSearch={() => {}}
        onSave={onSave}
		onArtworkChange={vi.fn().mockResolvedValue(undefined)}
      />,
    );

    const title = screen.getByLabelText('标题');
    await user.clear(title);
    await user.type(title, '再回首（修订）');
    await user.click(screen.getByRole('button', {name: '保存修改'}));

    expect(screen.getByText('确认写入 1 项修改')).toBeInTheDocument();
    expect(screen.getByText('再回首（修订）')).toBeInTheDocument();

    await user.click(screen.getByRole('button', {name: '确认写入'}));
    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({title: '再回首（修订）'}), {writeTag: true, exportLrc: false});
  });

  it('saves edited lyrics to the embedded audio tag and can additionally export a .lrc', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(
      <TrackInspector
        track={seedTracks[0]}
        saving={false}
        mobileOpen
        onCloseMobile={() => {}}
        onSearch={() => {}}
        onSave={onSave}
        onArtworkChange={vi.fn().mockResolvedValue(undefined)}
      />,
    );

    await user.click(screen.getByRole('tab', {name: '歌词'}));
    const lyrics = screen.getByPlaceholderText(/在这里输入歌词/);
    fireEvent.change(lyrics, {target: {value: '[00:01.00] embedded only'}});
    const lrc = screen.getByRole('checkbox', {name: /同时导出 \.lrc 歌词文件/});
    expect(lrc).not.toBeChecked();
    await user.click(lrc);
    await user.click(screen.getByRole('button', {name: '保存修改'}));
    await user.click(screen.getByRole('button', {name: '确认写入'}));

    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({lyrics: '[00:01.00] embedded only'}), {writeTag: true, exportLrc: true});
  });

  it('leaves .lrc export optional for cue virtual tracks, where lyrics cannot be embedded', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn().mockResolvedValue(undefined);
    const cueTrack: Track = {
      ...seedTracks[0],
      cuePath: 'Album/整轨.flac',
      startOffsetSeconds: 120,
      endOffsetSeconds: 300,
      durationSeconds: 180,
    };
    render(
      <TrackInspector
        track={cueTrack}
        saving={false}
        mobileOpen
        onCloseMobile={() => {}}
        onSearch={() => {}}
        onSave={onSave}
        onArtworkChange={vi.fn().mockResolvedValue(undefined)}
      />,
    );

    await user.click(screen.getByRole('tab', {name: '歌词'}));
    // 整轨 + 只处理歌词：不预勾「写入 cue 标签」，避免顺带重写整条 cue 标签
    expect(screen.getByRole('checkbox', {name: /写入 cue 标签/})).not.toBeChecked();
    const lrc = screen.getByRole('checkbox', {name: /导出 \.lrc 歌词文件/});
    expect(lrc).not.toBeChecked();
    expect(lrc).not.toBeDisabled();

    fireEvent.change(screen.getByPlaceholderText(/在这里输入歌词/), {target: {value: '[00:02.00] cue lyrics'}});
    await user.click(screen.getByRole('button', {name: '保存修改'}));
    await user.click(screen.getByRole('button', {name: '确认写入'}));

    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({lyrics: '[00:02.00] cue lyrics'}), {writeTag: false, exportLrc: false});
  });

  it('keeps the cue tag write checked on whole-track items once metadata is edited', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn().mockResolvedValue(undefined);
    const cueTrack: Track = {
      ...seedTracks[0],
      cuePath: 'Album/整轨.flac',
      startOffsetSeconds: 120,
      endOffsetSeconds: 300,
      durationSeconds: 180,
    };
    render(
      <TrackInspector
        track={cueTrack}
        saving={false}
        mobileOpen
        onCloseMobile={() => {}}
        onSearch={() => {}}
        onSave={onSave}
        onArtworkChange={vi.fn().mockResolvedValue(undefined)}
      />,
    );

    // 动了元数据（标题）→ 复选框必须回到勾选，否则整轨的元数据将无从保存
    fireEvent.change(screen.getByLabelText('标题'), {target: {value: '再回首（整轨）'}});
    await user.click(screen.getByRole('tab', {name: '歌词'}));
    expect(screen.getByRole('checkbox', {name: /写入 cue 标签/})).toBeChecked();

    await user.click(screen.getByRole('button', {name: '保存修改'}));
    await user.click(screen.getByRole('button', {name: '确认写入'}));

    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({title: '再回首（整轨）'}), {writeTag: true, exportLrc: false});
  });

  it('honours a manual toggle of the cue tag write on whole-track items', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn().mockResolvedValue(undefined);
    const cueTrack: Track = {
      ...seedTracks[0],
      cuePath: 'Album/整轨.flac',
      startOffsetSeconds: 120,
      endOffsetSeconds: 300,
      durationSeconds: 180,
    };
    render(
      <TrackInspector
        track={cueTrack}
        saving={false}
        mobileOpen
        onCloseMobile={() => {}}
        onSearch={() => {}}
        onSave={onSave}
        onArtworkChange={vi.fn().mockResolvedValue(undefined)}
      />,
    );

    await user.click(screen.getByRole('tab', {name: '歌词'}));
    const writeCue = screen.getByRole('checkbox', {name: /写入 cue 标签/});
    expect(writeCue).not.toBeChecked();
    await user.click(writeCue);
    expect(writeCue).toBeChecked();

    fireEvent.change(screen.getByPlaceholderText(/在这里输入歌词/), {target: {value: '[00:03.00] cue lyrics'}});
    await user.click(screen.getByRole('button', {name: '保存修改'}));
    await user.click(screen.getByRole('button', {name: '确认写入'}));

    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({lyrics: '[00:03.00] cue lyrics'}), {writeTag: true, exportLrc: false});
  });

  it('exports .lrc for cue virtual tracks when the user opts in', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn().mockResolvedValue(undefined);
    const cueTrack: Track = {
      ...seedTracks[0],
      cuePath: 'Album/整轨.flac',
      startOffsetSeconds: 120,
      endOffsetSeconds: 300,
      durationSeconds: 180,
    };
    render(
      <TrackInspector
        track={cueTrack}
        saving={false}
        mobileOpen
        onCloseMobile={() => {}}
        onSearch={() => {}}
        onSave={onSave}
        onArtworkChange={vi.fn().mockResolvedValue(undefined)}
      />,
    );

    await user.click(screen.getByRole('tab', {name: '歌词'}));
    const lrc = screen.getByRole('checkbox', {name: /导出 \.lrc 歌词文件/});
    await user.click(lrc);
    expect(lrc).toBeChecked();

    fireEvent.change(screen.getByPlaceholderText(/在这里输入歌词/), {target: {value: '[00:02.00] cue lyrics'}});
    await user.click(screen.getByRole('button', {name: '保存修改'}));
    await user.click(screen.getByRole('button', {name: '确认写入'}));

    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({lyrics: '[00:02.00] cue lyrics'}), {writeTag: false, exportLrc: true});
  });

  it('edits extended embedded fields without changing unknown raw tags', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(
      <TrackInspector
        track={seedTracks[0]}
        saving={false}
        mobileOpen
        onCloseMobile={() => {}}
        onSearch={() => {}}
        onSave={onSave}
        onArtworkChange={vi.fn().mockResolvedValue(undefined)}
      />,
    );

    await user.click(screen.getByRole('button', {name: /扩展内嵌字段/}));
    await user.type(screen.getByLabelText('注释'), 'liner note');
    await user.type(screen.getByLabelText('BPM'), '128');
    await user.type(screen.getByLabelText('MusicBrainz Track ID'), 'track-mbid');
    await user.click(screen.getByRole('button', {name: '保存修改'}));
    await user.click(screen.getByRole('button', {name: '确认写入'}));

    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({
      comment: 'liner note', bpm: 128, musicbrainzTrackId: 'track-mbid',
    }), {writeTag: true, exportLrc: false});
  });

  it('uploads artwork and requires a second click before deletion', async () => {
	const user = userEvent.setup();
	const onArtworkChange = vi.fn().mockResolvedValue(undefined);
	const {container} = render(
	  <TrackInspector
		track={seedTracks[0]}
		saving={false}
		mobileOpen
		onCloseMobile={() => {}}
		onSearch={() => {}}
		onSave={vi.fn().mockResolvedValue(undefined)}
		onArtworkChange={onArtworkChange}
	  />,
	);
	await user.click(screen.getByRole('tab', {name: '封面'}));
	const fileInput = container.querySelector<HTMLInputElement>('input[type="file"]');
	if (!fileInput) throw new Error('missing artwork file input');
	const file = new File([new Uint8Array([137, 80, 78, 71])], 'cover.png', {type: 'image/png'});
	await user.upload(fileInput, file);
	expect(onArtworkChange).toHaveBeenCalledWith(file);

	await user.click(screen.getByRole('button', {name: '删除当前封面'}));
	expect(onArtworkChange).toHaveBeenCalledTimes(1);
    await user.click(screen.getByRole('button', {name: '再次点击确认删除'}));
    expect(onArtworkChange).toHaveBeenLastCalledWith(null);
  });

  it('keeps the mock preview player interactive', async () => {
    const user = userEvent.setup();
    render(
      <TrackInspector
        track={seedTracks[0]}
        saving={false}
        mobileOpen
        onCloseMobile={() => {}}
        onSearch={() => {}}
        onSave={vi.fn().mockResolvedValue(undefined)}
        onArtworkChange={vi.fn().mockResolvedValue(undefined)}
      />,
    );

    await user.click(screen.getByTitle('试听'));
    expect(screen.getByTitle('暂停试听')).toBeInTheDocument();
    await user.click(screen.getByTitle('暂停试听'));
    expect(screen.getByTitle('试听')).toBeInTheDocument();
  });

  it('copies the relative path and reports the result', async () => {
    const user = userEvent.setup();
    const onNotice = vi.fn();
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', {configurable: true, value: {writeText}});
    render(
      <TrackInspector
        track={seedTracks[0]}
        saving={false}
        mobileOpen
        onCloseMobile={() => {}}
        onSearch={() => {}}
        onSave={vi.fn().mockResolvedValue(undefined)}
        onArtworkChange={vi.fn().mockResolvedValue(undefined)}
        onNotice={onNotice}
      />,
    );

    await user.click(screen.getByRole('tab', {name: '技术'}));
    await user.click(screen.getByRole('button', {name: '复制相对路径'}));
    expect(writeText).toHaveBeenCalledWith(seedTracks[0].relativePath);
    expect(onNotice).toHaveBeenCalledWith('相对路径已复制');
  });

  it('opens the raw tag panel in mock mode', async () => {
    const user = userEvent.setup();
    render(
      <TrackInspector
        track={seedTracks[0]}
        saving={false}
        mobileOpen
        onCloseMobile={() => {}}
        onSearch={() => {}}
        onSave={vi.fn().mockResolvedValue(undefined)}
        onArtworkChange={vi.fn().mockResolvedValue(undefined)}
      />,
    );

    await user.click(screen.getByRole('button', {name: '查看原始标签'}));
    expect(await screen.findByText('TagLib PropertyMap')).toBeInTheDocument();
    expect(screen.getByText('TITLE')).toBeInTheDocument();
    expect(screen.getAllByText('再回首').length).toBeGreaterThanOrEqual(1);
  });

  it('opens a lyrics-focused provider search from the lyrics tab', async () => {
    const user = userEvent.setup();
    const onSearch = vi.fn();
    render(
      <TrackInspector
        track={seedTracks[0]}
        saving={false}
        mobileOpen
        onCloseMobile={() => {}}
        onSearch={onSearch}
        onSave={vi.fn().mockResolvedValue(undefined)}
        onArtworkChange={vi.fn().mockResolvedValue(undefined)}
      />,
    );

    await user.click(screen.getByRole('tab', {name: '歌词'}));
    await user.click(screen.getByRole('button', {name: '查找歌词'}));
    expect(onSearch).toHaveBeenCalledWith('lyrics');
  });
});

function renderInspector(overrides: Partial<Parameters<typeof TrackInspector>[0]> = {}) {
  return render(
    <TrackInspector
      track={seedTracks[0]}
      saving={false}
      mobileOpen
      onCloseMobile={() => {}}
      onSearch={() => {}}
      onSave={vi.fn().mockResolvedValue(undefined)}
      onArtworkChange={vi.fn().mockResolvedValue(undefined)}
      {...overrides}
    />,
  );
}

describe('TrackInspector 正在播放提示', () => {
  // 老板的困惑来源：播放器在播「無心睡眠」，面板标题却停在上次点过的曲目。
  // 两种情形必须分开表达 —— 面板就是当前曲时改 eyebrow，不是当前曲时给横幅。
  it('面板就是正在播放的那首时，eyebrow 直接写 NOW PLAYING', () => {
    const {container} = renderInspector({playerTrackId: seedTracks[0].id, playerPlaying: true});
    const eyebrow = container.querySelector('.eyebrow.is-playing');
    expect(eyebrow).not.toBeNull();
    expect(eyebrow).toHaveTextContent('NOW PLAYING');
    expect(container.querySelector('.now-playing-banner')).toBeNull();
  });

  it('在看别首而播放器还在播时，给出横幅并支持一键切回', async () => {
    const user = userEvent.setup();
    const onSelectTrack = vi.fn();
    const other: Track = {...seedTracks[1], id: 'trk-other', title: '無心睡眠'};
    const {container} = renderInspector({
      playerTrackId: other.id,
      playerTrack: other,
      playerPlaying: true,
      onSelectTrack,
    });

    const banner = container.querySelector('.now-playing-banner');
    expect(banner).not.toBeNull();
    expect(banner).toHaveTextContent('無心睡眠');
    const inspector = container.querySelector('.track-inspector');
    const banners = inspector?.querySelector('.inspector-banners');
    const children = [...(inspector?.children ?? [])];
    expect(banners?.contains(banner as HTMLDivElement)).toBe(true);
    expect(children.indexOf(banners as Element)).toBeLessThan(children.findIndex((child) => child.classList.contains('inspector-tabs')));
    expect(children.findIndex((child) => child.classList.contains('inspector-tabs'))).toBeLessThan(children.findIndex((child) => child.classList.contains('inspector-scroll')));
    expect(children.findIndex((child) => child.classList.contains('inspector-scroll'))).toBeLessThan(children.findIndex((child) => child.classList.contains('inspector-actions')));
    await user.click(screen.getByRole('button', {name: '查看这首'}));
    expect(onSelectTrack).toHaveBeenCalledWith('trk-other');
  });

  it('暂停中只要在播那一首，横幅与 eyebrow 的逻辑不变（状态跟曲子而非跟播放键）', () => {
    const other: Track = {...seedTracks[1], id: 'trk-other', title: '拒絕再玩'};
    const {container} = renderInspector({playerTrackId: other.id, playerTrack: other, playerPlaying: false});
    expect(container.querySelector('.eyebrow')?.textContent).toContain('NOW INSPECTING');
    expect(container.querySelector('.now-playing-banner')).not.toBeNull();
  });

  it('没有在播曲目时不出现任何播放提示', () => {
    const {container} = renderInspector({playerTrackId: undefined, playerTrack: null});
    expect(container.querySelector('.eyebrow.is-playing')).toBeNull();
    expect(container.querySelector('.now-playing-banner')).toBeNull();
  });
});
