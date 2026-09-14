import { beforeEach, describe, expect, it, vi } from 'vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import type { ReactNode } from 'react';

import { HttpUtil } from '@/utils';
import { ThemeProvider } from '@/hooks/useTheme';
import { sections as endpointsSections } from '@/pages/api-docs/endpoints';

import ResellersPage from '@/pages/resellers/ResellersPage';
import ResellerReportPage from '@/pages/resellers/ResellerReportPage';
import ResellerProfilePage from '@/pages/resellers/ResellerProfilePage';

const sessionState = vi.hoisted(() => ({ role: 'admin' as 'admin' | 'reseller' }));

vi.mock('@/api/queries/useSession', () => ({
  useSession: () => ({
    session: { role: sessionState.role, username: 'ali', name: 'Ali' },
    role: sessionState.role,
    isAdmin: sessionState.role === 'admin',
    isReseller: sessionState.role === 'reseller',
    loading: false,
    error: null,
  }),
}));

vi.mock('@/layouts/AppSidebar', () => ({ default: () => <div data-testid="sidebar" /> }));

/** Live snapshot of reseller #1 as returned by GET /panel/api/resellers/list. */
const statFixture = {
  reseller: {
    id: 1,
    username: 'ali',
    name: 'Ali',
    comment: 'tg:@ali',
    enable: true,
    trafficLimit: 53687091200,
    clientLimit: 10,
    inboundLimit: 2,
    expiryTime: 0,
    pricePerGb: 0.5,
    deposit: 20,
    createdAt: 1789343363011,
    updatedAt: 1789343363011,
  },
  inboundCount: 1,
  clientCount: 2,
  onlineCount: 0,
  usedTraffic: 2147483648,
  allocatedTraffic: 32212254720,
  remainingTraffic: 21474836480,
  cost: 1,
  balance: 19,
  expired: false,
  overQuota: false,
  disabled: false,
};

function ok<T>(obj: T) {
  return { success: true, msg: '', obj } as never;
}

function mockApi(handler: (url: string) => unknown) {
  vi.spyOn(HttpUtil, 'get').mockImplementation(((url: string) => Promise.resolve(ok(handler(url)))) as never);
  vi.spyOn(HttpUtil, 'post').mockImplementation(((url: string) => Promise.resolve(ok(handler(url)))) as never);
}

function renderPage(node: ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>{node}</ThemeProvider>
    </QueryClientProvider>,
  );
}

