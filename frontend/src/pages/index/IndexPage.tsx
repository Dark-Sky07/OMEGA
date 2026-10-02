import { lazy, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Card,
  ConfigProvider,
  Layout,
  message,
  Modal,
  Result,
  Spin,
  Tooltip,
} from 'antd';
import {
  BarsOutlined,
  ControlOutlined,
  CloudServerOutlined,
  CloudDownloadOutlined,
  AreaChartOutlined,
  GlobalOutlined,
  EyeOutlined,
  EyeInvisibleOutlined,
  ThunderboltOutlined,
  DesktopOutlined,
  DatabaseOutlined,
  ForkOutlined,
  CopyOutlined,
  GithubOutlined,
  HddOutlined,
  ReloadOutlined,
  RocketOutlined,
  SwapOutlined,
} from '@ant-design/icons';

import { HttpUtil, SizeFormatter, TimeFormatter, ClipboardManager, FileManager, CPUFormatter } from '@/utils';
import { useTheme } from '@/hooks/useTheme';
import { useStatusQuery } from '@/api/queries/useStatusQuery';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import AppSidebar from '@/layouts/AppSidebar';
import { LazyMount } from '@/components/utility';
import { setMessageInstance } from '@/utils/messageBus';
import ResourceTile from './ResourceTile';
import ServicePanel from './ServicePanel';
import NetworkPanel from './NetworkPanel';
import type { PanelUpdateInfo } from './PanelUpdateModal';
const JsonEditor = lazy(() => import('@/components/form/JsonEditor'));
const PanelUpdateModal = lazy(() => import('./PanelUpdateModal'));
const LogModal = lazy(() => import('./LogModal'));
const BackupModal = lazy(() => import('./BackupModal'));
const SystemHistoryModal = lazy(() => import('./SystemHistoryModal'));
const XrayMetricsModal = lazy(() => import('./XrayMetricsModal'));
const XrayLogModal = lazy(() => import('./XrayLogModal'));
const AmneziaWGLogModal = lazy(() => import('./AmneziaWGLogModal'));
const VersionModal = lazy(() => import('./VersionModal'));
import './IndexPage.css';

const NET_HISTORY_POINTS = 60;

type StateKind = 'running' | 'stop' | 'error' | 'unknown';
function stateKind(state: string): StateKind {
  return state === 'running' || state === 'stop' || state === 'error' ? state : 'unknown';
}

/** Small key/value tile used in the overview's bottom row. */
function InfoTile({
  icon,
  label,
  value,
  hint,
  onClick,
  className = '',
}: {
  icon: React.ReactNode;
  label: React.ReactNode;
  value: React.ReactNode;
  hint?: React.ReactNode;
  onClick?: () => void;
  className?: string;
}) {
  const content = (
    <>
      <span className="info-tile-icon">{icon}</span>
      <span className="info-tile-copy">
        <span className="info-tile-label">{label}</span>
        <span className="info-tile-value">{value}</span>
        {hint && <span className="info-tile-hint">{hint}</span>}
      </span>
    </>
  );
  const cls = `info-tile ${onClick ? 'is-clickable' : ''} ${className}`.trim();
  if (onClick) {
    return (
      <button type="button" className={cls} onClick={onClick}>
        {content}
      </button>
    );
  }
  return <div className={cls}>{content}</div>;
}

