import { beforeEach, describe, expect, it, vi } from 'vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';

import { HttpUtil } from '@/utils';
import { ThemeProvider } from '@/hooks/useTheme';

import ResellersPage from '@/pages/resellers/ResellersPage';

const sessionState = vi.hoisted(() => ({ role: 'admin' as 'admin' | 'reseller' }));

vi.mock('@/api/queries/useSession', () => ({
  useSession: () => ({
    session: { role: sessionState.role, username: 'admin', name: 'Admin' },
    role: sessionState.role,
    isAdmin: sessionState.role === 'admin',
    isReseller: sessionState.role === 'reseller',
    loading: false,
    error: null,
  }),
}));

vi.mock('@/layouts/AppSidebar', () => ({ default: () => <div data-testid="sidebar" /> }));

const GB = 1024 * 1024 * 1024;

/** Healthy reseller: enabled, within quota, one client online. */
const aliStat = {
  reseller: {
    id: 1,
    username: 'ali',
    name: 'Ali',
    comment: 'tg:@ali',
    enable: true,
    trafficLimit: 50 * GB,
    clientLimit: 10,
    expiryTime: 0,
    createdAt: 1735689600000,
    updatedAt: 1735776000000,
  },
  inboundCount: 2,
  clientCount: 3,
  onlineCount: 1,
  usedTraffic: 2 * GB,
  allocatedTraffic: 20 * GB,
  remainingTraffic: 30 * GB,
  expired: false,
  overQuota: false,
  disabled: false,
};

/** Disabled by the admin AND over the traffic quota. */
const saraStat = {
  reseller: {
    id: 2,
    username: 'sara',
    name: 'Sara',
    comment: '',
    enable: false,
    trafficLimit: 80 * GB,
    clientLimit: 0,
    expiryTime: 0,
    createdAt: 1735689600000,
    updatedAt: 1735862400000,
  },
  inboundCount: 1,
  clientCount: 5,
  onlineCount: 0,
  usedTraffic: 5 * GB,
  allocatedTraffic: 100 * GB,
  remainingTraffic: -20 * GB,
  expired: false,
  overQuota: true,
  disabled: true,
};

/** Expired account, no inbounds or clients. */
const rezaStat = {
  reseller: {
    id: 3,
    username: 'reza',
    name: 'Reza',
    comment: '',
    enable: true,
    trafficLimit: 0,
    clientLimit: 0,
    expiryTime: 1735000000000,
    createdAt: 1735603200000,
    updatedAt: 1735603200000,
  },
  inboundCount: 0,
  clientCount: 0,
  onlineCount: 0,
  usedTraffic: 0,
  allocatedTraffic: 0,
  remainingTraffic: 0,
  expired: true,
  overQuota: true,
  disabled: false,
};

function ok<T>(obj: T) {
  return { success: true, msg: '', obj } as never;
}

function mockApi() {
  vi.spyOn(HttpUtil, 'get').mockImplementation(((url: string) => {
    if (url === '/panel/api/resellers/list') return Promise.resolve(ok([aliStat, saraStat, rezaStat]));
    if (url === '/panel/api/resellers/assignments') return Promise.resolve(ok([]));
    return Promise.resolve(ok(null));
  }) as never);
  vi.spyOn(HttpUtil, 'post').mockImplementation((() => Promise.resolve(ok(null))) as never);
}

function renderPage(node: ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, refetchInterval: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>{node}</ThemeProvider>
    </QueryClientProvider>,
  );
}

function statText(title: string): string {
  const titleEl = screen.getAllByText(title).find((el) => el.closest('.ant-statistic'));
  expect(titleEl, `statistic titled ${title} not found`).toBeTruthy();
  const stat = (titleEl as HTMLElement).closest('.ant-statistic') as HTMLElement;
  return (stat.querySelector('.ant-statistic-content') as HTMLElement).textContent || '';
}

describe('resellers overview (admin)', () => {
  beforeEach(() => {
    sessionState.role = 'admin';
    mockApi();
  });

  it('switches from the manage table to the overview and shows aggregate numbers', async () => {
    renderPage(<ResellersPage />);

    // Manage view is the default: the management table actions are visible.
    await waitFor(() => expect(screen.getByText('Ali')).toBeTruthy());

    fireEvent.click(screen.getByText('Overview'));

    await waitFor(() => expect(statText('Traffic used')).toBe('7.00 GB'));
    expect(statText('Traffic allocated')).toBe('120.00 GB');
    expect(statText('Clients')).toBe('8');
    expect(statText('Assigned inbounds')).toBe('3');
    expect(statText('Online now')).toBe('1');
    expect(statText('Needs attention')).toBe('2');

    // Every reseller has a row with its live numbers.
    expect(screen.getByText('Sara')).toBeTruthy();
    expect(screen.getByText('Reza')).toBeTruthy();
    expect(screen.getByText('● 1')).toBeTruthy();
    // Health buckets: Sara is disabled (wins over over-quota), Reza is expired.
    expect(screen.getByText('Disabled')).toBeTruthy();
    expect(screen.getByText('Expired')).toBeTruthy();
    // Unlimited traffic renders as a dash in the remaining column.
    expect(screen.getByText('—')).toBeTruthy();
  });

  it('filters the overview table by search', async () => {
    renderPage(<ResellersPage />);
    await waitFor(() => expect(screen.getByText('Ali')).toBeTruthy());
    fireEvent.click(screen.getByText('Overview'));
    await waitFor(() => expect(screen.getByText('Reza')).toBeTruthy());

    const input = screen.getByPlaceholderText('Search name, username or note');
    fireEvent.change(input, { target: { value: 'sara' } });

    await waitFor(() => expect(screen.queryByText('Ali')).toBeNull());
    expect(screen.getByText('Sara')).toBeTruthy();

    fireEvent.change(input, { target: { value: '' } });
    await waitFor(() => expect(screen.getByText('Ali')).toBeTruthy());
  });

  it('keeps the manage table when switching back', async () => {
    renderPage(<ResellersPage />);
    await waitFor(() => expect(screen.getByText('Ali')).toBeTruthy());

    fireEvent.click(screen.getByText('Overview'));
    await waitFor(() => expect(statText('Traffic used')).toBe('7.00 GB'));
    fireEvent.click(screen.getByText('Manage'));

    await waitFor(() => expect(screen.queryByText('Traffic used')).toBeNull());
    // The management table row for Ali is back, with its manage-only columns
    // (the "Active" enable header exists in both the visible header and the
    // hidden antd measurement clone).
    expect(screen.getByText('Ali')).toBeTruthy();
    expect(screen.getAllByText('Active').length).toBeGreaterThan(0);
  });
});
