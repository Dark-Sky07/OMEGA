import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { KeyboardEvent as ReactKeyboardEvent } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Input, Modal, Spin, Tag } from 'antd';
import type { InputRef } from 'antd';
import {
  ApiOutlined,
  CloudServerOutlined,
  ClusterOutlined,
  CodeOutlined,
  DashboardOutlined,
  DatabaseOutlined,
  ImportOutlined,
  LogoutOutlined,
  MessageOutlined,
  SafetyOutlined,
  SearchOutlined,
  SettingOutlined,
  ShopOutlined,
  SunOutlined,
  SwapOutlined,
  TagsOutlined,
  TeamOutlined,
  ToolOutlined,
  UserOutlined,
} from '@ant-design/icons';

import { HttpUtil } from '@/utils';
import { useTheme } from '@/hooks/useTheme';
import { useSession } from '@/api/queries/useSession';
import { useInboundOptions } from '@/api/queries/useInboundOptions';
import { filterPaletteItems, type PaletteItem } from './commandPaletteFilter';
import './CommandPalette.css';

export { filterPaletteItems, type PaletteItem };

export const OPEN_COMMAND_PALETTE_EVENT = 'phoenix:open-command-palette';

export function openCommandPalette() {
  window.dispatchEvent(new CustomEvent(OPEN_COMMAND_PALETTE_EVENT));
}

interface PagedClientHit {
  email: string;
  subId?: string;
  comment?: string;
  enable?: boolean;
  inboundId?: number;
}

interface PagedClientsEnvelope {
  success?: boolean;
  obj?: {
    records?: PagedClientHit[];
  };
}

function jumpToClientsSearch(navigate: ReturnType<typeof useNavigate>, pathname: string, email: string) {
  try {
    const raw = JSON.parse(localStorage.getItem('clientsFilterState') || '{}');
    localStorage.setItem('clientsFilterState', JSON.stringify({ ...raw, searchKey: email }));
  } catch {
    // ignore storage quota errors
  }
  if (pathname === '/clients') {
    window.location.reload();
  } else {
    navigate('/clients');
  }
}

