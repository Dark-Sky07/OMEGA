import { describe, expect, it, vi } from 'vitest';
import { fireEvent, screen, waitFor } from '@testing-library/react';

import PanelUpdateModal from '@/pages/index/PanelUpdateModal';

import { renderWithProviders } from './test-utils';

const baseInfo = { currentVersion: '5.0.3', latestVersion: '5.0.4', updateAvailable: true };
const upToDateInfo = { currentVersion: '5.0.4', latestVersion: '5.0.4', updateAvailable: false };

function renderModal(props: Partial<Parameters<typeof PanelUpdateModal>[0]> = {}) {
  return renderWithProviders(
    <PanelUpdateModal
      open
      info={baseInfo}
      onClose={() => {}}
      onBusy={() => {}}
      {...props}
    />,
  );
}

describe('PanelUpdateModal', () => {
  it('shows the update button with the latest version when an update is available', () => {
    renderModal();
    expect(screen.getByText('Update Panel')).toBeTruthy();
    const btn = screen.getByRole('button', { name: /update 5\.0\.4/i });
    expect(btn).toBeTruthy();
    expect(!btn.classList.contains('ant-btn-disabled')).toBe(true);
    expect(screen.getByText('v5.0.3')).toBeTruthy();
    expect(screen.getByText('5.0.4')).toBeTruthy();
  });

  it('offers check-again and a releases link when up to date', async () => {
    const onRefresh = vi.fn();
    renderModal({ info: upToDateInfo, onRefresh });
    expect(screen.queryByRole('button', { name: /update 5\.0\.4/i })).toBeNull();
    expect(screen.getByText(/Panel is up to date/i)).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: /check again/i }));
    await waitFor(() => expect(onRefresh).toHaveBeenCalled());
    const releases = screen.getByRole('link', { name: /view releases/i });
    expect(releases.getAttribute('href')).toBe('https://github.com/Dark-Sky07/PHOENIX/releases');
  });

  it('shows a clear retry state when the version check failed', async () => {
    const onRefresh = vi.fn();
    renderModal({ checkFailed: true, onRefresh });
    expect(screen.getByText(/Could not check for updates/i)).toBeTruthy();
    expect(screen.queryByRole('button', { name: /update 5\.0\.4/i })).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: /check again/i }));
    await waitFor(() => expect(onRefresh).toHaveBeenCalled());
  });
});