describe('reseller pages', () => {
  beforeEach(() => {
    sessionState.role = 'admin';
    mockApi((url) => {
      if (url === '/panel/api/resellers/list') return [statFixture];
      if (url === '/panel/api/resellers/assignments') {
        return [{ resellerId: 1, inboundIds: [1], emails: [] }];
      }
      if (url === '/panel/api/inbounds/list/slim') {
        return [{ id: 1, remark: 'res-inb', port: 12001, protocol: 'vless', enable: true }];
      }
      if (url === '/panel/api/clients/list') return [{ email: 'ali-c1', totalGB: 10737418240 }];
      if (url === '/panel/api/reseller/report' || url === '/panel/api/resellers/report/1') {
        return {
          stat: statFixture,
          clients: [
            {
              email: 'ali-c1',
              enable: true,
              totalBytes: 10737418240,
              expiryTime: 0,
              up: 0,
              down: 0,
              used: 0,
              cost: 0,
              inboundIds: [1],
            },
            {
              email: 'ali-c2',
              enable: true,
              totalBytes: 21474836480,
              expiryTime: 0,
              up: 1073741824,
              down: 1073741824,
              used: 2147483648,
              cost: 1,
              inboundIds: [1],
            },
          ],
          transactions: [
            { id: 1, resellerId: 1, type: 'deposit', amount: 5, balance: 15, comment: 'cash', createdAt: 1789343400000 },
          ],
        };
      }
      if (url === '/panel/api/reseller/profile') return statFixture;
      return {};
    });
  });

  it('admin reseller list renders quotas, cost and balance from the API', async () => {
    renderPage(<ResellersPage />);

    await waitFor(() => expect(screen.getByText('Ali')).toBeTruthy());

    const list = document.body.textContent ?? '';
    expect(list).toContain('1 / 2'); // inbounds used / limit
    expect(list).toContain('2 / 10'); // clients used / limit
    expect(list).toContain('30.00 GB / 50.00 GB'); // allocated / limit
    expect(list).toContain('19.00'); // balance = deposit - cost
    expect(HttpUtil.get).toHaveBeenCalledWith('/panel/api/resellers/list');
  });

  it('admin can open the create form and it posts the documented payload', async () => {
    renderPage(<ResellersPage />);
    await waitFor(() => expect(screen.getByText('Ali')).toBeTruthy());

    fireEvent.click(screen.getByRole('button', { name: /New Reseller/i }));

    const dialog = await waitFor(
      () => {
        const el = document.querySelector('.ant-modal');
        if (!el) throw new Error('modal not open');
        return el as HTMLElement;
      },
      { timeout: 5000 },
    );
    expect(within(dialog).getByText('Traffic quota')).toBeTruthy();
    expect(within(dialog).getByText('Client limit')).toBeTruthy();
    expect(within(dialog).getByText('Inbound limit')).toBeTruthy();
    expect(within(dialog).getByText('Price per GB')).toBeTruthy();
  });

  it('reseller report page shows usage, cost, balance and the client rows', async () => {
    sessionState.role = 'reseller';
    renderPage(<ResellerReportPage />);

    await waitFor(() => expect(screen.getByText('ali-c2')).toBeTruthy());
    const text = document.body.textContent ?? '';
    expect(text).toContain('2.00 GB'); // used traffic
    expect(text).toContain('1.00'); // cost
    expect(text).toContain('19.00'); // balance
    expect(text).toContain('deposit'); // ledger row type
    expect(HttpUtil.get).toHaveBeenCalledWith('/panel/api/reseller/report');
  });

  it('reseller profile page exposes quotas and validates the password form', async () => {
    sessionState.role = 'reseller';
    renderPage(<ResellerProfilePage />);

    await waitFor(() => expect(screen.getByText('tg:@ali')).toBeTruthy());
    expect(HttpUtil.get).toHaveBeenCalledWith('/panel/api/reseller/profile');

    const inputs = document.querySelectorAll('input[type="password"]');
    expect(inputs.length).toBe(3);
    fireEvent.change(inputs[0], { target: { value: 'ali12345' } });
    fireEvent.change(inputs[1], { target: { value: 'newpass1' } });
    fireEvent.change(inputs[2], { target: { value: 'newpass2' } });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /Save/i }));
    });

    await waitFor(() => expect(screen.getByText('Passwords do not match')).toBeTruthy());
    expect(HttpUtil.post).not.toHaveBeenCalledWith('/panel/api/reseller/password', expect.anything());
  });

  it('every reseller endpoint the UI calls is documented in the API docs', () => {
    const documented = new Set(
      endpointsSections
        .flatMap((section) => section.endpoints)
        .map((endpoint) => `${endpoint.method} ${endpoint.path}`),
    );

    const used = [
      'GET /panel/api/resellers/list',
      'GET /panel/api/resellers/assignments',
      'GET /panel/api/resellers/report/:id',
      'POST /panel/api/resellers/add',
      'POST /panel/api/resellers/update/:id',
      'POST /panel/api/resellers/del/:id',
      'POST /panel/api/resellers/setEnable/:id',
      'POST /panel/api/resellers/resetPassword/:id',
      'POST /panel/api/resellers/assignInbound',
      'POST /panel/api/resellers/unassignInbound',
      'POST /panel/api/resellers/assignClient',
      'POST /panel/api/resellers/unassignClient',
      'POST /panel/api/resellers/balance',
      'GET /panel/api/reseller/profile',
      'GET /panel/api/reseller/report',
      'POST /panel/api/reseller/password',
      'GET /panel/api/auth/me',
    ];

    for (const endpoint of used) {
      expect(documented.has(endpoint), `${endpoint} is missing from endpoints.ts`).toBe(true);
    }
  });
});
