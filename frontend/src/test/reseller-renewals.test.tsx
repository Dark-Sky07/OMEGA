import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';

import { ClipboardManager, HttpUtil } from '@/utils';
import { ThemeProvider } from '@/hooks/useTheme';
import ResellerRenewalsPage from '@/pages/resellers/ResellerRenewalsPage';

vi.mock('@/layouts/AppSidebar', () => ({ default: () => <div data-testid="sidebar" /> }));
vi.mock('@/api/queries/useAllSettings', () => ({
  useAllSettings: () => ({ allSetting: { checkoutEnable: true } }),
}));

function ok<T>(obj: T) {
  return { success: true, msg: '', obj } as never;
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider><ResellerRenewalsPage /></ThemeProvider>
    </QueryClientProvider>,
  );
}

describe('reseller renewal center', () => {
  let now: number;

  beforeEach(() => {
    now = Date.now();
    vi.spyOn(HttpUtil, 'get').mockImplementation(((url: string) => {
      if (url === '/panel/api/reseller/report') {
        return Promise.resolve(ok({
          stat: { reseller: { id: 1, username: 'ali', name: 'Ali' } },
          clients: [
            { email: 'expiring@example.com', comment: 'Customer A', enable: true, totalBytes: 10 * 1024 ** 3, expiryTime: now + 3 * 24 * 60 * 60 * 1000, inboundIds: [] },
            { email: 'expired@example.com', enable: true, totalBytes: 0, expiryTime: now - 24 * 60 * 60 * 1000, inboundIds: [] },
            { email: 'later@example.com', enable: true, totalBytes: 0, expiryTime: now + 90 * 24 * 60 * 60 * 1000, inboundIds: [] },
          ],
          transactions: [],
        }));
      }
      if (url.startsWith('/panel/api/clients/checkout-link/')) {
        return Promise.resolve(ok({ token: 'signed-token' }));
      }
      return Promise.resolve(ok({}));
    }) as never);
    vi.spyOn(ClipboardManager, 'copyText').mockResolvedValue(true);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('shows expired and upcoming customers for follow-up', async () => {
    renderPage();

    expect(await screen.findByText('expiring@example.com')).toBeTruthy();
    expect(screen.getByText('expired@example.com')).toBeTruthy();
    expect(screen.getByText('later@example.com')).toBeTruthy();
    expect(screen.getByText('Expiring soon')).toBeTruthy();
    expect(screen.getAllByText('Expired').length).toBeGreaterThan(0);
  });

  it('copies a signed checkout link for a reseller-owned customer', async () => {
    renderPage();

    await screen.findByText('expiring@example.com');
    const actions = screen.getAllByRole('button', { name: /Copy renewal link/ });
    fireEvent.click(actions[0]);

    await waitFor(() => expect(ClipboardManager.copyText).toHaveBeenCalledWith(
      `http://localhost:3000/panel/checkout?ref=signed-token`,
    ));
    expect(HttpUtil.get).toHaveBeenCalledWith(
      '/panel/api/clients/checkout-link/expiring%40example.com',
      undefined,
      { silent: true },
    );
  }, 15_000);
});
