import { useCallback, useEffect, useMemo, useState } from 'react';
import type { ComponentType } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Drawer, Layout, Menu, Popover, Tooltip } from 'antd';
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
  SearchOutlined,
  SettingOutlined,
  ShopOutlined,
  SunOutlined,
  SwapOutlined,
  TagsOutlined,
  TeamOutlined,
  ToolOutlined,
  TranslationOutlined,
  UploadOutlined,
} from '@ant-design/icons';

import { HttpUtil, LanguageManager } from '@/utils';
import { pauseAnimationsUntilLeave, useTheme } from '@/hooks/useTheme';
import { useAllSettings } from '@/api/queries/useAllSettings';
import { useSession } from '@/api/queries/useSession';
import { usePanelUpdateInfo } from '@/api/queries/usePanelUpdateInfo';
import { openCommandPalette } from '@/components/CommandPalette';
import phoenixCrest from '@/assets/phoenix-crest.png';
import './AppSidebar.css';

const SIDEBAR_COLLAPSED_KEY = 'isSidebarCollapsed';
const REPO_URL = 'https://github.com/Dark-Sky07/PHOENIX';
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

/** PHOENIX branding by default; reseller accounts may show their own identity. */
function Brand({ compact, name, logo, color }: { compact?: boolean; name?: string; logo?: string; color?: string }) {
  const branded = Boolean(name || logo);
  return (
    <div className={`brand-block${compact ? ' is-compact' : ''}${branded ? ' is-branded' : ''}`}>
      <span className="brand-mark" aria-hidden="true" style={color ? { boxShadow: `0 0 0 1px ${color}` } : undefined}>
        {logo ? <img src={logo} alt="" className="brand-mark-img" /> : branded ? <span className="brand-mark-glyph">{(name || 'R').slice(0, 1).toUpperCase()}</span> : <img src={phoenixCrest} alt="" className="brand-mark-img" />}
      </span>
      <span className="brand-copy">
        <span className={`brand-text${branded ? ' is-branded' : ''}`} style={color ? { color } : undefined}>{branded ? (name || 'Reseller') : 'PHOENIX'}</span>
        {!branded && <span className="brand-sub">Eternal Flame Core</span>}
      </span>
    </div>
  );
}


function VersionBadge({ version, updateAvailable, collapsed }: { version: string; updateAvailable?: boolean; collapsed?: boolean }) {
  const { t } = useTranslation();
  if (!version) return null;
  const label = `v${version}`;
  const title = updateAvailable ? `${label} — ${t('pages.index.panelUpdateAvailable')}` : label;
  return (
    <a
      href={REPO_URL}
      target="_blank"
      rel="noopener noreferrer"
      className={`sider-version${collapsed ? ' is-collapsed' : ''}${updateAvailable ? ' has-update' : ''}`}
      aria-label={`GitHub ${label}`}
      title={title}
    >
      <GithubOutlined />
      {updateAvailable && <span className="sider-version-dot" aria-hidden="true" />}
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
        className="sidebar-theme-cycle phoenix-icon-btn"
        aria-label={ariaLabel}
        onClick={onCycle}
      >
        {icon}
      </button>
    </Tooltip>
  );
}

function LanguageSwitcher({ isDark }: { isDark: boolean }) {
  const { t } = useTranslation();
  const language = LanguageManager.getLanguage();
  const languageItems = useMemo(
    () => LanguageManager.supportedLanguages.map((item) => ({
      key: item.value,
      label: (
        <span className="sidebar-language-option">
          <span aria-hidden="true">{item.icon}</span>
          <span>{item.name}</span>
        </span>
      ),
    })),
    [],
  );
  const onLanguageSelect = useCallback<NonNullable<MenuProps['onClick']>>(({ key }) => {
    LanguageManager.setLanguage(String(key));
  }, []);
  const label = t('pages.settings.language');

  return (
    <Popover
      rootClassName={isDark ? 'dark' : 'light'}
      placement="topLeft"
      trigger="click"
      styles={{ content: { padding: 4 } }}
      content={
        <Menu
          mode="vertical"
          selectable
          selectedKeys={[language]}
          items={languageItems}
          onClick={onLanguageSelect}
          style={{ border: 'none', minWidth: 176, maxHeight: 320, overflowY: 'auto' }}
        />
      }
    >
      <Tooltip title={label} placement="top">
        <button
          type="button"
          className="sidebar-language phoenix-icon-btn"
          aria-label={label}
          title={label}
        >
          <TranslationOutlined />
        </button>
      </Tooltip>
    </Popover>
  );
}

