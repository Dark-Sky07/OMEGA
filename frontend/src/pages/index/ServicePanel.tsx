import { useMemo } from 'react';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Card, Popover, Tooltip } from 'antd';
import {
  ApiOutlined,
  BarsOutlined,
  CloudSyncOutlined,
  LockOutlined,
  PlayCircleOutlined,
  PoweroffOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  ThunderboltOutlined,
  ToolOutlined,
} from '@ant-design/icons';

import type { DaemonInfo, Status } from '@/models/status';
import './ServicePanel.css';

const STATE_KEYS: Record<string, string> = {
  running: 'pages.index.xrayStatusRunning',
  stop: 'pages.index.xrayStatusStop',
  error: 'pages.index.xrayStatusError',
};

type StateKind = 'running' | 'stop' | 'error' | 'unknown';

function stateKind(state: string): StateKind {
  if (state === 'running' || state === 'stop' || state === 'error') return state;
  return 'unknown';
}

interface ServiceAction {
  key: string;
  icon: ReactNode;
  label: string;
  onClick: () => void;
  tone?: 'default' | 'danger' | 'primary';
}

interface ServiceRowProps {
  icon: ReactNode;
  name: string;
  state: string;
  errorMsg?: string;
  meta: ReactNode[];
  actions: ServiceAction[];
  onOpenLogs: () => void;
}

function StatePill({ state, errorMsg, onOpenLogs }: { state: string; errorMsg?: string; onOpenLogs: () => void }) {
  const { t } = useTranslation();
  const kind = stateKind(state);
  const text = t(STATE_KEYS[state] ?? 'pages.index.xrayStatusUnknown');
  const pill = (
    <span className={`omega-pill is-${kind}`}>
      <span className="omega-dot" />
      {text}
    </span>
  );
  if (kind !== 'error') return pill;
  const lines = (errorMsg || '').split('\n').filter((l) => l.trim() !== '');
  return (
    <Popover
      placement="bottomRight"
      title={(
        <div className="service-error-title">
          <span>{t('pages.index.xrayStatusError')}</span>
          <Button size="small" type="text" icon={<BarsOutlined />} onClick={onOpenLogs}>
            {t('pages.index.logs')}
          </Button>
        </div>
      )}
      content={(
        <div className="service-error-body">
          {lines.length === 0 ? <span className="service-error-line">—</span> : lines.map((line, i) => (
            <span key={i} className="service-error-line">{line}</span>
          ))}
        </div>
      )}
    >
      {pill}
    </Popover>
  );
}

function ServiceRow({ icon, name, state, errorMsg, meta, actions, onOpenLogs }: ServiceRowProps) {
  const kind = stateKind(state);
  return (
    <div className={`service-row is-${kind}`}>
      <div className="service-row-main">
        <span className="service-row-icon">{icon}</span>
        <div className="service-row-copy">
          <div className="service-row-name">
            <span>{name}</span>
            <StatePill state={state} errorMsg={errorMsg} onOpenLogs={onOpenLogs} />
          </div>
          <div className="service-row-meta">
            {meta.map((item, i) => (
              <span key={i} className="service-row-meta-item">{item}</span>
            ))}
          </div>
        </div>
      </div>
      <div className="service-row-actions">
        {actions.map((action) => (
          <Tooltip key={action.key} title={action.label} placement="top">
            <Button
              size="small"
              type={action.tone === 'primary' ? 'primary' : 'default'}
              danger={action.tone === 'danger'}
              icon={action.icon}
              aria-label={action.label}
              className="service-action"
              onClick={action.onClick}
            >
              <span className="service-action-label">{action.label}</span>
            </Button>
          </Tooltip>
        ))}
      </div>
    </div>
  );
}

interface ServicePanelProps {
  status: Status;
  accessLogEnable: boolean;
  onStopXray: () => void;
  onRestartXray: () => void;
  onOpenXrayLogs: () => void;
  onOpenAmneziaWGLogs: () => void;
  onOpenVersionSwitch: () => void;
  onOpenLogs: () => void;
  openvpn: DaemonHandlers;
  l2tp: DaemonHandlers;
}

export interface DaemonHandlers {
  onStart: () => void;
  onStop: () => void;
  onRestart: () => void;
  onUpdate: () => void;
}

type Translate = (key: string, fallback: string) => string;

