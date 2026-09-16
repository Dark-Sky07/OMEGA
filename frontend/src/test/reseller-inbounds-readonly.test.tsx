import { beforeEach, describe, expect, it, vi } from 'vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import { ThemeProvider } from '@/hooks/useTheme';
import InboundList from '@/pages/inbounds/list/InboundList';
import { buildRowActionsMenu } from '@/pages/inbounds/list/RowActions';
import type { ClientCountEntry, DBInboundRecord } from '@/pages/inbounds/list/types';

const sessionState = vi.hoisted(() => ({ role: 'admin' as 'admin' | 'reseller' }));

vi.mock('@/api/queries/useSession', () => ({
  useSession: () => ({
    session: { role: sessionState.role, username: 'u', name: 'U' },
    role: sessionState.role,
    isAdmin: sessionState.role === 'admin',
    isReseller: sessionState.role === 'reseller',
    loading: false,
    error: null,
  }),
}));

const t = (k: string) => k;

function menuKeys(items: ReturnType<typeof buildRowActionsMenu>): string[] {
  return (items || [])
    .filter((i): i is { key: string } => !!i && typeof i === 'object' && 'key' in i)
    .map((i) => String(i.key));
}

function lastItem(items: ReturnType<typeof buildRowActionsMenu>): unknown {
  const list = items || [];
  return list[list.length - 1];
}

const record = {
  id: 1,
  enable: true,
  remark: 'res-inb',
  subSortIndex: 1,
  port: 12001,
  protocol: 'vless',
  up: 0,
  down: 0,
  total: 0,
  expiryTime: 0,
  _expiryTime: null,
  nodeId: null,
  settings: '{}',
  streamSettings: '{}',
  isVLess: true,
} as DBInboundRecord;

const clientCount: Record<number, ClientCountEntry> = {
  1: { clients: 2, active: ['a@x'], deactive: [], depleted: [], expiring: [], online: [] },
};

function renderList(isMobile = false) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const handlers = {
    onAddInbound: vi.fn(),
    onGeneralAction: vi.fn(),
    onRowAction: vi.fn(),
    onBulkDelete: vi.fn(async () => true),
  };
  const view = render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <InboundList
          dbInbounds={[record]}
          clientCount={clientCount}
          onlineClients={[]}
          lastOnlineMap={{}}
          expireDiff={0}
          trafficDiff={0}
          pageSize={10}
          isMobile={isMobile}
          subEnable={true}
          nodesById={new Map()}
          hasActiveNode={false}
          {...handlers}
        />
      </ThemeProvider>
    </QueryClientProvider>,
  );
  return { view, handlers };
}

