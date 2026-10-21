import { beforeEach, describe, expect, it, vi } from 'vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createMemoryRouter, MemoryRouter, RouterProvider } from 'react-router-dom';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';

import { ThemeProvider } from '@/hooks/useTheme';
import { usePageTitle } from '@/hooks/usePageTitle';
import AppSidebar from '@/layouts/AppSidebar';
import { appRoutes } from '@/routes';

const sessionState = vi.hoisted(() => ({
  role: 'admin' as 'admin' | 'reseller',
  brand: null as null | { name: string; logo: string; color: string },
}));

vi.mock('@/api/queries/useSession', () => ({
  useSession: () => ({
    session: {
      role: sessionState.role,
      username: 'ali',
      name: 'Ali',
      stat: sessionState.brand ? { reseller: { brandName: sessionState.brand.name, brandLogo: sessionState.brand.logo, brandColor: sessionState.brand.color } } : undefined,
    },
    role: sessionState.role,
    isAdmin: sessionState.role === 'admin',
    isReseller: sessionState.role === 'reseller',
    loading: false,
    error: null,
  }),
}));

vi.mock('@/api/queries/useAllSettings', () => ({
  useAllSettings: () => ({ allSetting: { subJsonEnable: false, subClashEnable: false } }),
}));

vi.mock('@/layouts/PanelLayout', async () => {
  const { Outlet } = await import('react-router-dom');
  return { default: () => <Outlet /> };
});

// The guards are what this test cares about — keep the routed pages cheap.
vi.mock('@/pages/index/IndexPage', () => ({ default: () => <div>page:index</div> }));
vi.mock('@/pages/inbounds/InboundsPage', () => ({ default: () => <div>page:inbounds</div> }));
vi.mock('@/pages/clients/ClientsPage', () => ({ default: () => <div>page:clients</div> }));
vi.mock('@/pages/groups/GroupsPage', () => ({ default: () => <div>page:groups</div> }));
vi.mock('@/pages/nodes/NodesPage', () => ({ default: () => <div>page:nodes</div> }));
vi.mock('@/pages/settings/SettingsPage', () => ({ default: () => <div>page:settings</div> }));
vi.mock('@/pages/xray/XrayPage', () => ({ default: () => <div>page:xray</div> }));
vi.mock('@/pages/api-docs/ApiDocsPage', () => ({ default: () => <div>page:apidocs</div> }));
vi.mock('@/pages/resellers/ResellersPage', () => ({ default: () => <div>page:resellers</div> }));
vi.mock('@/pages/resellers/ResellerReportPage', () => ({ default: () => <div>page:report</div> }));
vi.mock('@/pages/resellers/ResellerProfilePage', () => ({ default: () => <div>page:profile</div> }));

function renderAt(path: string) {
  const router = createMemoryRouter(appRoutes, { initialEntries: [path] });
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <RouterProvider router={router} />
      </ThemeProvider>
    </QueryClientProvider>,
  );
}

function renderSidebar() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <MemoryRouter initialEntries={['/']}>
          <AppSidebar />
        </MemoryRouter>
      </ThemeProvider>
    </QueryClientProvider>,
  );
}

function PageTitleHarness() {
  usePageTitle();
  return null;
}

function menuLabels(): string[] {
  return Array.from(document.querySelectorAll('.ant-menu-item')).map((el) => (el.textContent ?? '').trim());
}