function daemonActions(t: Translate, info: DaemonInfo, handlers: DaemonHandlers): ServiceAction[] {
  const toggle: ServiceAction = info.state === 'running'
    ? { key: 'stop', icon: <PoweroffOutlined />, label: t('pages.index.stopDaemon', 'Stop'), onClick: handlers.onStop, tone: 'danger' }
    : { key: 'start', icon: <PlayCircleOutlined />, label: t('pages.index.startDaemon', 'Start'), onClick: handlers.onStart, tone: 'primary' };
  return [
    toggle,
    { key: 'restart', icon: <ReloadOutlined />, label: t('pages.index.restartDaemon', 'Restart'), onClick: handlers.onRestart },
    { key: 'update', icon: <CloudSyncOutlined />, label: t('pages.index.updateDaemon', 'Update'), onClick: handlers.onUpdate },
  ];
}

/** Xray / OpenVPN / L2TP at a glance, with the same controls the old cards had. */
export default function ServicePanel({
  status,
  accessLogEnable,
  onStopXray,
  onRestartXray,
  onOpenXrayLogs,
  onOpenAmneziaWGLogs,
  onOpenVersionSwitch,
  onOpenLogs,
  openvpn,
  l2tp,
}: ServicePanelProps) {
  const { t } = useTranslation();
  const tr: Translate = (key, fallback) => t(key, fallback);

  const xrayVersion = status.xray.version && status.xray.version !== 'Unknown' ? `v${status.xray.version}` : '';

  const xrayActions = useMemo<ServiceAction[]>(() => {
    const list: ServiceAction[] = [
      { key: 'stop', icon: <PoweroffOutlined />, label: t('pages.index.stopXray'), onClick: onStopXray, tone: 'danger' },
      { key: 'restart', icon: <ReloadOutlined />, label: t('pages.index.restartXray'), onClick: onRestartXray },
      { key: 'switch', icon: <ToolOutlined />, label: xrayVersion || t('pages.index.xraySwitch'), onClick: onOpenVersionSwitch },
      { key: 'awg', icon: <ApiOutlined />, label: t('pages.index.amneziawgLogs'), onClick: onOpenAmneziaWGLogs },
    ];
    // the xray log viewer reads the access log file, so the button only makes
    // sense when one is configured (unlike IP limit, which no longer needs it)
    if (accessLogEnable) {
      list.push({ key: 'logs', icon: <BarsOutlined />, label: t('pages.index.logs'), onClick: onOpenXrayLogs });
    }
    return list;
  }, [t, accessLogEnable, onStopXray, onRestartXray, onOpenVersionSwitch, onOpenAmneziaWGLogs, onOpenXrayLogs, xrayVersion]);

  const daemonMeta = (info: DaemonInfo): ReactNode[] => [
    <>{t('pages.index.daemonInbounds', 'Inbounds')}: <b>{info.inboundCount}</b></>,
    <>{t('pages.index.daemonOnline', 'Online')}: <b>{info.onlineClients}</b></>,
  ];

  return (
    <Card
      className="service-panel"
      title={(
        <span className="service-panel-title">
          <span className="omega-eyebrow">{t('pages.index.services', 'Services')}</span>
        </span>
      )}
      extra={(
        <Tooltip title={t('pages.index.logs')}>
          <Button size="small" type="text" icon={<BarsOutlined />} onClick={onOpenLogs}>
            {t('pages.index.logs')}
          </Button>
        </Tooltip>
      )}
    >
      <ServiceRow
        icon={<ThunderboltOutlined />}
        name="Xray"
        state={status.xray.state}
        errorMsg={status.xray.errorMsg}
        meta={xrayVersion ? [<>{t('pages.index.xraySwitch')}: <b>{xrayVersion}</b></>] : []}
        actions={xrayActions}
        onOpenLogs={onOpenLogs}
      />
      <ServiceRow
        icon={<SafetyCertificateOutlined />}
        name="OpenVPN"
        state={status.openvpn.state}
        errorMsg={status.openvpn.errorMsg}
        meta={daemonMeta(status.openvpn)}
        actions={daemonActions(tr, status.openvpn, openvpn)}
        onOpenLogs={onOpenLogs}
      />
      <ServiceRow
        icon={<LockOutlined />}
        name="L2TP / IPsec"
        state={status.l2tp.state}
        errorMsg={status.l2tp.errorMsg}
        meta={daemonMeta(status.l2tp)}
        actions={daemonActions(tr, status.l2tp, l2tp)}
        onOpenLogs={onOpenLogs}
      />
    </Card>
  );
}
