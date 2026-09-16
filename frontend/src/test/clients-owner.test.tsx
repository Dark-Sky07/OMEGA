import { beforeEach, describe, expect, it, vi } from 'vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';

import { HttpUtil } from '@/utils';
import { ThemeProvider } from '@/hooks/useTheme';

import ClientsPage from '@/pages/clients/ClientsPage';

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

// Explicitly assigned to Ali (via email), although it sits on admin inbound #1.
const directClient = { email: 'direct@example.com', subId: 'abc-1', inboundIds: [1] };
// Not explicitly assigned, but lives on Ali's inbound #2.
const viaInboundClient = { email: 'via-inbound@example.com', subId: 'abc-2', inboundIds: [2] };
// Plain admin client on admin inbound #1.
const adminClient = { email: 'admin@example.com', subId: 'abc-3', inboundIds: [1] };

const pagedResponse = {
  items: [directClient, viaInboundClient, adminClient],
  total: 3,
  filtered: 3,
  page: 1,
  pageSize: 50,
  summary: { total: 3, active: 3, online: [], depleted: [], expiring: [], deactive: [] },
  groups: [],
};

function ok<T>(obj: T) {
  return { success: true, msg: '', obj } as never;
}

function mockApi() {
  vi.spyOn(HttpUtil, 'get').mockImplementation(((url: string) => {
    if (url.startsWith('/panel/api/clients/list/paged')) return Promise.resolve(ok(pagedResponse));
    if (url === '/panel/api/inbounds/options') {
      return Promise.resolve(ok([{ id: 1, remark: 'one' }, { id: 2, remark: 'two' }]));
    }
    if (url === '/panel/api/resellers/assignments') {
      return Promise.resolve(
        ok([{ resellerId: 1, name: 'Ali', username: 'ali', inboundIds: [2], emails: ['direct@example.com'] }]),
      );
    }
    return Promise.resolve(ok(null));
  }) as never);
  vi.spyOn(HttpUtil, 'post').mockImplementation(((url: string) => {
    if (url === '/panel/api/setting/defaultSettings') return Promise.resolve(ok({}));
    if (url === '/panel/api/setting/all') return Promise.resolve(ok({}));
    if (url === '/panel/api/clients/onlines') return Promise.resolve(ok([]));
    return Promise.resolve(ok(null));
  }) as never);
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

function rowFor(email: string): HTMLElement {
  const row = screen.getByText(email).closest('tr');
  expect(row, `row for ${email} not found`).toBeTruthy();
  return row as HTMLElement;
}

describe('clients page owner tags (admin)', () => {
  beforeEach(() => {
    // Reset the HttpUtil spies from the previous test: re-spying the same
    // static method keeps the existing spy and its call history, which would
    // leak admin-session calls into the reseller-session assertions.
    vi.clearAllMocks();
    sessionState.role = 'admin';
    mockApi();
  });

  it('labels reseller-created clients and admin clients in the client column', async () => {
    renderPage(<ClientsPage />);

    await waitFor(() => expect(screen.getByText('direct@example.com')).toBeTruthy());

    // Explicit assignment wins even though the inbound itself is admin-owned.
    const directRow = rowFor('direct@example.com');
    expect(directRow.textContent).toContain('Ali');

    // Ownership via the attached inbound.
    const inboundRow = rowFor('via-inbound@example.com');
    expect(inboundRow.textContent).toContain('Ali');

    // Admin-created clients are labelled as the admin's.
    const adminRow = rowFor('admin@example.com');
    expect(adminRow.textContent).toContain('Admin');

    // Exactly two reseller (blue) tags in the table body.
    const tags = Array.from(document.querySelectorAll('.ant-table-tbody .ant-tag'));
    const aliTags = tags.filter((tag) => tag.textContent === 'Ali');
    const adminTags = tags.filter((tag) => tag.textContent === 'Admin');
    expect(aliTags.length).toBe(2);
    expect(adminTags.length).toBe(1);
    expect(aliTags[0].className).toContain('ant-tag-blue');
  });

  it('does not show owner tags for reseller sessions (their clients are all theirs)', async () => {
    sessionState.role = 'reseller';
    renderPage(<ClientsPage />);

    await waitFor(() => expect(screen.getByText('direct@example.com')).toBeTruthy());

    // No assignments request happens for reseller sessions, so no owner tags.
    const tags = Array.from(document.querySelectorAll('.ant-table-tbody .ant-tag'));
    expect(tags.some((tag) => tag.textContent === 'Ali' || tag.textContent === 'Admin')).toBe(false);
    expect(HttpUtil.get).not.toHaveBeenCalledWith('/panel/api/resellers/assignments', undefined, { silent: true });
  });
});