export default function IndexPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const { status, fetched, fetchError, refresh } = useStatusQuery();
  const { isMobile } = useMediaQuery();
  const [messageApi, messageContextHolder] = message.useMessage();
  useEffect(() => { setMessageInstance(messageApi); }, [messageApi]);

  const [accessLogEnable, setAccessLogEnable] = useState(false);
  const [panelUpdateInfo, setPanelUpdateInfo] = useState<PanelUpdateInfo>({
    currentVersion: '',
    latestVersion: '',
    updateAvailable: false,
  });

  const basePath = window.X_UI_BASE_PATH || '';

  const [showIp, setShowIp] = useState(false);
  const [logsOpen, setLogsOpen] = useState(false);
  const [backupOpen, setBackupOpen] = useState(false);
  const [panelUpdateOpen, setPanelUpdateOpen] = useState(false);
  const [sysHistoryOpen, setSysHistoryOpen] = useState(false);
  const [xrayMetricsOpen, setXrayMetricsOpen] = useState(false);
  const [xrayLogsOpen, setXrayLogsOpen] = useState(false);
  const [amneziaWGLogsOpen, setAmneziaWGLogsOpen] = useState(false);
  const [versionOpen, setVersionOpen] = useState(false);
  const [configTextOpen, setConfigTextOpen] = useState(false);
  const [configText, setConfigText] = useState('');
  const [loading, setLoading] = useState(false);
  const [loadingTip, setLoadingTip] = useState(t('loading'));

  useEffect(() => {
    HttpUtil.post<{ accessLogEnable?: boolean }>('/panel/api/setting/defaultSettings').then((msg) => {
      if (msg?.success && msg.obj) setAccessLogEnable(!!msg.obj.accessLogEnable);
    });
    HttpUtil.get<PanelUpdateInfo>('/panel/api/server/getPanelUpdateInfo').then((msg) => {
      if (msg?.success && msg.obj) setPanelUpdateInfo(msg.obj);
    });
  }, []);

  const displayVersion = useMemo(
    () => panelUpdateInfo.currentVersion || window.X_UI_CUR_VER || '?',
    [panelUpdateInfo.currentVersion],
  );

  const setBusy = useCallback(
    ({ busy, tip }: { busy: boolean; tip?: string }) => {
      setLoading(busy);
      if (tip) setLoadingTip(tip);
    },
    [],
  );

  const stopXray = useCallback(async () => {
    await HttpUtil.post('/panel/api/server/stopXrayService');
    await refresh();
  }, [refresh]);

  const restartXray = useCallback(async () => {
    await HttpUtil.post('/panel/api/server/restartXrayService');
    await refresh();
  }, [refresh]);

  const stopOpenVPN = useCallback(async () => {
    await HttpUtil.post('/panel/api/server/stopOpenVPNService');
    await refresh();
  }, [refresh]);

  const startOpenVPN = useCallback(async () => {
    await HttpUtil.post('/panel/api/server/startOpenVPNService');
    await refresh();
  }, [refresh]);

  const restartOpenVPN = useCallback(async () => {
    await HttpUtil.post('/panel/api/server/restartOpenVPNService');
    await refresh();
  }, [refresh]);

  const updateOpenVPN = useCallback(async () => {
    await HttpUtil.post('/panel/api/server/updateOpenVPNService');
    await refresh();
  }, [refresh]);

  const stopL2TP = useCallback(async () => {
    await HttpUtil.post('/panel/api/server/stopL2TPService');
    await refresh();
  }, [refresh]);

  const startL2TP = useCallback(async () => {
    await HttpUtil.post('/panel/api/server/startL2TPService');
    await refresh();
  }, [refresh]);

  const restartL2TP = useCallback(async () => {
    await HttpUtil.post('/panel/api/server/restartL2TPService');
    await refresh();
  }, [refresh]);

  const updateL2TP = useCallback(async () => {
    await HttpUtil.post('/panel/api/server/updateL2TPService');
    await refresh();
  }, [refresh]);

  function openPanelVersion() {
    if (panelUpdateInfo.updateAvailable) {
      setPanelUpdateOpen(true);
    } else {
      window.open('https://github.com/Dark-Sky07/OMEGA/releases', '_blank', 'noopener,noreferrer');
    }
  }

  function openGithub() {
    window.open('https://github.com/Dark-Sky07', '_blank', 'noopener,noreferrer');
  }

  async function openConfig() {
    setLoading(true);
    try {
      const msg = await HttpUtil.get('/panel/api/server/getConfigJson');
      if (!msg?.success) return;
      setConfigText(JSON.stringify(msg.obj, null, 2));
      setConfigTextOpen(true);
    } finally {
      setLoading(false);
    }
  }

  async function copyConfig() {
    const ok = await ClipboardManager.copyText(configText || '');
    if (ok) messageApi.success('Copied');
  }

  function downloadConfig() {
    FileManager.downloadTextFile(configText, 'config.json');
  }

  const pageClass = `index-page ${isDark ? 'is-dark' : ''} ${isUltra ? 'is-ultra' : ''}`.trim();

  // Rolling throughput history for the network sparkline (one sample per poll).
  const [netHistory, setNetHistory] = useState<{ up: number[]; down: number[] }>({ up: [], down: [] });
  const lastSampleRef = useRef<{ up: number; down: number } | null>(null);
  useEffect(() => {
    if (!fetched || fetchError) return;
    const sample = { up: status.netIO.up, down: status.netIO.down };
    const prev = lastSampleRef.current;
    if (prev && prev.up === sample.up && prev.down === sample.down) return;
    lastSampleRef.current = sample;
    setNetHistory((h) => ({
      up: [...h.up, sample.up].slice(-NET_HISTORY_POINTS),
      down: [...h.down, sample.down].slice(-NET_HISTORY_POINTS),
    }));
  }, [status.netIO.up, status.netIO.down, fetched, fetchError]);

  const openvpnHandlers = useMemo(() => ({
    onStart: startOpenVPN, onStop: stopOpenVPN, onRestart: restartOpenVPN, onUpdate: updateOpenVPN,
  }), [startOpenVPN, stopOpenVPN, restartOpenVPN, updateOpenVPN]);
  const l2tpHandlers = useMemo(() => ({
    onStart: startL2TP, onStop: stopL2TP, onRestart: restartL2TP, onUpdate: updateL2TP,
  }), [startL2TP, stopL2TP, restartL2TP, updateL2TP]);

  const xrayVersionLabel = status.xray.version && status.xray.version !== 'Unknown' ? `v${status.xray.version}` : '';
  const services: { key: string; name: string; state: string }[] = [
    { key: 'xray', name: 'Xray', state: status.xray.state },
    { key: 'openvpn', name: 'OpenVPN', state: status.openvpn.state },
    { key: 'l2tp', name: 'L2TP', state: status.l2tp.state },
  ];

  const cpuDetails = (
    <>
      <div><b>{t('pages.index.logicalProcessors')}:</b> {status.logicalPro}</div>
      <div><b>{t('pages.index.frequency')}:</b> {CPUFormatter.cpuSpeedFormat(status.cpuSpeedMhz)}</div>
      <div><b>Load:</b> {status.loads.join(' / ')}</div>
    </>
  );

  return (
    <ConfigProvider theme={antdThemeConfig}>
      {messageContextHolder}
      <Layout className={pageClass}>
        <AppSidebar />

        <Layout className="content-shell">
          <Layout.Content className="content-area">
            <Spin
              spinning={loading || !fetched}
              delay={200}
              description={loading ? loadingTip : t('loading')}
              size="large"
            >
              {!fetched ? (
                <div className="loading-spacer" />
              ) : fetchError ? (
                <Result
                  status="error"
                  title={t('somethingWentWrong')}
                  subTitle={fetchError}
                  extra={<Button type="primary" onClick={refresh}>{t('refresh')}</Button>}
                />
              ) : (
                <div className="dash">
                  <header className="dash-hero omega-rise">
                    <div className="dash-hero-copy">
                      <span className="omega-eyebrow">
                        <span className="omega-kicker">OMEGA</span>
                        {displayVersion && <span className="dash-hero-version">v{displayVersion}</span>}
                      </span>
                      <h1 className="omega-page-title">{t('pages.index.title')}</h1>
                      <p className="omega-page-subtitle">{t('pages.index.heroSubtitle', 'Live health of this server and every tunnel it runs.')}</p>
                      <div className="dash-hero-pills">
                        {services.map((svc) => (
                          <span key={svc.key} className={`omega-pill is-${stateKind(svc.state)}`}>
                            <span className="omega-dot" />
                            {svc.name}
                            {svc.key === 'xray' && xrayVersionLabel && <em>{xrayVersionLabel}</em>}
                          </span>
                        ))}
                      </div>
                    </div>
                    <div className="dash-hero-actions">
                      {panelUpdateInfo.updateAvailable && (
                        <Button type="primary" icon={<RocketOutlined />} onClick={openPanelVersion} className="dash-update-btn">
                          {t('update')} {panelUpdateInfo.latestVersion}
                        </Button>
                      )}
                      <Tooltip title={t('pages.index.restartXray')}>
                        <Button icon={<ReloadOutlined />} onClick={restartXray}>{isMobile ? null : t('pages.index.restartXray')}</Button>
                      </Tooltip>
                      <Tooltip title={t('pages.index.logs')}>
                        <Button icon={<BarsOutlined />} onClick={() => setLogsOpen(true)}>{isMobile ? null : t('pages.index.logs')}</Button>
                      </Tooltip>
                      <Tooltip title={t('pages.index.config')}>
                        <Button icon={<ControlOutlined />} onClick={openConfig}>{isMobile ? null : t('pages.index.config')}</Button>
                      </Tooltip>
                      <Tooltip title={t('pages.index.backupTitle')}>
                        <Button icon={<CloudServerOutlined />} onClick={() => setBackupOpen(true)}>{isMobile ? null : t('pages.index.backupTitle')}</Button>
                      </Tooltip>
                    </div>
                  </header>

                  <section className="dash-resources omega-rise omega-rise-1" aria-label={t('pages.index.resources', 'Resources')}>
                    <ResourceTile
                      label={t('pages.index.cpu')}
                      icon={<ThunderboltOutlined />}
                      percent={status.cpu.percent}
                      value={CPUFormatter.cpuCoreFormat(status.cpuCores)}
                      caption={`${t('pages.index.frequency')}: ${CPUFormatter.cpuSpeedFormat(status.cpuSpeedMhz)}`}
                      details={cpuDetails}
                      size={isMobile ? 'compact' : 'default'}
                    />
                    <ResourceTile
                      label={t('pages.index.memory')}
                      icon={<DatabaseOutlined />}
                      percent={status.mem.percent}
                      value={SizeFormatter.sizeFormat(status.mem.current)}
                      caption={`/ ${SizeFormatter.sizeFormat(status.mem.total)}`}
                      size={isMobile ? 'compact' : 'default'}
                    />
                    <ResourceTile
                      label={t('pages.index.swap')}
                      icon={<SwapOutlined />}
                      percent={status.swap.percent}
                      value={SizeFormatter.sizeFormat(status.swap.current)}
                      caption={`/ ${SizeFormatter.sizeFormat(status.swap.total)}`}
                      size={isMobile ? 'compact' : 'default'}
                    />
                    <ResourceTile
                      label={t('pages.index.storage')}
                      icon={<HddOutlined />}
                      percent={status.disk.percent}
                      value={SizeFormatter.sizeFormat(status.disk.current)}
                      caption={`/ ${SizeFormatter.sizeFormat(status.disk.total)}`}
                      size={isMobile ? 'compact' : 'default'}
                    />
                  </section>

                  <section className="dash-main omega-rise omega-rise-2">
                    <ServicePanel
                      status={status}
                      accessLogEnable={accessLogEnable}
                      onStopXray={stopXray}
                      onRestartXray={restartXray}
                      onOpenXrayLogs={() => setXrayLogsOpen(true)}
                      onOpenAmneziaWGLogs={() => setAmneziaWGLogsOpen(true)}
                      onOpenVersionSwitch={() => setVersionOpen(true)}
                      onOpenLogs={() => setLogsOpen(true)}
                      openvpn={openvpnHandlers}
                      l2tp={l2tpHandlers}
                    />
                    <NetworkPanel
                      status={status}
                      upHistory={netHistory.up}
                      downHistory={netHistory.down}
                      onOpenHistory={() => setSysHistoryOpen(true)}
                    />
                  </section>

                  <section className="dash-info omega-rise omega-rise-3">
                    <Card className="info-card" title={<span className="omega-eyebrow">{t('pages.index.operationHours')}</span>}>
                      <div className="info-grid">
                        <InfoTile icon={<ThunderboltOutlined />} label="Xray" value={TimeFormatter.formatSecond(status.appStats.uptime)} />
                        <InfoTile icon={<DesktopOutlined />} label="OS" value={TimeFormatter.formatSecond(status.uptime)} />
                      </div>
                    </Card>

                    <Card className="info-card" title={<span className="omega-eyebrow">{t('pages.index.panelProcess', 'Panel process')}</span>}>
                      <div className="info-grid">
                        <InfoTile icon={<DatabaseOutlined />} label={t('pages.index.memory')} value={SizeFormatter.sizeFormat(status.appStats.mem)} />
                        <InfoTile icon={<ForkOutlined />} label={t('pages.index.threads')} value={status.appStats.threads} />
                      </div>
                    </Card>

                    <Card
                      className="info-card"
                      title={<span className="omega-eyebrow">{t('pages.index.ipAddresses')}</span>}
                      extra={(
                        <Tooltip title={t('pages.index.toggleIpVisibility')} placement={isMobile ? 'topRight' : 'top'}>
                          <button
                            type="button"
                            className="omega-icon-btn ip-toggle-btn"
                            aria-label={t('pages.index.toggleIpVisibility')}
                            aria-pressed={showIp}
                            onClick={() => setShowIp((v) => !v)}
                          >
                            {showIp ? <EyeOutlined className="ip-toggle-icon" /> : <EyeInvisibleOutlined className="ip-toggle-icon" />}
                          </button>
                        </Tooltip>
                      )}
                    >
                      <div className={`info-grid ${showIp ? 'ip-visible' : 'ip-hidden'}`}>
                        <InfoTile icon={<GlobalOutlined />} label="IPv4" value={<span className="ip-value">{status.publicIP.ipv4 || '—'}</span>} />
                        <InfoTile icon={<GlobalOutlined />} label="IPv6" value={<span className="ip-value">{status.publicIP.ipv6 || '—'}</span>} />
                      </div>
                    </Card>

                    <Card className="info-card" title={<span className="omega-eyebrow">{t('pages.index.charts')}</span>}>
                      <div className="info-grid">
                        <InfoTile icon={<AreaChartOutlined />} label={t('pages.index.charts')} value={t('pages.index.systemHistoryTitle')} onClick={() => setSysHistoryOpen(true)} />
                        <InfoTile icon={<AreaChartOutlined />} label="Xray" value={t('pages.index.xrayMetricsTitle')} onClick={() => setXrayMetricsOpen(true)} />
                      </div>
                    </Card>

                    <Card className="info-card" title={<span className="omega-eyebrow">OMEGA</span>}>
                      <div className="info-grid">
                        <InfoTile
                          icon={<CloudDownloadOutlined />}
                          label={panelUpdateInfo.updateAvailable ? t('update') : t('pages.index.upToDate')}
                          value={panelUpdateInfo.updateAvailable ? `v${panelUpdateInfo.latestVersion}` : `v${displayVersion}`}
                          onClick={openPanelVersion}
                          className={panelUpdateInfo.updateAvailable ? 'is-update' : ''}
                        />
                        <InfoTile icon={<GithubOutlined />} label="GitHub" value="Dark-Sky07" onClick={openGithub} />
                      </div>
                    </Card>
                  </section>
                </div>
              )}
            </Spin>
          </Layout.Content>
        </Layout>

        <LazyMount when={panelUpdateOpen}>
          <PanelUpdateModal
            open={panelUpdateOpen}
            info={panelUpdateInfo}
            onClose={() => setPanelUpdateOpen(false)}
            onBusy={setBusy}
          />
        </LazyMount>
        <LazyMount when={logsOpen}>
          <LogModal open={logsOpen} onClose={() => setLogsOpen(false)} />
        </LazyMount>
        <LazyMount when={backupOpen}>
          <BackupModal
            open={backupOpen}
            basePath={basePath}
            onClose={() => setBackupOpen(false)}
            onBusy={setBusy}
          />
        </LazyMount>
        <LazyMount when={sysHistoryOpen}>
          <SystemHistoryModal
            open={sysHistoryOpen}
            status={status}
            onClose={() => setSysHistoryOpen(false)}
          />
        </LazyMount>
        <LazyMount when={xrayMetricsOpen}>
          <XrayMetricsModal open={xrayMetricsOpen} onClose={() => setXrayMetricsOpen(false)} />
        </LazyMount>
        <LazyMount when={xrayLogsOpen}>
          <XrayLogModal open={xrayLogsOpen} onClose={() => setXrayLogsOpen(false)} />
        </LazyMount>
        <LazyMount when={amneziaWGLogsOpen}>
          <AmneziaWGLogModal
            open={amneziaWGLogsOpen}
            onClose={() => setAmneziaWGLogsOpen(false)}
          />
        </LazyMount>
        <LazyMount when={versionOpen}>
          <VersionModal
            open={versionOpen}
            status={status}
            onClose={() => setVersionOpen(false)}
            onBusy={setBusy}
          />
        </LazyMount>

        <LazyMount when={configTextOpen}>
          <Modal
            open={configTextOpen}
            title={t('pages.index.config')}
            width={isMobile ? '100%' : 900}
            style={isMobile
              ? { top: 20, maxWidth: 'calc(100vw - 16px)' }
              : { top: 20 }}
            onCancel={() => setConfigTextOpen(false)}
            footer={[
              <Button
                key="download"
                onClick={downloadConfig}
                size={isMobile ? 'small' : 'middle'}
                icon={<CloudDownloadOutlined />}
              >
                {isMobile ? 'Download' : 'config.json'}
              </Button>,
              <Button
                key="copy"
                type="primary"
                onClick={copyConfig}
                size={isMobile ? 'small' : 'middle'}
                icon={<CopyOutlined />}
              >
                Copy
              </Button>,
            ]}
          >
            <JsonEditor
              value={configText}
              onChange={setConfigText}
              minHeight={isMobile ? '300px' : 'calc(100vh - 220px)'}
              maxHeight={isMobile ? '70vh' : 'calc(100vh - 220px)'}
              readOnly
            />
          </Modal>
        </LazyMount>
      </Layout>
    </ConfigProvider>
  );
}