export default function AppSidebar() {
  const { t } = useTranslation();
  const { isDark, isUltra, toggleTheme, toggleUltra } = useTheme();
  const navigate = useNavigate();
  const { pathname, hash } = useLocation();
  const { allSetting } = useAllSettings();
  const { isReseller, session } = useSession();
  const resellerBrand = session?.stat?.reseller;
  const activeBrand: { brandName?: string; brandLogo?: string; brandColor?: string; brandTheme?: string } = isReseller ? (resellerBrand || {}) : allSetting;
  const themeAccents: Record<string, string> = { ocean: '#1677FF', forest: '#1C9A68', sunset: '#D9485F' };
  const accent = activeBrand?.brandColor || themeAccents[activeBrand?.brandTheme || ''];
  const configuredBrandName = activeBrand?.brandName && activeBrand.brandName !== 'PHOENIX' ? activeBrand.brandName : undefined;
  const brandProps = configuredBrandName || activeBrand?.brandLogo || accent ? {
    name: configuredBrandName || (activeBrand?.brandLogo ? (isReseller ? session?.name : 'PHOENIX') : undefined),
    logo: activeBrand?.brandLogo,
    color: accent,
  } : {};
  const hasCustomBrand = Boolean(brandProps.name || brandProps.logo);
  useEffect(() => {
    window.dispatchEvent(new CustomEvent('phoenix-branding-change', {
      detail: { theme: activeBrand?.brandTheme || 'default', color: activeBrand?.brandColor || '' },
    }));
  }, [activeBrand?.brandTheme, activeBrand?.brandColor]);
  useEffect(() => () => {
    window.dispatchEvent(new CustomEvent('phoenix-branding-change', { detail: { theme: 'default', color: '' } }));
  }, []);
  const showSubFormats = !!(allSetting.subJsonEnable || allSetting.subClashEnable);

  const [collapsed, setCollapsed] = useState<boolean>(() => readCollapsed());
  const [drawerOpen, setDrawerOpen] = useState(false);

  const currentTheme: 'light' | 'dark' = isDark ? 'dark' : 'light';
  const panelVersion = window.X_UI_CUR_VER || '';
  const { info: panelUpdateInfo } = usePanelUpdateInfo();

  const tabs = useMemo<NavTab[]>(() => {
    if (isReseller) {
      // Reseller (نمایندگی) accounts manage only what the admin assigned to them.
      return [
        { key: '/reseller/report', icon: 'dashboard', title: t('menu.report') },
        { key: '/reseller/renewals', icon: 'team', title: t('resellers.renewalCenter') },
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
    <div className={`ant-sidebar phoenix-sidebar${collapsed ? ' is-collapsed' : ''}`}>
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
          <Brand compact={collapsed} {...brandProps} />
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
              <Tooltip title={`${t('commandPalette.open') || 'Quick search'} (Ctrl+K)`} placement="top">
                <button
                  type="button"
                  className="sidebar-search phoenix-icon-btn"
                  aria-label="Command palette (Ctrl+K)"
                  onClick={openCommandPalette}
                >
                  <SearchOutlined />
                </button>
              </Tooltip>
              {isReseller && <LanguageSwitcher isDark={isDark} />}
              <ThemeCycleButton
                id="theme-cycle"
                isDark={isDark}
                isUltra={isUltra}
                onCycle={() => cycleTheme('theme-cycle')}
                ariaLabel={t('menu.theme')}
              />
              <Tooltip title={collapseLabel} placement="top">
                <button
                  type="button"
                  className="sidebar-collapse phoenix-icon-btn"
                  aria-label={collapseLabel}
                  aria-expanded={!collapsed}
                  onClick={() => setCollapsedPersist(!collapsed)}
                >
                  {collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
                </button>
              </Tooltip>
            </div>
            {!hasCustomBrand && <VersionBadge version={panelVersion} updateAvailable={panelUpdateInfo.updateAvailable} collapsed={collapsed} />}
          </div>
        </div>
      </Layout.Sider>

      <Drawer
        placement="left"
        closable={false}
        open={drawerOpen}
        rootClassName={`${currentTheme} phoenix-drawer`}
        size="min(84vw, 320px)"
        styles={{
          wrapper: { padding: 0 },
          body: { padding: 0, display: 'flex', flexDirection: 'column', height: '100%' },
          header: { display: 'none' },
        }}
        onClose={() => setDrawerOpen(false)}
      >
        <div className="drawer-header">
          <Brand {...brandProps} />
          <div className="drawer-header-actions">
            {isReseller && <LanguageSwitcher isDark={isDark} />}
            <ThemeCycleButton
              id="theme-cycle-drawer"
              isDark={isDark}
              isUltra={isUltra}
              onCycle={() => cycleTheme('theme-cycle-drawer')}
              ariaLabel={t('menu.theme')}
            />
            <button
              className="drawer-close phoenix-icon-btn"
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
          {!hasCustomBrand && <VersionBadge version={panelVersion} updateAvailable={panelUpdateInfo.updateAvailable} />}
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
