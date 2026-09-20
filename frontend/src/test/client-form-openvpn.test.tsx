import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactElement } from 'react';

import type { InboundOption } from '@/hooks/useClients';
import ClientFormModal from '@/pages/clients/ClientFormModal';
import ClientBulkAddModal from '@/pages/clients/ClientBulkAddModal';
import BulkAttachInboundsModal from '@/pages/clients/BulkAttachInboundsModal';
import { ThemeProvider } from '@/hooks/useTheme';

import { renderWithProviders } from './test-utils';

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

function renderWithQuery(ui: ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>{ui}</ThemeProvider>
    </QueryClientProvider>,
  );
}

const openvpnInbound: InboundOption = {
  id: 9,
  port: 1194,
  protocol: 'openvpn',
  remark: 'OVPN Server',
  ssMethod: '',
  tag: 'inbound-1194',
  tlsFlowCapable: false,
};

const vlessInbound: InboundOption = {
  id: 1,
  port: 443,
  protocol: 'vless',
  remark: 'VLESS Server',
  ssMethod: '',
  tag: 'inbound-443',
  tlsFlowCapable: true,
};

// Single-client protocol: must stay excluded from the attach lists.
const httpInbound: InboundOption = {
  id: 5,
  port: 8080,
  protocol: 'http',
  remark: 'HTTP Server',
  ssMethod: '',
  tag: 'inbound-8080',
  tlsFlowCapable: false,
};

const inbounds = [openvpnInbound, vlessInbound, httpInbound];

function openSelectByPlaceholder(placeholder: string): string[] {
  const el = screen.getByText(placeholder);
  const select = el.closest('.ant-select') as HTMLElement | null;
  if (!select) throw new Error(`Select not found for placeholder: ${placeholder}`);
  fireEvent.mouseDown((select.querySelector('.ant-select-selector') ?? select) as HTMLElement);
  return Array.from(
    document.querySelectorAll('.ant-select-dropdown:not(.ant-select-dropdown-hidden) .ant-select-item-option'),
  )
    .map((o) => (o.getAttribute('title') ?? o.textContent ?? '').trim())
    .filter(Boolean);
}

function expectOpenVpnAttachable(options: string[]) {
  expect(options).toContain('OVPN Server');
  expect(options).toContain('VLESS Server');
  expect(options).not.toContain('HTTP Server');
}

describe('OpenVPN inbounds in client attach lists', () => {
  it('ClientFormModal offers openvpn inbounds as attachable', () => {
    renderWithProviders(
      <ClientFormModal
        open
        mode="add"
        client={null}
        inbounds={inbounds}
        save={vi.fn().mockResolvedValue({ success: true, msg: '' })}
        onOpenChange={() => {}}
      />,
    );

    expectOpenVpnAttachable(openSelectByPlaceholder('Select one or more inbounds'));
  });

  it('ClientBulkAddModal offers openvpn inbounds as attachable', () => {
    renderWithQuery(
      <ClientBulkAddModal open inbounds={inbounds} onOpenChange={() => {}} />,
    );

    expectOpenVpnAttachable(openSelectByPlaceholder('Select one or more inbounds'));
  });

  it('BulkAttachInboundsModal offers openvpn inbounds as attach targets', () => {
    renderWithProviders(
      <BulkAttachInboundsModal
        open
        count={2}
        inbounds={inbounds}
        onOpenChange={() => {}}
        onSubmit={vi.fn().mockResolvedValue(null)}
      />,
    );

    expectOpenVpnAttachable(openSelectByPlaceholder('Target inbounds'));
  });
});