export default function CommandPalette() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { pathname } = useLocation();
  const { toggleTheme, isDark } = useTheme();
  const { isReseller } = useSession();
  const { data: inboundOptions = [] } = useInboundOptions();

  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const [activeIdx, setActiveIdx] = useState(0);
  const [clientHits, setClientHits] = useState<PagedClientHit[]>([]);
  const [searchingClients, setSearchingClients] = useState(false);
  const inputRef = useRef<InputRef>(null);

  const close = useCallback(() => {
    setOpen(false);
  }, []);

  // Global Ctrl+K / Cmd+K listener + custom trigger event from the sidebar button.
  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        setOpen((prev) => !prev);
      }
    }
    function onCustomOpen() {
      setOpen(true);
    }
    window.addEventListener('keydown', onKeyDown);
    window.addEventListener(OPEN_COMMAND_PALETTE_EVENT, onCustomOpen);
    return () => {
      window.removeEventListener('keydown', onKeyDown);
      window.removeEventListener(OPEN_COMMAND_PALETTE_EVENT, onCustomOpen);
    };
  }, []);

  useEffect(() => {
    if (!open) return;
    setQuery('');
    setActiveIdx(0);
    setClientHits([]);
    const timer = window.setTimeout(() => inputRef.current?.focus(), 40);
    return () => window.clearTimeout(timer);
  }, [open]);

  // Debounced live client search when the user types >=2 characters.
  useEffect(() => {
    if (!open) return;
    const q = query.trim();
    if (q.length < 2) {
      setClientHits([]);
      setSearchingClients(false);
      return;
    }
    let cancelled = false;
    setSearchingClients(true);
    const timer = window.setTimeout(async () => {
      try {
        const sp = new URLSearchParams({ page: '1', pageSize: '8', search: q });
        const msg = (await HttpUtil.get(
          `/panel/api/clients/list/paged?${sp.toString()}`,
          undefined,
          { silent: true },
        )) as PagedClientsEnvelope;
        if (!cancelled) {
          setClientHits(Array.isArray(msg?.obj?.records) ? msg.obj!.records! : []);
        }
      } catch {
        if (!cancelled) setClientHits([]);
      } finally {
        if (!cancelled) setSearchingClients(false);
      }
    }, 220);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [open, query]);

  const go = useCallback((dest: string) => {
    close();
    navigate(dest);
  }, [close, navigate]);

  const staticItems = useMemo<PaletteItem[]>(() => {
    if (isReseller) {
      return [
        {
          id: 'nav-report',
          group: 'pages',
          title: t('menu.report'),
          keywords: 'dashboard report overview',
          onSelect: () => go('/reseller/report'),
        },
        {
          id: 'nav-inbounds',
          group: 'pages',
          title: t('menu.inbounds'),
          keywords: 'inbounds ports',
          onSelect: () => go('/inbounds'),
        },
        {
          id: 'nav-clients',
          group: 'pages',
          title: t('menu.clients'),
          keywords: 'clients users',
          onSelect: () => go('/clients'),
        },
        {
          id: 'nav-profile',
          group: 'pages',
          title: t('menu.profile'),
          keywords: 'profile account',
          onSelect: () => go('/reseller/profile'),
        },
      ];
    }

    const pages: PaletteItem[] = [
      {
        id: 'nav-dashboard',
        group: 'pages',
        title: t('menu.dashboard'),
        keywords: 'home status system overview',
        onSelect: () => go('/'),
      },
      {
        id: 'nav-inbounds',
        group: 'pages',
        title: t('menu.inbounds'),
        keywords: 'inbounds vless vmess trojan shadowsocks ports',
        onSelect: () => go('/inbounds'),
      },
      {
        id: 'nav-clients',
        group: 'pages',
        title: t('menu.clients'),
        keywords: 'clients users traffic',
        onSelect: () => go('/clients'),
      },
      {
        id: 'nav-groups',
        group: 'pages',
        title: t('menu.groups'),
        keywords: 'groups tags',
        onSelect: () => go('/groups'),
      },
      {
        id: 'nav-resellers',
        group: 'pages',
        title: t('menu.resellers'),
        keywords: 'resellers agents',
        onSelect: () => go('/resellers'),
      },
      {
        id: 'nav-nodes',
        group: 'pages',
        title: t('menu.nodes'),
        keywords: 'nodes cluster multi-server',
        onSelect: () => go('/nodes'),
      },
      {
        id: 'nav-outbounds',
        group: 'pages',
        title: t('pages.xray.Outbounds'),
        keywords: 'outbounds warp freedom socks',
        onSelect: () => go('/xray#outbound'),
      },
      {
        id: 'nav-settings-general',
        group: 'pages',
        title: `${t('menu.settings')} → ${t('pages.settings.panelSettings')}`,
        keywords: 'settings general port path certificate',
        onSelect: () => go('/settings#general'),
      },
      {
        id: 'nav-settings-security',
        group: 'pages',
        title: `${t('menu.settings')} → ${t('pages.settings.securitySettings')}`,
        keywords: 'settings security password 2fa two-factor api token',
        onSelect: () => go('/settings#security'),
      },
      {
        id: 'nav-settings-telegram',
        group: 'pages',
        title: `${t('menu.settings')} → ${t('pages.settings.TGBotSettings')}`,
        keywords: 'settings telegram bot notify broadcast',
        onSelect: () => go('/settings#telegram'),
      },
      {
        id: 'nav-settings-sub',
        group: 'pages',
        title: `${t('menu.settings')} → ${t('pages.settings.subSettings')}`,
        keywords: 'settings subscription sub clash singbox hwid',
        onSelect: () => go('/settings#subscription'),
      },
      {
        id: 'nav-xray-routing',
        group: 'pages',
        title: `${t('menu.xray')} → ${t('pages.xray.Routings')}`,
        keywords: 'xray routing rules geoip geosite',
        onSelect: () => go('/xray#routing'),
      },
      {
        id: 'nav-xray-dns',
        group: 'pages',
        title: `${t('menu.xray')} → DNS`,
        keywords: 'xray dns doh',
        onSelect: () => go('/xray#dns'),
      },
      {
        id: 'nav-simurgh',
        group: 'pages',
        title: 'Simurgh Tunnel',
        keywords: 'simurgh tunnel relay exit',
        onSelect: () => go('/simurgh'),
      },
      {
        id: 'nav-apidocs',
        group: 'pages',
        title: t('menu.apiDocs'),
        keywords: 'api docs openapi swagger',
        onSelect: () => go('/api-docs'),
      },
    ];

    const inboundItems: PaletteItem[] = inboundOptions.map((ib) => ({
      id: `inbound-${ib.id}`,
      group: 'inbounds',
      title: ib.remark || `Inbound #${ib.id}`,
      subtitle: ib.port ? `:${ib.port}` : undefined,
      badge: ib.protocol?.toUpperCase(),
      keywords: `inbound ${ib.remark || ''} ${ib.protocol || ''} ${ib.port || ''}`,
      onSelect: () => go('/inbounds'),
    }));

    const actions: PaletteItem[] = [
      {
        id: 'act-theme',
        group: 'actions',
        title: t('commandPalette.toggleTheme') || 'Toggle dark / light theme',
        badge: isDark ? 'Dark' : 'Light',
        keywords: 'theme dark light mode appearance',
        onSelect: () => {
          toggleTheme();
          close();
        },
      },
      {
        id: 'act-logout',
        group: 'actions',
        title: t('logout'),
        keywords: 'logout sign out exit',
        onSelect: async () => {
          close();
          await HttpUtil.post('/logout');
          window.location.href = window.X_UI_BASE_PATH || '/';
        },
      },
    ];

    return [...pages, ...inboundItems, ...actions];
  }, [isReseller, t, go, inboundOptions, isDark, toggleTheme, close]);

  const clientItems = useMemo<PaletteItem[]>(() => {
    return clientHits.map((c, idx) => ({
      id: `client-${c.email}-${idx}`,
      group: 'clients',
      title: c.email,
      subtitle: c.comment || c.subId || undefined,
      badge: c.enable === false ? (t('disabled') || 'Off') : (t('enabled') || 'On'),
      keywords: `${c.email} ${c.subId || ''} ${c.comment || ''}`,
      onSelect: () => {
        close();
        jumpToClientsSearch(navigate, pathname, c.email);
      },
    }));
  }, [clientHits, t, close, navigate, pathname]);

  const visibleItems = useMemo<PaletteItem[]>(() => {
    const filteredStatic = filterPaletteItems(staticItems, query);
    // When searching, surface matching clients right at the top.
    if (clientItems.length > 0) {
      return [...clientItems, ...filteredStatic].slice(0, 24);
    }
    return filteredStatic.slice(0, 20);
  }, [staticItems, clientItems, query]);

  useEffect(() => {
    setActiveIdx(0);
  }, [query, clientHits.length]);

  const onInputKeyDown = (e: ReactKeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setActiveIdx((i) => (visibleItems.length ? (i + 1) % visibleItems.length : 0));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setActiveIdx((i) => (visibleItems.length ? (i - 1 + visibleItems.length) % visibleItems.length : 0));
    } else if (e.key === 'Enter') {
      e.preventDefault();
      const target = visibleItems[activeIdx];
      if (target) target.onSelect();
    }
  };

  function iconFor(item: PaletteItem) {
    if (item.group === 'clients') return <UserOutlined />;
    if (item.group === 'inbounds') return <ImportOutlined />;
    if (item.id === 'act-theme') return <SunOutlined />;
    if (item.id === 'act-logout') return <LogoutOutlined />;
    if (item.id.includes('dashboard') || item.id.includes('report')) return <DashboardOutlined />;
    if (item.id.includes('clients')) return <TeamOutlined />;
    if (item.id.includes('groups')) return <TagsOutlined />;
    if (item.id.includes('resellers')) return <ShopOutlined />;
    if (item.id.includes('nodes')) return <ClusterOutlined />;
    if (item.id.includes('outbounds') || item.id.includes('routing')) return <SwapOutlined />;
    if (item.id.includes('security')) return <SafetyOutlined />;
    if (item.id.includes('telegram')) return <MessageOutlined />;
    if (item.id.includes('sub')) return <CloudServerOutlined />;
    if (item.id.includes('dns')) return <DatabaseOutlined />;
    if (item.id.includes('apidocs')) return <ApiOutlined />;
    if (item.id.includes('xray')) return <ToolOutlined />;
    if (item.id.includes('settings')) return <SettingOutlined />;
    return <CodeOutlined />;
  }

  const groupLabel: Record<PaletteItem['group'], string> = {
    clients: t('commandPalette.groupClients') || t('menu.clients'),
    pages: t('commandPalette.groupPages') || 'Pages',
    inbounds: t('commandPalette.groupInbounds') || t('menu.inbounds'),
    actions: t('commandPalette.groupActions') || 'Quick actions',
  };

  return (
    <Modal
      open={open}
      onCancel={close}
      footer={null}
      closable={false}
      width={560}
      className="phoenix-command-palette"
      destroyOnHidden
    >
      <div className="cp-search-bar">
        <SearchOutlined className="cp-search-icon" />
        <Input
          ref={inputRef}
          variant="borderless"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={onInputKeyDown}
          placeholder={
            t('commandPalette.placeholder')
            || 'Jump to page, search client email / subId, or run an action…'
          }
          className="cp-search-input"
        />
        {searchingClients ? <Spin size="small" /> : <kbd className="cp-kbd">ESC</kbd>}
      </div>

      <div className="cp-list" role="listbox">
        {visibleItems.length === 0 ? (
          <div className="cp-empty">
            {t('commandPalette.empty') || 'No matching pages, clients, or actions.'}
          </div>
        ) : (
          visibleItems.map((item, idx) => {
            const prev = idx > 0 ? visibleItems[idx - 1] : null;
            const showGroupHeader = !prev || prev.group !== item.group;
            const active = idx === activeIdx;
            return (
              <div key={item.id}>
                {showGroupHeader && (
                  <div className="cp-group-header">{groupLabel[item.group]}</div>
                )}
                <button
                  type="button"
                  role="option"
                  aria-selected={active}
                  className={`cp-item${active ? ' is-active' : ''}`}
                  onMouseEnter={() => setActiveIdx(idx)}
                  onClick={() => item.onSelect()}
                >
                  <span className="cp-item-icon">{iconFor(item)}</span>
                  <span className="cp-item-main">
                    <span className="cp-item-title">{item.title}</span>
                    {item.subtitle && <span className="cp-item-sub">{item.subtitle}</span>}
                  </span>
                  {item.badge && <Tag className="cp-item-badge">{item.badge}</Tag>}
                </button>
              </div>
            );
          })
        )}
      </div>

      <div className="cp-footer">
        <span><kbd>↑</kbd><kbd>↓</kbd> {t('commandPalette.hintNavigate') || 'navigate'}</span>
        <span><kbd>↵</kbd> {t('commandPalette.hintSelect') || 'select'}</span>
        <span><kbd>Ctrl</kbd>+<kbd>K</kbd></span>
      </div>
    </Modal>
  );
}
