import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { cleanup, render, screen, waitFor } from '@testing-library/react';

import { ThemeProvider } from '@/hooks/useTheme';
import { Status } from '@/models/status';
import { DBInbound } from '@/models/dbinbound';

const GB = 1024 ** 3;
const mockInbounds = [
  new DBInbound({ id: 1, remark: 'Reality-443', port: 443, protocol: 'vless', enable: true, up: 12 * GB, down: 80 * GB, total: 0, expiryTime: 0,
    settings: JSON.stringify({ clients: [], decryption: 'none' }), streamSettings: JSON.stringify({ network: 'tcp', security: 'reality', realitySettings: {} }), sniffing: '{}', allocate: '{}' } as never),
  new DBInbound({ id: 3, remark: 'OpenVPN-1194', port: 1194, protocol: 'openvpn', enable: true, up: 1 * GB, down: 9 * GB, total: 100 * GB, expiryTime: Date.now() + 86400000 * 20,
    settings: JSON.stringify({ clients: [] }), streamSettings: '', sniffing: '{}', allocate: '{}' } as never),
  new DBInbound({ id: 15, remark: 'L2TP', port: 1701, protocol: 'l2tp', enable: false, up: 0, down: 0, total: 0, expiryTime: 0,
    settings: JSON.stringify({ clients: [] }), streamSettings: '', sniffing: '{}', allocate: '{}' } as never),
];

vi.mock('@/pages/inbounds/useInbounds', () => ({
  useInbounds: () => ({
    fetched: true,
    fetchError: '',
    dbInbounds: mockInbounds,
    clientCount: { 1: { clients: 4, active: ['a@x', 'b@x'], deactive: ['c@x'], depleted: ['d@x'], expiring: [], online: ['a@x'] } },
    onlineClients: ['a@x'],
    lastOnlineMap: {},
    statsVersion: 0,
    totals: { up: 13 * GB, down: 89 * GB },
    expireDiff: 0,
    trafficDiff: 0,
    subSettings: { enable: true },
    remarkModel: '-ieo',
    datepicker: 'gregorian',
    tgBotEnable: false,
    ipLimitEnable: false,
    pageSize: 0,
    refresh: async () => {},
    hydrateInbound: async () => null,
    applyTrafficEvent: () => {},
    applyClientStatsEvent: () => {},
  }),
}));

vi.mock('@/hooks/useWebSocket', () => ({ useWebSocket: () => {} }));

const noop = async () => ({ success: true, msg: '', obj: null });
const mockClients = [
  { id: 1, email: 'ali@omega', subId: 'sub-ali', uuid: 'u1', totalGB: 50 * GB, expiryTime: Date.now() + 10 * 86400000, enable: true, inboundIds: [1, 3], group: 'vip', comment: 'tg:@ali',
    traffic: { up: 2 * GB, down: 20 * GB, total: 50 * GB, enable: true, lastOnline: Date.now() - 60000 } },
  { id: 2, email: 'sara@omega', subId: 'sub-sara', uuid: 'u2', totalGB: 0, expiryTime: 0, enable: true, inboundIds: [1], traffic: { up: GB, down: 3 * GB, total: 0, enable: true } },
  { id: 3, email: 'reza@omega', subId: 'sub-reza', uuid: 'u3', totalGB: 10 * GB, expiryTime: Date.now() - 86400000, enable: false, inboundIds: [15], traffic: { up: 5 * GB, down: 5 * GB, total: 10 * GB, enable: false } },
];
vi.mock('@/hooks/useClients', () => ({
  useClients: () => ({
    clients: mockClients, total: 3, filtered: 3,
    summary: { total: 3, active: 2, online: ['ali@omega'], depleted: ['reza@omega'], expiring: ['ali@omega'], deactive: ['reza@omega'] },
    allGroups: ['vip'],
    hydrate: async () => null,
    ownerOf: () => undefined,
    query: {}, setQuery: () => {},
    inbounds: [
      { id: 1, remark: 'Reality-443', tag: 'inbound-443', protocol: 'vless', port: 443 },
      { id: 3, remark: 'OpenVPN-1194', tag: 'inbound-1194', protocol: 'openvpn', port: 1194 },
      { id: 15, remark: 'L2TP', tag: 'inbound-1701', protocol: 'l2tp', port: 1701 },
    ],
    onlines: ['ali@omega'], loading: false, fetched: true, fetchError: '',
    subSettings: { enable: true }, ipLimitEnable: false, tgBotEnable: false, expireDiff: 0, trafficDiff: 0, pageSize: 0,
    refresh: async () => {}, create: noop, bulkCreate: noop, update: noop, remove: noop, bulkDelete: noop, bulkAdjust: noop,
    bulkAddToGroup: noop, bulkRemoveFromGroup: noop, attach: noop, bulkAttach: noop, detach: noop, bulkDetach: noop,
    resetTraffic: noop, resetAllTraffics: noop, delDepleted: noop, setEnable: noop,
    applyTrafficEvent: () => {}, applyClientStatsEvent: () => {},
  }),
}));
vi.mock('@/api/queries/useNodesQuery', () => ({
  useNodesQuery: () => ({ nodes: [], totals: { online: 0, offline: 0, avgLatency: 0 }, loading: false, error: null, refetch: async () => {} }),
}));

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

  it('renders the inbounds page without runtime errors', async () => {
    const { default: InboundsPage } = await import('@/pages/inbounds/InboundsPage');
    const { container } = wrap(<InboundsPage />);
    await waitFor(() => expect(container.querySelector('.inbound-list-card')).toBeTruthy());
    expect(container.querySelector('.omega-page-header')).toBeTruthy();
    expect(container.querySelectorAll('.omega-stat').length).toBe(5);
    expect(container.querySelectorAll('.ant-table-row').length).toBe(3);
    expect(container.querySelectorAll('.omega-chip.is-protocol').length).toBe(3);
    expect(container.querySelectorAll('.client-traffic-cell').length).toBe(3);
    expect(screen.getByText('Reality-443')).toBeTruthy();
    const realErrors = errors.filter((a) => !String(a[0]).includes('act(') && !String(a[0]).includes('ResizeObserver'));
    expect(realErrors, JSON.stringify(realErrors.map((a) => String(a[0]).slice(0, 300)))).toEqual([]);
  });

  it('renders the clients page without runtime errors', async () => {
    const { default: ClientsPage } = await import('@/pages/clients/ClientsPage');
    const { container } = wrap(<ClientsPage />);
    await waitFor(() => expect(container.querySelector('.clients-card')).toBeTruthy());
    expect(container.querySelector('.omega-page-header')).toBeTruthy();
    expect(container.querySelectorAll('.omega-stat').length).toBe(6);
    expect(container.querySelectorAll('.ant-table-row').length).toBe(3);
    expect(container.querySelectorAll('.ant-table-row .omega-pill').length).toBe(3);
    expect(screen.getByText('ali@omega')).toBeTruthy();
    const realErrors = errors.filter((a) => !String(a[0]).includes('act(') && !String(a[0]).includes('ResizeObserver'));
    expect(realErrors, JSON.stringify(realErrors.map((a) => String(a[0]).slice(0, 300)))).toEqual([]);
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
