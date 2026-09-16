import { beforeEach, describe, expect, it, vi } from 'vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import type { ReactNode } from 'react';

import { HttpUtil } from '@/utils';
import { ThemeProvider } from '@/hooks/useTheme';

import ResellersPage from '@/pages/resellers/ResellersPage';

vi.mock('@/api/queries/useSession', () => ({
  useSession: () => ({
    session: { role: 'admin', username: 'admin', name: 'Admin' },
    role: 'admin',
    isAdmin: true,
    isReseller: false,
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

const slimInbounds = [
  { id: 1, remark: 'res-inb', port: 12001, protocol: 'vless', enable: true },
  { id: 2, remark: 'second-inb', port: 12002, protocol: 'vmess', enable: true },
];

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

/** Finds the table row of a reseller by its display name. */
function rowFor(name: string): HTMLElement {
  const row = screen.getByText(name).closest('tr');
  if (!row) throw new Error(`no table row for ${name}`);
  return row as HTMLElement;
}

/** Finds a row action button by the antd icon it renders (edit, export, ...). */
function rowButton(row: HTMLElement, icon: string): HTMLElement {
  const btn = Array.from(row.querySelectorAll('button')).find((b) => b.querySelector(`.anticon-${icon}`));
  if (!btn) throw new Error(`no row button with icon ${icon}`);
  return btn as HTMLElement;
}

/** The create/edit form modal: waits until its fields are actually mounted. */
async function openFormDialog(): Promise<HTMLElement> {
  return waitFor(
    () => {
      const el = document.querySelector('.ant-modal');
      if (!el || el.querySelectorAll('input').length === 0) throw new Error('form dialog not open');
      return el as HTMLElement;
    },
    { timeout: 5000 },
  );
}

function textInputs(dialog: HTMLElement): HTMLInputElement[] {
  return Array.from(dialog.querySelectorAll('input:not([type="checkbox"])')) as HTMLInputElement[];
}

describe('reseller form (admin)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockApi((url) => {
      if (url === '/panel/api/resellers/list') return [statFixture];
      if (url === '/panel/api/resellers/assignments') {
        return [{ resellerId: 1, inboundIds: [1], emails: [] }];
      }
      if (url === '/panel/api/inbounds/list/slim') return slimInbounds;
      if (url === '/panel/api/resellers/add') return { ...statFixture.reseller, id: 2, username: 'sara' };
      return {};
    });
  });

  it('edit modal is pre-filled with the reseller values', async () => {
    renderPage(<ResellersPage />);
    await waitFor(() => expect(screen.getByText('Ali')).toBeTruthy());

    fireEvent.click(rowButton(rowFor('Ali'), 'edit'));
    const dialog = await openFormDialog();
    expect(within(dialog).getByText('Edit Reseller')).toBeTruthy();

    const inputs = textInputs(dialog);
    await waitFor(() => expect(inputs[0].value).toBe('ali'));
    expect(inputs[2].value).toBe('Ali');
  });

  it('create form starts empty', async () => {
    renderPage(<ResellersPage />);
    await waitFor(() => expect(screen.getByText('Ali')).toBeTruthy());

    fireEvent.click(screen.getByRole('button', { name: /New Reseller/i }));
    const dialog = await openFormDialog();
    const inputs = textInputs(dialog);
    expect(inputs[0].value).toBe('');
    expect(inputs[2].value).toBe('');
  });

  it('create form offers assigned inbounds and wires them on save', async () => {
    renderPage(<ResellersPage />);
    await waitFor(() => expect(screen.getByText('Ali')).toBeTruthy());

    fireEvent.click(screen.getByRole('button', { name: /New Reseller/i }));
    const dialog = await openFormDialog();
    await waitFor(() => expect(within(dialog).getByText('Assigned inbounds')).toBeTruthy());

    const inputs = textInputs(dialog);
    fireEvent.change(inputs[0], { target: { value: 'sara' } });
    fireEvent.change(inputs[1], { target: { value: 'sara12345' } });
    // #1 is owned by Ali: taken in create mode, so its checkbox is disabled.
    expect((within(dialog).getByRole('checkbox', { name: /res-inb/ }) as HTMLInputElement).disabled).toBe(true);
    fireEvent.click(within(dialog).getByRole('checkbox', { name: /second-inb/ }));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));

    await waitFor(() => expect(HttpUtil.post).toHaveBeenCalledWith('/panel/api/resellers/add', expect.anything()));
    await waitFor(() =>
      expect(HttpUtil.post).toHaveBeenCalledWith('/panel/api/resellers/assignInbound', {
        resellerId: 2,
        inboundId: 2,
      }),
    );
    const payload = vi.mocked(HttpUtil.post).mock.calls.find((c) => c[0] === '/panel/api/resellers/add')?.[1] as Record<
      string,
      unknown
    >;
    expect(payload.username).toBe('sara');
  });

  it('edit form assigns/unassigns the inbound diff on save', async () => {
    renderPage(<ResellersPage />);
    await waitFor(() => expect(screen.getByText('Ali')).toBeTruthy());

    fireEvent.click(rowButton(rowFor('Ali'), 'edit'));
    const dialog = await openFormDialog();
    await waitFor(() => expect(within(dialog).getByText('Assigned inbounds')).toBeTruthy());

    // Ali owns #1: uncheck it, check #2 instead.
    const first = within(dialog).getByRole('checkbox', { name: /res-inb/ }) as HTMLInputElement;
    const second = within(dialog).getByRole('checkbox', { name: /second-inb/ }) as HTMLInputElement;
    await waitFor(() => expect(first.checked).toBe(true));
    expect(second.checked).toBe(false);
    fireEvent.click(first);
    fireEvent.click(second);
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));

    await waitFor(() =>
      expect(HttpUtil.post).toHaveBeenCalledWith('/panel/api/resellers/update/1', expect.anything()),
    );
    await waitFor(() =>
      expect(HttpUtil.post).toHaveBeenCalledWith('/panel/api/resellers/unassignInbound', {
        resellerId: 1,
        inboundId: 1,
      }),
    );
    await waitFor(() =>
      expect(HttpUtil.post).toHaveBeenCalledWith('/panel/api/resellers/assignInbound', {
        resellerId: 1,
        inboundId: 2,
      }),
    );
  });
});
