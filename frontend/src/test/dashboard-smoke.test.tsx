import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { cleanup, render, screen, waitFor } from '@testing-library/react';

import { ThemeProvider } from '@/hooks/useTheme';
import { Status } from '@/models/status';

const statusObj = {
  cpu: 5.8, cpuCores: 4, logicalPro: 8, cpuSpeedMhz: 2900,
  disk: { current: 65820373811.2, total: 171798691840 },
  loads: [0.42, 0.51, 0.47],
  mem: { current: 3328599654.4, total: 8589934592 },
  netIO: { up: 2263899, down: 7579498 },
  netTraffic: { sent: 1331445393576, recv: 4224690142682 },
  publicIP: { ipv4: '185.92.14.37', ipv6: '2a02:4780:12:ab::1' },
  swap: { current: 644245094.4, total: 2147483648 },
  tcpCount: 392, udpCount: 107, uptime: 3197021, appUptime: 266400,
  appStats: { threads: 23, mem: 155189248, uptime: 266400 },
  xray: { state: 'running', errorMsg: '', version: '25.9.11', color: 'green' },
  openvpn: { state: 'running', errorMsg: '', inboundCount: 1, onlineClients: 14, manualStop: false, color: 'green' },
  l2tp: { state: 'stop', errorMsg: '', inboundCount: 1, onlineClients: 0, manualStop: true, color: 'orange' },
};

vi.mock('@/api/queries/useSession', () => ({
  useSession: () => ({
    session: { role: 'admin', username: 'admin', name: 'Admin' },
    role: 'admin', isAdmin: true, isReseller: false, loading: false, error: null,
  }),
}));

vi.mock('@/api/queries/useAllSettings', () => ({
  useAllSettings: () => ({ allSetting: { subJsonEnable: false, subClashEnable: false } }),
}));

vi.mock('@/api/queries/useStatusQuery', () => ({
  useStatusQuery: () => ({
    status: new Status(statusObj as never),
    fetched: true,
    fetchError: '',
    refresh: async () => {},
  }),
}));

vi.mock('@/utils', async (importOriginal) => {
  const mod = await importOriginal<typeof import('@/utils')>();
  return {
    ...mod,
    HttpUtil: {
      ...mod.HttpUtil,
      get: vi.fn(async () => ({ success: true, msg: '', obj: {} })),
      post: vi.fn(async () => ({ success: true, msg: '', obj: {} })),
    },
  };
});

function wrap(ui: React.ReactNode) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <MemoryRouter initialEntries={['/']}>{ui}</MemoryRouter>
      </ThemeProvider>
    </QueryClientProvider>,
  );
}

describe('smoke: redesigned dashboard + login', () => {
  const errors: unknown[][] = [];
  beforeEach(() => {
    errors.length = 0;
    vi.spyOn(console, 'error').mockImplementation((...args: unknown[]) => { errors.push(args); });
  });
  afterEach(() => { cleanup(); vi.restoreAllMocks(); });

  it('renders the dashboard without runtime errors', async () => {
    const { default: IndexPage } = await import('@/pages/index/IndexPage');
    const { container } = wrap(<IndexPage />);
    expect(container.querySelector('.dash-hero')).toBeTruthy();
    expect(container.querySelectorAll('.resource-tile').length).toBe(4);
    expect(container.querySelectorAll('.service-row').length).toBeGreaterThanOrEqual(3);
    expect(screen.getAllByText('Xray').length).toBeGreaterThan(0);
    expect(screen.getAllByText('OpenVPN').length).toBeGreaterThan(0);
    expect(screen.getAllByText(/L2TP/).length).toBeGreaterThan(0);
    expect(screen.getByText('185.92.14.37')).toBeTruthy();
    const realErrors = errors.filter((a) => !String(a[0]).includes('act(') && !String(a[0]).includes('ResizeObserver'));
    expect(realErrors, JSON.stringify(realErrors.map((a) => String(a[0]).slice(0, 200)))).toEqual([]);
  });

  it('renders the login stage without runtime errors', async () => {
    const { default: LoginPage } = await import('@/pages/login/LoginPage');
    const { container } = wrap(<LoginPage />);
    await waitFor(() => expect(container.querySelector('.login-stage')).toBeTruthy());
    expect(container.querySelector('.login-aside')).toBeTruthy();
    expect(container.querySelector('form')).toBeTruthy();
    const realErrors = errors.filter((a) => !String(a[0]).includes('act('));
    expect(realErrors, JSON.stringify(realErrors.map((a) => String(a[0]).slice(0, 200)))).toEqual([]);
  });
});
