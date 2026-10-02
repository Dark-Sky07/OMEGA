import { useCallback, useEffect, useMemo, useState } from 'react';
import type { ComponentType } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Drawer, Layout, Menu, Tooltip } from 'antd';
import type { MenuProps } from 'antd';
import {
  ApiOutlined,
  CloseOutlined,
  CloudServerOutlined,
  ClusterOutlined,
  CodeOutlined,
  DashboardOutlined,
  DatabaseOutlined,
  GithubOutlined,
  HeartOutlined,
  IdcardOutlined,
  ImportOutlined,
  LogoutOutlined,
  MenuFoldOutlined,
  MenuOutlined,
  MenuUnfoldOutlined,
  MessageOutlined,
  MoonFilled,
  MoonOutlined,
  SafetyOutlined,
  SettingOutlined,
  ShopOutlined,
  SunOutlined,
  SwapOutlined,
  TagsOutlined,
  TeamOutlined,
  ToolOutlined,
  UploadOutlined,
} from '@ant-design/icons';

import { HttpUtil } from '@/utils';
import { pauseAnimationsUntilLeave, useTheme } from '@/hooks/useTheme';
import { useAllSettings } from '@/api/queries/useAllSettings';
import { useSession } from '@/api/queries/useSession';
import './AppSidebar.css';

const SIDEBAR_COLLAPSED_KEY = 'isSidebarCollapsed';
const DONATE_URL = 'https://donate.sanaei.dev/';
const REPO_URL = 'https://github.com/Dark-Sky07/OMEGA';
const LOGOUT_KEY = '__logout__';

type IconName =
  | 'dashboard'
  | 'inbound'
  | 'team'
  | 'groups'
  | 'setting'
  | 'tool'
  | 'cluster'
  | 'logout'
  | 'apidocs'
  | 'outbound'
  | 'reseller'
  | 'profile';

const iconByName: Record<IconName, ComponentType> = {
  dashboard: DashboardOutlined,
  inbound: ImportOutlined,
  team: TeamOutlined,
  groups: TagsOutlined,
  setting: SettingOutlined,
  tool: ToolOutlined,
  cluster: ClusterOutlined,
  logout: LogoutOutlined,
  apidocs: ApiOutlined,
  outbound: UploadOutlined,
  reseller: ShopOutlined,
  profile: IdcardOutlined,
};

type NavTab = { key: string; icon: IconName; title: string; group?: 'overview' | 'manage' | 'system' };

function readCollapsed(): boolean {
  try {
    return JSON.parse(localStorage.getItem(SIDEBAR_COLLAPSED_KEY) || 'false');
  } catch {
    return false;
  }
}

/** Gradient Ω monogram + wordmark. `.brand-text` always reads "OMEGA". */
function Brand({ compact }: { compact?: boolean }) {
  return (
    <div className={`brand-block${compact ? ' is-compact' : ''}`}>
      <span className="brand-mark" aria-hidden="true">
        <span className="brand-mark-glyph">Ω</span>
      </span>
      <span className="brand-copy">
        <span className="brand-text">OMEGA</span>
        <span className="brand-sub">control panel</span>
      </span>
    </div>
  );
}

function DonateButton({ ariaLabel }: { ariaLabel: string }) {
  return (
    <Tooltip title={ariaLabel} placement="top">
      <a
        href={DONATE_URL}
        target="_blank"
        rel="noopener noreferrer"
        className="sidebar-donate omega-icon-btn"
        aria-label={ariaLabel}
      >
        <HeartOutlined />
      </a>
    </Tooltip>
  );
}

function VersionBadge({ version, collapsed }: { version: string; collapsed?: boolean }) {
  if (!version) return null;
  const label = `v${version}`;
  return (
    <a
      href={REPO_URL}
      target="_blank"
      rel="noopener noreferrer"
      className={`sider-version${collapsed ? ' is-collapsed' : ''}`}
      aria-label={`GitHub ${label}`}
      title={label}
    >
      <GithubOutlined />
      {!collapsed && <span className="sider-version-text">{label}</span>}
    </a>
  );
}