describe('reseller read-only inbounds', () => {
  beforeEach(() => {
    sessionState.role = 'admin';
    vi.clearAllMocks();
  });

  it('row menu offers every action to the admin', () => {
    const keys = menuKeys(
      buildRowActionsMenu({ record, subEnable: true, t, isMobile: true, hasClients: true }),
    );
    for (const key of [
      'edit',
      'export',
      'subs',
      'clipboard',
      'resetTraffic',
      'clone',
      'attachExisting',
      'attachClients',
      'detachClients',
      'addToGroup',
      'delAllClients',
      'delete',
    ]) {
      expect(keys, `admin menu contains ${key}`).toContain(key);
    }
  });

  it('row menu hides inbound writes from resellers but keeps client actions', () => {
    const items = buildRowActionsMenu({ record, subEnable: true, t, isMobile: true, hasClients: true, isReseller: true });
    const keys = menuKeys(items);
    for (const key of ['edit', 'resetTraffic', 'clone', 'addToGroup', 'delAllClients', 'delete']) {
      expect(keys, `reseller menu hides ${key}`).not.toContain(key);
    }
    for (const key of ['export', 'subs', 'clipboard', 'attachExisting', 'attachClients', 'detachClients']) {
      expect(keys, `reseller menu keeps ${key}`).toContain(key);
    }
    // No dangling separator where the write actions used to be.
    expect(lastItem(items)).not.toMatchObject({ type: 'divider' });
  });

  it('admin list shows add, selection, switches and the edit button', async () => {
    renderList();
    await waitFor(() => expect(screen.getByText('res-inb')).toBeTruthy());

    expect(screen.getByRole('button', { name: /Add Inbound/ })).toBeTruthy();
    // Row-selection checkbox + enabled enable-switch.
    expect(document.querySelector('.ant-table-selection-column input[type="checkbox"]')).toBeTruthy();
    const enableSwitch = document.querySelector('.ant-table-tbody .ant-switch') as HTMLElement;
    expect(enableSwitch).toBeTruthy();
    expect(enableSwitch.classList.contains('ant-switch-disabled')).toBe(false);
    // Edit button + overflow menu per row.
    expect(document.querySelectorAll('.action-buttons button').length).toBe(2);
  });

  it('reseller list hides add/selection and disables the enable switch', async () => {
    sessionState.role = 'reseller';
    renderList();
    await waitFor(() => expect(screen.getByText('res-inb')).toBeTruthy());

    expect(screen.queryByRole('button', { name: /Add Inbound/ })).toBeNull();
    expect(document.querySelector('.ant-table-selection-column input[type="checkbox"]')).toBeNull();
    const enableSwitch = document.querySelector('.ant-table-tbody .ant-switch') as HTMLElement;
    expect(enableSwitch).toBeTruthy();
    expect(enableSwitch.classList.contains('ant-switch-disabled')).toBe(true);
    // No edit button: only the overflow menu trigger remains.
    expect(document.querySelectorAll('.action-buttons button').length).toBe(1);
  });

  it('reseller general menu keeps exports but drops import and reset-all', async () => {
    sessionState.role = 'reseller';
    renderList();
    await waitFor(() => expect(screen.getByText('res-inb')).toBeTruthy());

    const trigger = Array.from(document.querySelectorAll('.ant-card-head button')).find((b) =>
      b.querySelector('.anticon-menu'),
    ) as HTMLElement;
    fireEvent.click(trigger);
    await waitFor(() => expect(screen.getByText('Export All URLs')).toBeTruthy());
    const menu = document.querySelector('.ant-dropdown-menu') as HTMLElement;
    expect(within(menu).queryByText('Import an Inbound')).toBeNull();
    expect(within(menu).queryByText('Reset Traffic for All Inbounds')).toBeNull();
  });

  it('admin general menu keeps import and reset-all', async () => {
    renderList();
    await waitFor(() => expect(screen.getByText('res-inb')).toBeTruthy());

    const trigger = Array.from(document.querySelectorAll('.ant-card-head button')).find((b) =>
      b.querySelector('.anticon-menu'),
    ) as HTMLElement;
    fireEvent.click(trigger);
    await waitFor(() => expect(screen.getByText('Export All URLs')).toBeTruthy());
    const menu = document.querySelector('.ant-dropdown-menu') as HTMLElement;
    expect(within(menu).getByText('Import an Inbound')).toBeTruthy();
    expect(within(menu).getByText('Reset Traffic for All Inbounds')).toBeTruthy();
  });

  it('reseller mobile cards hide selection and disable the switch', async () => {
    sessionState.role = 'reseller';
    renderList(true);
    await waitFor(() => expect(screen.getByText('res-inb')).toBeTruthy());

    expect(document.querySelector('.card-bulk-bar')).toBeNull();
    expect(document.querySelector('.inbound-card input[type="checkbox"]')).toBeNull();
    const enableSwitch = document.querySelector('.inbound-card .ant-switch') as HTMLElement;
    expect(enableSwitch).toBeTruthy();
    expect(enableSwitch.classList.contains('ant-switch-disabled')).toBe(true);
  });
});
