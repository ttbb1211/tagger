import {render, screen} from '@testing-library/react';
import {describe, expect, it} from 'vitest';
import {TopBar} from '@/components/TopBar';

function renderTopBar(version?: string) {
  return render(
    <TopBar
      page="library"
      version={version}
      onNavigate={() => undefined}
      playerTrack={null}
      playerPlaying={false}
      onPlayerPlayingChange={() => undefined}
      onPlayerClose={() => undefined}
    />,
  );
}

describe('TopBar', () => {
  it('shows the injected version next to the brand', () => {
    renderTopBar('1.5.0');
    expect(screen.getByText('v1.5.0')).toBeInTheDocument();
  });

  it('falls back to the brand tagline when the version is unknown', () => {
    renderTopBar();
    expect(screen.getByText('MUSIC ARCHIVE / 01')).toBeInTheDocument();
  });
});