function ThemeCycleButton({ id, isDark, isUltra, onCycle, ariaLabel }: {
  id: string;
  isDark: boolean;
  isUltra: boolean;
  onCycle: () => void;
  ariaLabel: string;
}) {
  const icon = !isDark ? <SunOutlined /> : !isUltra ? <MoonOutlined /> : <MoonFilled />;
  return (
    <Tooltip title={ariaLabel} placement="top">
      <button
        id={id}
        type="button"
        className="sidebar-theme-cycle omega-icon-btn"
        aria-label={ariaLabel}
        onClick={onCycle}
      >
        {icon}
      </button>
    </Tooltip>
  );
}

export default function AppSidebar() {
  const { t } = useTranslation();
  const { isDark, isUltra, toggleTheme, toggleUltra } = useTheme();
  const navigate = useNavigate();
  const { pathname, hash } = useLocation();
  const { allSetting } = useAllSettings();
  const { isReseller } = useSession();
  const showSubFormats = !!(allSetting.subJsonEnable || allSetting.subClashEnable);

  const [collapsed, setCollapsed] = useState<boolean>(() => readCollapsed());
  const [drawerOpen, setDrawerOpen] = useState(false);

  const currentTheme: 'light' | 'dark' = isDark ? 'dark' : 'light';
  const panelVersion = window.X_UI_CUR_VER || '';

  const tabs = useMemo<NavTab[]>(() => {
    if (isReseller) {
      // Reseller (نمایندگی) accounts manage only what the admin assigned to them.
      return [
        { key: '/reseller/report', icon: 'dashboard', title: t('menu.report') },
        { key: '/inbounds', icon: 'inbound', title: t('menu.inbounds') },
        { key: '/clients', icon: 'team', title: t('menu.clients') },
        { key: '/reseller/profile', icon: 'profile', title: t('menu.profile') },
        { key: LOGOUT_KEY, icon: 'logout', title: t('logout') },
      ];
    }
    return [
      { key: '/', icon: 'dashboard', title: t('menu.dashboard'), group: 'overview' },
      { key: '/inbounds', icon: 'inbound', title: t('menu.inbounds'), group: 'manage' },
      { key: '/clients', icon: 'team', title: t('menu.clients'), group: 'manage' },
      { key: '/groups', icon: 'groups', title: t('menu.groups'), group: 'manage' },
      { key: '/resellers', icon: 'reseller', title: t('menu.resellers'), group: 'manage' },
      { key: '/nodes', icon: 'cluster', title: t('menu.nodes'), group: 'manage' },
      { key: '/xray#outbound', icon: 'outbound', title: t('pages.xray.Outbounds'), group: 'manage' },
      { key: '/settings', icon: 'setting', title: t('menu.settings'), group: 'system' },
      { key: '/xray', icon: 'tool', title: t('menu.xray'), group: 'system' },
      { key: '/api-docs', icon: 'apidocs', title: t('menu.apiDocs'), group: 'system' },
      { key: LOGOUT_KEY, icon: 'logout', title: t('logout') },
    ];
  }, [t, isReseller]);

  const navItems = useMemo(() => tabs.filter((tab) => tab.icon !== 'logout'), [tabs]);
  const utilItems = useMemo(() => tabs.filter((tab) => tab.icon === 'logout'), [tabs]);

  const settingsChildren = useMemo<NonNullable<MenuProps['items']>>(() => {
    const children: NonNullable<MenuProps['items']> = [
      { key: '/settings#general', icon: <SettingOutlined />, label: t('pages.settings.panelSettings') },
      { key: '/settings#security', icon: <SafetyOutlined />, label: t('pages.settings.securitySettings') },
      { key: '/settings#telegram', icon: <MessageOutlined />, label: t('pages.settings.TGBotSettings') },
      { key: '/settings#subscription', icon: <CloudServerOutlined />, label: t('pages.settings.subSettings') },
    ];
    if (showSubFormats) {
      children.push({ key: '/settings#subscription-formats', icon: <CodeOutlined />, label: 'Sub Formats' });
    }
    return children;
  }, [t, showSubFormats]);

  const xrayChildren = useMemo<NonNullable<MenuProps['items']>>(() => [
    { key: '/xray#basic', icon: <SettingOutlined />, label: t('pages.xray.basicTemplate') },
    { key: '/xray#routing', icon: <SwapOutlined />, label: t('pages.xray.Routings') },
    { key: '/xray#balancer', icon: <ClusterOutlined />, label: t('pages.xray.Balancers') },
    { key: '/xray#dns', icon: <DatabaseOutlined />, label: 'DNS' },
    { key: '/xray#advanced', icon: <CodeOutlined />, label: t('pages.xray.advancedTemplate') },
  ], [t]);

  const settingsActive = pathname === '/settings';
  const xrayActive = pathname === '/xray';
  const selectedKey = settingsActive
    ? `/settings${hash || '#general'}`
    : xrayActive
      ? `/xray${hash || '#basic'}`
      : (pathname === '' ? '/' : pathname);

  // The Outbounds top-level item lives on /xray#outbound, so don't auto-open the
  // Xray Configs submenu for it.
  const openSubmenu = settingsActive ? '/settings' : xrayActive && hash !== '#outbound' ? '/xray' : null;
  const [openKeys, setOpenKeys] = useState<string[]>(() => (openSubmenu ? [openSubmenu] : []));
  useEffect(() => {
    if (openSubmenu) {
      setOpenKeys((keys) => (keys.includes(openSubmenu) ? keys : [...keys, openSubmenu]));
    }
  }, [openSubmenu]);

  const toMenuItem = useCallback((tab: NavTab): NonNullable<MenuProps['items']>[number] => {
    const Icon = iconByName[tab.icon];
    if (tab.key === '/settings') {
      return { key: tab.key, icon: <Icon />, label: tab.title, children: settingsChildren };
    }
    if (tab.key === '/xray') {
      return { key: tab.key, icon: <Icon />, label: tab.title, children: xrayChildren };
    }
    return { key: tab.key, icon: <Icon />, label: tab.title };
  }, [settingsChildren, xrayChildren]);

  const groupTitles = useMemo(() => ({
    overview: t('menu.groupOverview', 'Overview'),
    manage: t('menu.groupManage', 'Manage'),
    system: t('menu.groupSystem', 'System'),
  }), [t]);

  // Admin navigation is sectioned; reseller navigation is a flat list.
  const toMenuItems = useCallback((items: NavTab[]): MenuProps['items'] => {
    const grouped = items.some((tab) => tab.group);
    if (!grouped) return items.map(toMenuItem);
    const order: NonNullable<NavTab['group']>[] = ['overview', 'manage', 'system'];
    const result: NonNullable<MenuProps['items']> = [];
    for (const group of order) {
      const members = items.filter((tab) => tab.group === group);
      if (members.length === 0) continue;
      result.push({
        type: 'group',
        key: `group-${group}`,
        label: groupTitles[group],
        children: members.map(toMenuItem),
      });
    }
    const loose = items.filter((tab) => !tab.group);
    result.push(...loose.map(toMenuItem));
    return result;
  }, [toMenuItem, groupTitles]);

  const openLink = useCallback(async (key: string) => {
    if (key === LOGOUT_KEY) {
      await HttpUtil.post('/logout');
      window.location.href = window.X_UI_BASE_PATH || '/';
      return;
    }
    navigate(key);
  }, [navigate]);

  const onMenuClick = useCallback<NonNullable<MenuProps['onClick']>>(({ key }) => {
    openLink(String(key));
  }, [openLink]);

  const setCollapsedPersist = useCallback((isCollapsed: boolean) => {
    localStorage.setItem(SIDEBAR_COLLAPSED_KEY, String(isCollapsed));
    setCollapsed(isCollapsed);
  }, []);

  const onSiderCollapse = useCallback((isCollapsed: boolean, type: 'clickTrigger' | 'responsive') => {
    if (type === 'clickTrigger') setCollapsedPersist(isCollapsed);
  }, [setCollapsedPersist]);

  const cycleTheme = useCallback((id: string) => {
    pauseAnimationsUntilLeave(id);
    if (!isDark) {
      toggleTheme();
      if (isUltra) toggleUltra();
    } else if (!isUltra) {
      toggleUltra();
    } else {
      toggleUltra();
      toggleTheme();
    }
  }, [isDark, isUltra, toggleTheme, toggleUltra]);

  const collapseLabel = collapsed ? t('menu.expand', 'Expand sidebar') : t('menu.collapse', 'Collapse sidebar');

  return (
    <div className={`ant-sidebar omega-sidebar${collapsed ? ' is-collapsed' : ''}`}>
      <Layout.Sider
        theme={currentTheme}
        width={252}
        collapsedWidth={84}
        collapsible
        collapsed={collapsed}
        breakpoint="md"
        trigger={null}
        onCollapse={onSiderCollapse}
      >
        <div className={`sider-brand${collapsed ? ' sider-brand-collapsed' : ''}`}>
          <Brand compact={collapsed} />
        </div>

        <Menu
          theme={currentTheme}
          mode="inline"
          inlineIndent={18}
          selectedKeys={[selectedKey]}
          openKeys={collapsed ? undefined : openKeys}
          onOpenChange={(keys) => setOpenKeys(keys as string[])}
          className="sider-nav"
          items={toMenuItems(navItems)}
          onClick={onMenuClick}
        />

        <div className="sider-bottom">
          <Menu
            theme={currentTheme}
            mode="inline"
            inlineIndent={18}
            selectedKeys={[selectedKey]}
            className="sider-utility"
            items={toMenuItems(utilItems)}
            onClick={onMenuClick}
          />
          <div className="sider-footer">
            <div className="sider-tools">
              <ThemeCycleButton
                id="theme-cycle"
                isDark={isDark}
                isUltra={isUltra}
                onCycle={() => cycleTheme('theme-cycle')}
                ariaLabel={t('menu.theme')}
              />
              <DonateButton ariaLabel={t('menu.donate') || 'Donate'} />
              <Tooltip title={collapseLabel} placement="top">
                <button
                  type="button"
                  className="sidebar-collapse omega-icon-btn"
                  aria-label={collapseLabel}
                  aria-expanded={!collapsed}
                  onClick={() => setCollapsedPersist(!collapsed)}
                >
                  {collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
                </button>
              </Tooltip>
            </div>
            <VersionBadge version={panelVersion} collapsed={collapsed} />
          </div>
        </div>
      </Layout.Sider>

      <Drawer
        placement="left"
        closable={false}
        open={drawerOpen}
        rootClassName={`${currentTheme} omega-drawer`}
        size="min(84vw, 320px)"
        styles={{
          wrapper: { padding: 0 },
          body: { padding: 0, display: 'flex', flexDirection: 'column', height: '100%' },
          header: { display: 'none' },
        }}
        onClose={() => setDrawerOpen(false)}
      >
        <div className="drawer-header">
          <Brand />
          <div className="drawer-header-actions">
            <ThemeCycleButton
              id="theme-cycle-drawer"
              isDark={isDark}
              isUltra={isUltra}
              onCycle={() => cycleTheme('theme-cycle-drawer')}
              ariaLabel={t('menu.theme')}
            />
            <button
              className="drawer-close omega-icon-btn"
              type="button"
              aria-label={t('close')}
              onClick={() => setDrawerOpen(false)}
            >
              <CloseOutlined />
            </button>
          </div>
        </div>
        <Menu
          theme={currentTheme}
          mode="inline"
          inlineIndent={18}
          selectedKeys={[selectedKey]}
          openKeys={openKeys}
          onOpenChange={(keys) => setOpenKeys(keys as string[])}
          className="drawer-menu drawer-nav"
          items={toMenuItems(navItems)}
          onClick={(info) => { onMenuClick(info); setDrawerOpen(false); }}
        />
        <Menu
          theme={currentTheme}
          mode="inline"
          inlineIndent={18}
          selectedKeys={[selectedKey]}
          className="drawer-menu drawer-utility"
          items={toMenuItems(utilItems)}
          onClick={(info) => { onMenuClick(info); setDrawerOpen(false); }}
        />
        <div className="drawer-footer">
          <DonateButton ariaLabel={t('menu.donate') || 'Donate'} />
          <VersionBadge version={panelVersion} />
        </div>
      </Drawer>

      {!drawerOpen && (
        <button
          className="drawer-handle"
          type="button"
          aria-label={t('menu.dashboard')}
          onClick={() => setDrawerOpen(true)}
        >
          <MenuOutlined />
        </button>
      )}
    </div>
  );
}