describe('reseller navigation', () => {
  beforeEach(() => {
    sessionState.role = 'admin';
    sessionState.brand = null;
  });

  it('admin sidebar offers the reseller management entry and PHOENIX branding', () => {
    renderSidebar();
    expect(menuLabels()).toContain('Resellers');
    expect(document.querySelector('.brand-text')?.textContent).toBe('PHOENIX');
  });

  it('reseller sidebar is limited to their own sections', () => {
    sessionState.role = 'reseller';
    renderSidebar();

    const labels = menuLabels();
    expect(labels).toContain('Report');
    expect(labels).toContain('Inbounds');
    expect(labels).toContain('Clients');
    expect(labels).toContain('Profile');
    expect(labels).toContain('Log Out');
    for (const adminOnly of ['Panel Settings', 'Xray Configs', 'API Docs', 'Nodes', 'Groups', 'Overview']) {
      expect(labels).not.toContain(adminOnly);
    }
  });

  it('applies reseller branding and hides the PHOENIX version link', () => {
    sessionState.role = 'reseller';
    sessionState.brand = { name: 'Northstar', logo: '/assets/northstar.svg', color: '#224466' };
    renderSidebar();

    expect(document.querySelector('.brand-text')?.textContent).toBe('Northstar');
    expect(document.querySelector('.brand-mark-img')?.getAttribute('src')).toBe('/assets/northstar.svg');
    expect(document.querySelector('.brand-text')?.getAttribute('style')).toContain('color: rgb(34, 68, 102)');
    expect(document.querySelector('.sider-version')).toBeNull();
  });

  it('lets resellers change the panel language from the desktop sidebar', async () => {
    sessionState.role = 'reseller';
    renderSidebar();

    fireEvent.click(screen.getByRole('button', { name: 'Language' }));
    expect(await screen.findByText('فارسی')).toBeTruthy();
    expect(screen.getByText('Português')).toBeTruthy();
    expect(screen.getByText('العربية')).toBeTruthy();
  });

  it('uses reseller branding in the tab title and favicon, then restores defaults for admins', async () => {
    const favicon = document.createElement('link');
    favicon.rel = 'icon';
    favicon.href = '/phoenix-icon.png';
    document.head.insertBefore(favicon, document.head.firstChild);
    sessionState.role = 'reseller';
    sessionState.brand = { name: 'Northstar', logo: '/assets/northstar.svg', color: '#224466' };

    const view = render(
      <MemoryRouter initialEntries={['/reseller/profile']}>
        <PageTitleHarness />
      </MemoryRouter>,
    );
    await waitFor(() => expect(document.title).toContain('Northstar'));
    expect(favicon.getAttribute('href')).toBe('/assets/northstar.svg');

    sessionState.brand = { name: 'Northstar', logo: 'javascript:alert(1)', color: '#224466' };
    view.rerender(
      <MemoryRouter initialEntries={['/reseller/profile']}>
        <PageTitleHarness />
      </MemoryRouter>,
    );
    await waitFor(() => expect(favicon.href).toMatch(/^data:image\/svg\+xml,/));

    sessionState.role = 'admin';
    view.rerender(
      <MemoryRouter initialEntries={['/reseller/profile']}>
        <PageTitleHarness />
      </MemoryRouter>,
    );
    await waitFor(() => expect(document.title).not.toContain('Northstar'));
    expect(favicon.getAttribute('href')).toBe('/phoenix-icon.png');
    favicon.remove();
  });

  it('reseller landing route redirects to the sales report', async () => {
    sessionState.role = 'reseller';
    renderAt('/');
    await waitFor(() => expect(screen.getByText('page:report')).toBeTruthy());
  });

  it('reseller hitting an admin route is bounced to the sales report', async () => {
    sessionState.role = 'reseller';
    renderAt('/settings');
    await waitFor(() => expect(screen.getByText('page:report')).toBeTruthy());
    expect(screen.queryByText('page:settings')).toBeNull();
  });

  it('resellers keep inbounds and clients, admins keep the panel pages', async () => {
    sessionState.role = 'reseller';
    const resellerInbounds = renderAt('/inbounds');
    await waitFor(() => expect(screen.getAllByText('page:inbounds').length).toBeGreaterThan(0));
    resellerInbounds.unmount();

    sessionState.role = 'admin';
    const adminSettings = renderAt('/settings');
    await waitFor(() => expect(screen.getByText('page:settings')).toBeTruthy());
    adminSettings.unmount();

    const adminResellers = renderAt('/resellers');
    await waitFor(() => expect(screen.getByText('page:resellers')).toBeTruthy());
    adminResellers.unmount();

    // An admin has no reseller report view: the route sends them to /resellers.
    renderAt('/reseller/report');
    await waitFor(() => expect(screen.getByText('page:resellers')).toBeTruthy());
  });
});
