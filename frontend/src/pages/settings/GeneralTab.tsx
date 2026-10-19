import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Button,
  Form,
  Input,
  InputNumber,
  Modal,
  Select,
  Space,
  Switch,
  Table,
  Tabs,
  Tag,
  message,
} from 'antd';
import {
  ApartmentOutlined,
  AuditOutlined,
  BellOutlined,
  ClockCircleOutlined,
  CloudUploadOutlined,
  DeleteOutlined,
  GlobalOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  SettingOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';
import type { AllSetting } from '@/models/setting';
import { HttpUtil, LanguageManager } from '@/utils';
import { SettingListItem } from '@/components/ui';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { catTabLabel } from './catTabLabel';
import { sanitizePath } from './uriPath';

interface ApiMsg<T = unknown> {
  success?: boolean;
  msg?: string;
  obj?: T;
}

interface CertStatusView {
  configured: boolean;
  certFile: string;
  keyFile: string;
  subject?: string;
  issuer?: string;
  selfSigned?: boolean;
  notAfter?: number;
  daysLeft?: number;
  expired?: boolean;
  error?: string;
}

interface WatchdogStatusView {
  enabled: boolean;
  activeTag: string;
  candidateTags?: string[];
  lastCheckedAt?: number;
  lastFailoverAt?: number;
  lastFailoverFrom?: string;
  lastFailoverTo?: string;
}

interface AuditLogEntry {
  id: number;
  actorType: string;
  actorId: number;
  actorName: string;
  action: string;
  module: string;
  target: string;
  ip: string;
  status: number;
  createdAt: number;
}

interface CheckHostNodeResult {
  node: string;
  location: string;
  success: boolean;
  status?: 'ok' | 'closed_port' | 'pending' | 'filtered';
  latencyMs: number;
  message?: string;
}

interface CheckHostReport {
  target: string;
  mode: string;
  permanentLink?: string;
  checkedAt: number;
  nodes: CheckHostNodeResult[];
}

interface GeneralTabProps {
  allSetting: AllSetting;
  updateSetting: (patch: Partial<AllSetting>) => void;
  defaultSubTab?: string;
  onlySubTab?: string;
}

const DATEPICKER_LIST: { name: string; value: 'gregorian' | 'jalalian' }[] = [
  { name: 'Gregorian (Standard)', value: 'gregorian' },
  { name: 'Jalalian (شمسی)', value: 'jalalian' },
];

export default function GeneralTab({ allSetting, updateSetting, defaultSubTab = '1', onlySubTab }: GeneralTabProps) {
  const { t } = useTranslation();
  const { isMobile } = useMediaQuery();
  const [activeSubTab, setActiveSubTab] = useState<string>(onlySubTab || defaultSubTab);
  useEffect(() => {
    setActiveSubTab(onlySubTab || defaultSubTab);
  }, [onlySubTab, defaultSubTab]);

  const [lang, setLang] = useState<string>(() => LanguageManager.getLanguage());
  const [inboundOptions, setInboundOptions] = useState<{ label: string; value: string }[]>([]);
  const [outboundTagList, setOutboundTagList] = useState<string[]>([]);
  const [balancerTagList, setBalancerTagList] = useState<string[]>([]);

  const [certStatus, setCertStatus] = useState<CertStatusView | null>(null);
  const [issueOpen, setIssueOpen] = useState(false);
  const [issueDomain, setIssueDomain] = useState('');
  const [issueEmail, setIssueEmail] = useState('');
  const [issueMode, setIssueMode] = useState<'letsencrypt' | 'selfsigned'>('letsencrypt');
  const [issuing, setIssuing] = useState(false);
  const [cloudBackingUp, setCloudBackingUp] = useState(false);
  const [watchdogStatus, setWatchdogStatus] = useState<WatchdogStatusView | null>(null);
  const [watchdogChecking, setWatchdogChecking] = useState(false);
  const [auditLogs, setAuditLogs] = useState<AuditLogEntry[]>([]);
  const [auditLoading, setAuditLoading] = useState(false);
  const [auditActorType, setAuditActorType] = useState('');
  const [auditModule, setAuditModule] = useState('');
  const [auditSearch, setAuditSearch] = useState('');
  const defaultCheckPort = useMemo(() => {
    if (window.location.port) return window.location.port;
    if (window.location.protocol === 'https:') return '443';
    if (allSetting?.webPort && allSetting.webPort > 0) return String(allSetting.webPort);
    return '443';
  }, [allSetting?.webPort]);

  const [checkHostTarget, setCheckHostTarget] = useState(() => {
    const p = window.location.port || (window.location.protocol === 'https:' ? '443' : String(allSetting?.webPort || 443));
    return `${window.location.hostname}:${p}`;
  });
  const [checkHostMode, setCheckHostMode] = useState<'tcp' | 'ping'>('tcp');
  const [checkHostLoading, setCheckHostLoading] = useState(false);
  const [checkHostNodes, setCheckHostNodes] = useState<CheckHostNodeResult[]>([]);
  const [checkHostPermLink, setCheckHostPermLink] = useState('');
  const [inboundPorts, setInboundPorts] = useState<{ label: string; port: number }[]>([]);
  const [messageApi, messageContextHolder] = message.useMessage();

  function extractHostOnly(val: string): string {
    const trimmed = val.trim().replace(/^https?:\/\//i, '').split('/')[0] || window.location.hostname;
    if (trimmed.startsWith('[')) {
      const endBracket = trimmed.indexOf(']');
      if (endBracket > 0) return trimmed.slice(0, endBracket + 1);
    }
    const lastColon = trimmed.lastIndexOf(':');
    if (lastColon > 0 && trimmed.indexOf(':') === lastColon) {
      return trimmed.slice(0, lastColon);
    }
    return trimmed;
  }

  function handleCheckHostModeChange(nextMode: 'tcp' | 'ping') {
    setCheckHostMode(nextMode);
    const hostOnly = extractHostOnly(checkHostTarget);
    if (nextMode === 'ping') {
      setCheckHostTarget(hostOnly);
    } else if (!checkHostTarget.includes(':') || checkHostTarget.startsWith('[')) {
      const fallbackPort = inboundPorts[0]?.port ? String(inboundPorts[0].port) : defaultCheckPort;
      setCheckHostTarget(`${hostOnly}:${fallbackPort}`);
    }
  }

  async function runCheckHostNow(overrideTarget?: string, overrideMode?: 'tcp' | 'ping') {
    const activeMode = overrideMode ?? checkHostMode;
    const rawTarget = (overrideTarget ?? checkHostTarget).trim();
    if (!rawTarget) return;
    setCheckHostLoading(true);
    try {
      const msg = await HttpUtil.post('/panel/api/server/checkHost', {
        target: rawTarget,
        mode: activeMode,
      }) as ApiMsg<CheckHostReport>;
      if (msg?.success && msg.obj?.nodes) {
        setCheckHostNodes(msg.obj.nodes);
        setCheckHostPermLink(msg.obj.permanentLink || '');
        if (msg.obj.target) {
          setCheckHostTarget(msg.obj.target);
        }
      }
    } finally {
      setCheckHostLoading(false);
    }
  }

  const loadWatchdogStatus = useCallback(async () => {
    const msg = await HttpUtil.get('/panel/api/xray/watchdog/status', undefined, { silent: true }) as ApiMsg<WatchdogStatusView>;
    if (msg?.success && msg.obj) {
      setWatchdogStatus(msg.obj);
    }
  }, []);

  const loadAuditLogs = useCallback(async () => {
    setAuditLoading(true);
    try {
      const msg = await HttpUtil.get('/panel/api/setting/auditLogs', {
        limit: 200,
        actorType: auditActorType,
        module: auditModule,
        search: auditSearch,
      }, { silent: true }) as ApiMsg<AuditLogEntry[]>;
      if (msg?.success && Array.isArray(msg.obj)) {
        setAuditLogs(msg.obj);
      }
    } finally {
      setAuditLoading(false);
    }
  }, [auditActorType, auditModule, auditSearch]);

  useEffect(() => {
    loadWatchdogStatus();
    loadAuditLogs();
  }, [loadWatchdogStatus, loadAuditLogs]);

  async function runWatchdogCheckNow() {
    setWatchdogChecking(true);
    try {
      const msg = await HttpUtil.post('/panel/api/xray/watchdog/check', {}) as ApiMsg<WatchdogStatusView>;
      if (msg?.success && msg.obj) {
        setWatchdogStatus(msg.obj);
        messageApi.success(`${t('pages.settings.watchdogCheckDone', 'Watchdog check completed — Active')}: ${msg.obj.activeTag || '-'}`);
      }
    } finally {
      setWatchdogChecking(false);
    }
  }

  async function clearAuditLogsNow() {
    const msg = await HttpUtil.post('/panel/api/setting/auditLogs/clear', {}) as ApiMsg;
    if (msg?.success) {
      setAuditLogs([]);
    }
  }

  async function runCloudBackupNow() {
    setCloudBackingUp(true);
    try {
      await HttpUtil.post('/panel/api/setting/update', allSetting, { silent: true });
      const msg = await HttpUtil.post('/panel/api/setting/cloudBackup/run', {}) as ApiMsg<{ key?: string }>;
      if (msg?.success) {
        messageApi.success(`${t('pages.settings.cloudBackupSuccess', 'Cloud backup uploaded')}: ${msg.obj?.key || ''}`);
      }
    } finally {
      setCloudBackingUp(false);
    }
  }

  const loadCertStatus = useCallback(async () => {
    const msg = await HttpUtil.get('/panel/api/setting/cert/status', undefined, { silent: true }) as ApiMsg<CertStatusView>;
    if (msg?.success && msg.obj) {
      setCertStatus(msg.obj);
    }
  }, []);

  useEffect(() => {
    loadCertStatus();
  }, [loadCertStatus]);

  async function confirmIssueCert() {
    const domain = issueDomain.trim();
    if (!domain) {
      messageApi.error(t('pages.settings.certDomainRequired') || 'Domain is required');
      return;
    }
    setIssuing(true);
    try {
      const msg = await HttpUtil.post('/panel/api/setting/cert/issue', {
        domain,
        email: issueEmail.trim(),
        mode: issueMode,
        applyToPanel: true,
      }) as ApiMsg<CertStatusView>;
      if (msg?.success && msg.obj) {
        setCertStatus(msg.obj);
        updateSetting({ webCertFile: msg.obj.certFile, webKeyFile: msg.obj.keyFile });
        setIssueOpen(false);
        messageApi.success(t('pages.settings.certIssuedSuccess') || 'Certificate issued and applied');
      }
    } finally {
      setIssuing(false);
    }
  }

  useEffect(() => {
    let cancelled = false;
    (async () => {
      // /options is the slim picker-shaped endpoint — it skips the heavy
      // per-client settings and clientStats payloads that /list ships.
      const msg = await HttpUtil.get('/panel/api/inbounds/options') as ApiMsg<{
        tag: string; protocol: string; port: number;
      }[]>;
      if (cancelled) return;
      if (msg?.success && Array.isArray(msg.obj)) {
        setInboundOptions(msg.obj.map((ib) => ({
          label: `${ib.tag} (${ib.protocol}@${ib.port})`,
          value: ib.tag,
        })));
        const seenPorts = new Set<number>();
        const ports: { label: string; port: number }[] = [];
        for (const ib of msg.obj) {
          if (ib.port > 0 && !seenPorts.has(ib.port)) {
            seenPorts.add(ib.port);
            ports.push({ label: `${ib.protocol.toUpperCase()} :${ib.port}`, port: ib.port });
          }
        }
        setInboundPorts(ports.slice(0, 6));
      } else {
        setInboundOptions([]);
        setInboundPorts([]);
      }
    })();
    return () => { cancelled = true; };
  }, []);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      // Candidates for the panel egress picker: template outbounds plus
      // subscription-derived outbounds, and routing balancers. The panel egress
      // is injected as a routing rule, so a balancer tag is a valid target
      // (it load-balances the panel's own traffic). The geodata picker, by
      // contrast, dials a forced tag and can only use a concrete outbound.
      const msg = await HttpUtil.post('/panel/api/xray/', undefined, { silent: true }) as ApiMsg<string>;
      if (cancelled || !msg?.success || typeof msg.obj !== 'string') return;
      try {
        const payload = JSON.parse(msg.obj) as Record<string, unknown>;
        const template = (payload.xraySetting || {}) as Record<string, unknown>;
        const tags = new Set<string>();
        const outbounds = Array.isArray(template.outbounds) ? template.outbounds : [];
        for (const o of outbounds) {
          if (!o || typeof o !== 'object') continue;
          const rec = o as Record<string, unknown>;
          if (rec.protocol === 'blackhole') continue; // dropping traffic is never a useful egress
          const tag = rec.tag;
          if (typeof tag === 'string' && tag) tags.add(tag);
        }
        const subTags = Array.isArray(payload.subscriptionOutboundTags) ? payload.subscriptionOutboundTags : [];
        for (const tag of subTags) {
          if (typeof tag === 'string' && tag) tags.add(tag);
        }
        const balancerTags: string[] = [];
        const routing = (template.routing || {}) as Record<string, unknown>;
        const balancers = Array.isArray(routing.balancers) ? routing.balancers : [];
        for (const b of balancers) {
          if (!b || typeof b !== 'object') continue;
          const tag = (b as Record<string, unknown>).tag;
          if (typeof tag === 'string' && tag && !tags.has(tag)) balancerTags.push(tag);
        }
        setOutboundTagList([...tags]);
        setBalancerTagList(balancerTags);
      } catch {
        setOutboundTagList([]);
        setBalancerTagList([]);
      }
    })();
    return () => { cancelled = true; };
  }, []);

  // Outbound tags and balancer tags share one picker. When balancers exist they
  // get their own labeled group so it's clear the selection routes through a
  // balancer rather than a single outbound.
  const outboundOptions = useMemo<
    ({ label: string; value: string } | { label: string; options: { label: string; value: string }[] })[]
  >(() => {
    const outOpts = outboundTagList.map((tag) => ({ label: tag, value: tag }));
    if (balancerTagList.length === 0) return outOpts;
    return [
      { label: t('pages.xray.Outbounds'), options: outOpts },
      { label: t('pages.xray.Balancers'), options: balancerTagList.map((tag) => ({ label: tag, value: tag })) },
    ];
  }, [outboundTagList, balancerTagList, t]);

  const ldapInboundTagList = useMemo(() => {
    const csv = allSetting.ldapInboundTags || '';
    return csv.length ? csv.split(',').map((s) => s.trim()).filter(Boolean) : [];
  }, [allSetting.ldapInboundTags]);

  function setLdapInboundTagList(list: string[]) {
    updateSetting({ ldapInboundTags: Array.isArray(list) ? list.join(',') : '' });
  }

  function onLangChange(value: string) {
    setLang(value);
    LanguageManager.setLanguage(value);
  }

  const langOptions = useMemo(
    () => LanguageManager.supportedLanguages.map((l: { value: string; name: string; icon: string }) => ({
      value: l.value,
      label: (
        <>
          <span role="img" aria-label={l.name}>{l.icon}</span>
          &nbsp;&nbsp;<span>{l.name}</span>
        </>
      ),
    })),
    [],
  );

  const activeCloudProvider = (allSetting.cloudBackupProvider || 's3').toLowerCase();

  const tabItems = [
      {
        key: '1',
        label: catTabLabel(<SettingOutlined />, t('pages.settings.panelSettings'), isMobile),
        children: (
          <>
            <SettingListItem paddings="small" title={t('pages.settings.panelListeningIP')} description={t('pages.settings.panelListeningIPDesc')}>
              <Input value={allSetting.webListen} onChange={(e) => updateSetting({ webListen: e.target.value })} />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.panelListeningDomain')} description={t('pages.settings.panelListeningDomainDesc')}>
              <Input value={allSetting.webDomain} onChange={(e) => updateSetting({ webDomain: e.target.value })} />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.panelPort')} description={t('pages.settings.panelPortDesc')}>
              <InputNumber value={allSetting.webPort} min={1} max={65535} style={{ width: '100%' }}
                onChange={(v) => updateSetting({ webPort: Number(v) || 0 })} />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.panelUrlPath')} description={t('pages.settings.panelUrlPathDesc')}>
              <Input value={allSetting.webBasePath} onChange={(e) => updateSetting({ webBasePath: sanitizePath(e.target.value) })} />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.sessionMaxAge')} description={t('pages.settings.sessionMaxAgeDesc')}>
              <InputNumber value={allSetting.sessionMaxAge} min={60} max={525600} style={{ width: '100%' }}
                onChange={(v) => updateSetting({ sessionMaxAge: Number(v) || 0 })} />
            </SettingListItem>

            <SettingListItem
              paddings="small"
              title={t('pages.settings.trustedProxyCidrs')}
              description={t('pages.settings.trustedProxyCidrsDesc')}
            >
              <Input
                value={allSetting.trustedProxyCIDRs}
                placeholder="127.0.0.1/32,::1/128"
                onChange={(e) => updateSetting({ trustedProxyCIDRs: e.target.value })}
              />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.panelOutbound')} description={t('pages.settings.panelOutboundDesc')}>
              <Select
                style={{ width: '100%' }}
                allowClear
                showSearch
                value={allSetting.panelOutbound || undefined}
                placeholder={t('pages.settings.panelOutboundPh')}
                options={outboundOptions}
                onChange={(v) => updateSetting({ panelOutbound: (v as string | undefined) || '' })}
              />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.pageSize')} description={t('pages.settings.pageSizeDesc')}>
              <InputNumber value={allSetting.pageSize} min={0} max={1000} step={5} style={{ width: '100%' }}
                onChange={(v) => updateSetting({ pageSize: Number(v) || 0 })} />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.restartXrayOnClientDisable')} description={t('pages.settings.restartXrayOnClientDisableDesc')}>
              <Switch checked={allSetting.restartXrayOnClientDisable}
                onChange={(v) => updateSetting({ restartXrayOnClientDisable: v })} />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.language')}>
              <Select
                value={lang}
                onChange={onLangChange}
                style={{ width: '100%' }}
                options={langOptions}
              />
            </SettingListItem>
          </>
        ),
      },
      {
        key: '2',
        label: catTabLabel(<BellOutlined />, t('pages.settings.notifications'), isMobile),
        children: (
          <>
            <SettingListItem paddings="small" title={t('pages.settings.expireTimeDiff')} description={t('pages.settings.expireTimeDiffDesc')}>
              <InputNumber value={allSetting.expireDiff} min={0} style={{ width: '100%' }}
                onChange={(v) => updateSetting({ expireDiff: Number(v) || 0 })} />
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.trafficDiff')} description={t('pages.settings.trafficDiffDesc')}>
              <InputNumber value={allSetting.trafficDiff} min={0} max={100} style={{ width: '100%' }}
                onChange={(v) => updateSetting({ trafficDiff: Number(v) || 0 })} />
            </SettingListItem>
          </>
        ),
      },
      {
        key: '3',
        label: catTabLabel(<SafetyCertificateOutlined />, t('pages.settings.certs'), isMobile),
        children: (
          <>
            <SettingListItem
              paddings="small"
              title={t('pages.settings.oneClickSslTitle') || 'One-Click SSL Certificate'}
              description={
                t('pages.settings.oneClickSslDesc')
                || "Issue a free Let's Encrypt certificate (auto-renewed every 60 days) or a 1-year self-signed certificate."
              }
            >
              <Space wrap>
                {certStatus?.configured && !certStatus.error ? (
                  <Tag color={certStatus.expired ? 'red' : certStatus.selfSigned ? 'gold' : 'green'}>
                    {certStatus.subject || 'TLS'} · {certStatus.selfSigned
                      ? (t('pages.settings.certSelfSigned') || 'Self-Signed')
                      : (certStatus.issuer || "Let's Encrypt")} · {certStatus.daysLeft ?? 0}d
                  </Tag>
                ) : null}
                <Button type="primary" onClick={() => setIssueOpen(true)}>
                  {t('pages.settings.issueCertBtn') || 'Issue SSL Certificate'}
                </Button>
              </Space>
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.publicKeyPath')} description={t('pages.settings.publicKeyPathDesc')}>
              <Input value={allSetting.webCertFile} onChange={(e) => updateSetting({ webCertFile: e.target.value })} />
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.privateKeyPath')} description={t('pages.settings.privateKeyPathDesc')}>
              <Input value={allSetting.webKeyFile} onChange={(e) => updateSetting({ webKeyFile: e.target.value })} />
            </SettingListItem>
          </>
        ),
      },
      {
        key: '4',
        label: catTabLabel(<GlobalOutlined />, t('pages.settings.externalTraffic'), isMobile),
        children: (
          <>
            <SettingListItem paddings="small" title={t('pages.settings.externalTrafficInformEnable')} description={t('pages.settings.externalTrafficInformEnableDesc')}>
              <Switch checked={allSetting.externalTrafficInformEnable}
                onChange={(v) => updateSetting({ externalTrafficInformEnable: v })} />
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.externalTrafficInformURI')} description={t('pages.settings.externalTrafficInformURIDesc')}>
              <Input
                value={allSetting.externalTrafficInformURI}
                placeholder="(http|https)://domain[:port]/path/"
                onChange={(e) => updateSetting({ externalTrafficInformURI: e.target.value })}
              />
            </SettingListItem>
          </>
        ),
      },
      {
        key: '5',
        label: catTabLabel(<ClockCircleOutlined />, t('pages.settings.dateAndTime'), isMobile),
        children: (
          <>
            <SettingListItem paddings="small" title={t('pages.settings.timeZone')} description={t('pages.settings.timeZoneDesc')}>
              <Input value={allSetting.timeLocation} onChange={(e) => updateSetting({ timeLocation: e.target.value })} />
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.datepicker')} description={t('pages.settings.datepickerDescription')}>
              <Select
                value={allSetting.datepicker || 'gregorian'}
                onChange={(v) => updateSetting({ datepicker: v as 'gregorian' | 'jalalian' })}
                style={{ width: '100%' }}
                options={DATEPICKER_LIST.map((d) => ({ value: d.value, label: d.name }))}
              />
            </SettingListItem>
          </>
        ),
      },
      {
        key: '6',
        label: catTabLabel(<ApartmentOutlined />, 'LDAP', isMobile),
        children: (
          <div key="ldap-settings-panel" data-testid="ldap-settings-panel">
            <SettingListItem paddings="small" title={t('pages.settings.ldap.enable')}>
              <Switch checked={allSetting.ldapEnable} onChange={(v) => updateSetting({ ldapEnable: v })} />
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.ldap.host')}>
              <Input value={allSetting.ldapHost} onChange={(e) => updateSetting({ ldapHost: e.target.value })} />
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.ldap.port')}>
              <InputNumber value={allSetting.ldapPort} min={1} max={65535} style={{ width: '100%' }}
                onChange={(v) => updateSetting({ ldapPort: Number(v) || 0 })} />
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.ldap.useTls')}>
              <Switch checked={allSetting.ldapUseTLS} onChange={(v) => updateSetting({ ldapUseTLS: v })} />
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.ldap.bindDn')}>
              <Input value={allSetting.ldapBindDN} onChange={(e) => updateSetting({ ldapBindDN: e.target.value })} />
            </SettingListItem>
            <SettingListItem
              paddings="small"
              title={t('password')}
              description={allSetting.hasLdapPassword ? t('pages.settings.ldap.passwordConfigured') : t('pages.settings.ldap.passwordUnconfigured')}
            >
              <Input.Password
                value={allSetting.ldapPassword}
                placeholder={allSetting.hasLdapPassword ? t('pages.settings.ldap.passwordPlaceholder') : ''}
                onChange={(e) => updateSetting({ ldapPassword: e.target.value })}
              />
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.ldap.baseDn')}>
              <Input value={allSetting.ldapBaseDN} onChange={(e) => updateSetting({ ldapBaseDN: e.target.value })} />
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.ldap.userFilter')}>
              <Input value={allSetting.ldapUserFilter} onChange={(e) => updateSetting({ ldapUserFilter: e.target.value })} />
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.ldap.userAttr')}>
              <Input value={allSetting.ldapUserAttr} onChange={(e) => updateSetting({ ldapUserAttr: e.target.value })} />
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.ldap.vlessField')}>
              <Input value={allSetting.ldapVlessField} onChange={(e) => updateSetting({ ldapVlessField: e.target.value })} />
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.ldap.flagField')} description={t('pages.settings.ldap.flagFieldDesc')}>
              <Input value={allSetting.ldapFlagField} onChange={(e) => updateSetting({ ldapFlagField: e.target.value })} />
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.ldap.truthyValues')} description={t('pages.settings.ldap.truthyValuesDesc')}>
              <Input value={allSetting.ldapTruthyValues} onChange={(e) => updateSetting({ ldapTruthyValues: e.target.value })} />
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.ldap.invertFlag')} description={t('pages.settings.ldap.invertFlagDesc')}>
              <Switch checked={allSetting.ldapInvertFlag} onChange={(v) => updateSetting({ ldapInvertFlag: v })} />
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.ldap.syncSchedule')} description={t('pages.settings.ldap.syncScheduleDesc')}>
              <Input value={allSetting.ldapSyncCron} onChange={(e) => updateSetting({ ldapSyncCron: e.target.value })} />
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.ldap.inboundTags')} description={t('pages.settings.ldap.inboundTagsDesc')}>
              <>
                <Select
                  mode="multiple"
                  value={ldapInboundTagList}
                  onChange={setLdapInboundTagList}
                  style={{ width: '100%' }}
                  options={inboundOptions}
                />
                {inboundOptions.length === 0 && (
                  <div className="ldap-no-inbounds">{t('pages.settings.ldap.noInbounds')}</div>
                )}
              </>
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.ldap.autoCreate')}>
              <Switch checked={allSetting.ldapAutoCreate} onChange={(v) => updateSetting({ ldapAutoCreate: v })} />
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.ldap.autoDelete')}>
              <Switch checked={allSetting.ldapAutoDelete} onChange={(v) => updateSetting({ ldapAutoDelete: v })} />
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.ldap.defaultTotalGb')}>
              <InputNumber value={allSetting.ldapDefaultTotalGB} min={0} style={{ width: '100%' }}
                onChange={(v) => updateSetting({ ldapDefaultTotalGB: Number(v) || 0 })} />
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.ldap.defaultExpiryDays')}>
              <InputNumber value={allSetting.ldapDefaultExpiryDays} min={0} style={{ width: '100%' }}
                onChange={(v) => updateSetting({ ldapDefaultExpiryDays: Number(v) || 0 })} />
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.ldap.defaultIpLimit')}>
              <InputNumber value={allSetting.ldapDefaultLimitIP} min={0} style={{ width: '100%' }}
                onChange={(v) => updateSetting({ ldapDefaultLimitIP: Number(v) || 0 })} />
            </SettingListItem>
          </div>
        ),
      },
      {
        key: '7',
        label: catTabLabel(<CloudUploadOutlined />, t('pages.settings.cloudBackupTitle', 'Cloud Backup'), isMobile),
        children: (
          <div key="cloud-backup-settings-panel" data-testid="cloud-backup-settings-panel">
            <SettingListItem
              paddings="small"
              title={t('pages.settings.cloudBackupEnable', 'Scheduled Cloud Backup')}
              description={t('pages.settings.cloudBackupEnableDesc', 'Automatically upload database snapshots to S3 / Cloudflare R2 / MinIO, WebDAV, or Google Drive and prune older backups.')}
            >
              <Space>
                <Switch
                  checked={allSetting.cloudBackupEnable}
                  onChange={(v) => updateSetting({ cloudBackupEnable: v })}
                />
                <Button
                  icon={<CloudUploadOutlined />}
                  loading={cloudBackingUp}
                  onClick={runCloudBackupNow}
                >
                  {t('pages.settings.cloudBackupRunNow', 'Backup Now')}
                </Button>
              </Space>
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.cloudBackupProvider', 'Storage Provider')}>
              <Select
                value={activeCloudProvider}
                style={{ width: '100%' }}
                onChange={(v) => updateSetting({ cloudBackupProvider: v })}
                options={[
                  { value: 's3', label: 'S3 / Cloudflare R2 / MinIO (AWS Signature v4)' },
                  { value: 'webdav', label: 'WebDAV (Nextcloud / Hetzner Storage Box / Koofr)' },
                  { value: 'gdrive', label: t('pages.settings.cloudBackupProviderGdrive', 'Google Drive (Service Account / OAuth2)') },
                ]}
              />
            </SettingListItem>
            {activeCloudProvider === 'gdrive' ? (
              <>
                <Alert
                  type="info"
                  showIcon
                  style={{ margin: '8px 0 12px' }}
                  message={t(
                    'pages.settings.cloudBackupGdriveGuide',
                    'Google Drive Setup: (Method 1 - Recommended) Create a Google Cloud Service Account, enable Google Drive API, share your target Google Drive folder with the service account email, and paste the full JSON key below. (Method 2) Paste your OAuth2 Refresh Token below and enter your OAuth2 Client ID & Client Secret.',
                  )}
                />
                <SettingListItem
                  paddings="small"
                  title={t('pages.settings.cloudBackupGdriveFolder', 'Google Drive Folder ID or Link')}
                  description={t('pages.settings.cloudBackupGdriveFolderDesc', 'Paste the Google Drive Folder ID or full folder URL (e.g. https://drive.google.com/drive/folders/1AbCdEf...). Leave empty to upload to Drive root.')}
                >
                  <Input
                    value={allSetting.cloudBackupBucket}
                    placeholder="1AbCdEfGhIjKlMnOpQrStUvWxYz or https://drive.google.com/drive/folders/..."
                    onChange={(e) => updateSetting({ cloudBackupBucket: e.target.value })}
                  />
                </SettingListItem>
                <SettingListItem
                  paddings="small"
                  title={t('pages.settings.cloudBackupGdriveCredentials', 'Service Account JSON or OAuth2 Refresh Token')}
                  description={t('pages.settings.cloudBackupGdriveCredentialsDesc', 'Paste the complete Google Cloud service_account JSON key file, or an OAuth2 Refresh Token (1//0g...).')}
                >
                  <Input.TextArea
                    rows={4}
                    value={allSetting.cloudBackupSecretKey}
                    placeholder={'{\n  "type": "service_account",\n  "client_email": "...",\n  "private_key": "-----BEGIN PRIVATE KEY-----..."\n}'}
                    onChange={(e) => updateSetting({ cloudBackupSecretKey: e.target.value })}
                  />
                </SettingListItem>
                <SettingListItem
                  paddings="small"
                  title={t('pages.settings.cloudBackupGdriveClientId', 'OAuth2 Client ID (Optional)')}
                  description={t('pages.settings.cloudBackupGdriveClientIdDesc', 'Required only when using an OAuth2 Refresh Token instead of a Service Account JSON key.')}
                >
                  <Input
                    value={allSetting.cloudBackupAccessKey}
                    placeholder="1234567890-abc.apps.googleusercontent.com"
                    onChange={(e) => updateSetting({ cloudBackupAccessKey: e.target.value })}
                  />
                </SettingListItem>
                <SettingListItem
                  paddings="small"
                  title={t('pages.settings.cloudBackupGdriveClientSecret', 'OAuth2 Client Secret (Optional)')}
                  description={t('pages.settings.cloudBackupGdriveClientSecretDesc', 'Required only when using an OAuth2 Refresh Token instead of a Service Account JSON key.')}
                >
                  <Input.Password
                    value={allSetting.cloudBackupEndpoint}
                    placeholder="GOCSPX-..."
                    onChange={(e) => updateSetting({ cloudBackupEndpoint: e.target.value })}
                  />
                </SettingListItem>
              </>
            ) : (
              <>
                <SettingListItem
                  paddings="small"
                  title={t('pages.settings.cloudBackupEndpoint', 'Endpoint URL')}
                  description={activeCloudProvider === 'webdav'
                    ? 'e.g. https://u12345.your-storagebox.de/'
                    : 'e.g. https://<account-id>.r2.cloudflarestorage.com or https://s3.amazonaws.com'}
                >
                  <Input
                    value={allSetting.cloudBackupEndpoint}
                    placeholder="https://..."
                    onChange={(e) => updateSetting({ cloudBackupEndpoint: e.target.value })}
                  />
                </SettingListItem>
                {activeCloudProvider !== 'webdav' && (
                  <>
                    <SettingListItem paddings="small" title={t('pages.settings.cloudBackupBucket', 'S3 Bucket Name')}>
                      <Input
                        value={allSetting.cloudBackupBucket}
                        placeholder="omega-backups"
                        onChange={(e) => updateSetting({ cloudBackupBucket: e.target.value })}
                      />
                    </SettingListItem>
                    <SettingListItem paddings="small" title={t('pages.settings.cloudBackupRegion', 'S3 Region')}>
                      <Input
                        value={allSetting.cloudBackupRegion}
                        placeholder="auto"
                        onChange={(e) => updateSetting({ cloudBackupRegion: e.target.value })}
                      />
                    </SettingListItem>
                  </>
                )}
                <SettingListItem
                  paddings="small"
                  title={activeCloudProvider === 'webdav'
                    ? t('pages.settings.cloudBackupWebdavUser', 'WebDAV Username')
                    : t('pages.settings.cloudBackupAccessKey', 'S3 Access Key ID')}
                >
                  <Input
                    value={allSetting.cloudBackupAccessKey}
                    onChange={(e) => updateSetting({ cloudBackupAccessKey: e.target.value })}
                  />
                </SettingListItem>
                <SettingListItem
                  paddings="small"
                  title={activeCloudProvider === 'webdav'
                    ? t('pages.settings.cloudBackupWebdavPass', 'WebDAV Password')
                    : t('pages.settings.cloudBackupSecretKey', 'S3 Secret Access Key')}
                >
                  <Input.Password
                    value={allSetting.cloudBackupSecretKey}
                    onChange={(e) => updateSetting({ cloudBackupSecretKey: e.target.value })}
                  />
                </SettingListItem>
              </>
            )}
            <SettingListItem paddings="small" title={t('pages.settings.cloudBackupPrefix', 'Remote Folder / Prefix')}>
              <Input
                value={allSetting.cloudBackupPrefix}
                placeholder="omega-backups/"
                onChange={(e) => updateSetting({ cloudBackupPrefix: e.target.value })}
              />
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.cloudBackupIntervalHours', 'Backup Interval (Hours)')}>
              <InputNumber
                value={allSetting.cloudBackupIntervalHours}
                min={1}
                max={720}
                style={{ width: '100%' }}
                onChange={(v) => updateSetting({ cloudBackupIntervalHours: Number(v) || 12 })}
              />
            </SettingListItem>
            <SettingListItem
              paddings="small"
              title={t('pages.settings.cloudBackupRetention', 'Retention Limit (Max Backups to Keep)')}
              description={t('pages.settings.cloudBackupRetentionDesc', 'Older backups exceeding this count are automatically deleted (0 = keep all).')}
            >
              <InputNumber
                value={allSetting.cloudBackupRetention}
                min={0}
                max={1000}
                style={{ width: '100%' }}
                onChange={(v) => updateSetting({ cloudBackupRetention: Number(v) || 0 })}
              />
            </SettingListItem>
          </div>
        ),
      },
      {
        key: '8',
        label: catTabLabel(<ThunderboltOutlined />, t('pages.settings.watchdogTitle', 'Outbound Watchdog'), isMobile),
        children: (
          <>
            <SettingListItem
              paddings="small"
              title={t('pages.settings.outboundWatchdogEnable', 'Smart Outbound Auto-Failover Watchdog')}
              description={t('pages.settings.outboundWatchdogEnableDesc', 'Automatically health-check candidate outbounds (e.g. multiple Windscribe WireGuard, WARP, or tunnel outbounds) and promote the fastest healthy backup when the active outbound fails.')}
            >
              <Space wrap>
                <Switch
                  checked={allSetting.outboundWatchdogEnable}
                  onChange={(v) => updateSetting({ outboundWatchdogEnable: v })}
                />
                {watchdogStatus?.activeTag && (
                  <Tag color="green">
                    {t('pages.settings.watchdogActiveTag', 'Active Primary')}: {watchdogStatus.activeTag}
                  </Tag>
                )}
                {watchdogStatus?.lastFailoverTo && (
                  <Tag color="orange">
                    Last Failover: {watchdogStatus.lastFailoverFrom} → {watchdogStatus.lastFailoverTo}
                  </Tag>
                )}
                <Button
                  icon={<ThunderboltOutlined />}
                  loading={watchdogChecking}
                  onClick={runWatchdogCheckNow}
                >
                  {t('pages.settings.watchdogCheckNow', 'Check & Failover Now')}
                </Button>
              </Space>
            </SettingListItem>
            <SettingListItem
              paddings="small"
              title={t('pages.settings.outboundWatchdogTags', 'Candidate Outbound Tags (Priority Order)')}
              description={t('pages.settings.outboundWatchdogTagsDesc', 'Select candidate outbounds in failover priority order (leave empty to automatically monitor all non-direct outbounds).')}
            >
              <Select
                mode="tags"
                style={{ width: '100%' }}
                value={allSetting.outboundWatchdogTags ? allSetting.outboundWatchdogTags.split(',').map((s) => s.trim()).filter(Boolean) : []}
                onChange={(vals: string[]) => updateSetting({ outboundWatchdogTags: vals.join(',') })}
                options={outboundTagList.map((tag) => ({ label: tag, value: tag }))}
                placeholder="windscribe-wg-1, windscribe-wg-2"
              />
            </SettingListItem>
            <SettingListItem
              paddings="small"
              title={t('pages.settings.outboundWatchdogIntervalSec', 'Health-Check Interval (Seconds)')}
            >
              <InputNumber
                value={allSetting.outboundWatchdogIntervalSec}
                min={15}
                max={86400}
                style={{ width: '100%' }}
                onChange={(v) => updateSetting({ outboundWatchdogIntervalSec: Number(v) || 120 })}
              />
            </SettingListItem>
            <SettingListItem
              paddings="small"
              title={t('pages.settings.outboundWatchdogFailThreshold', 'Consecutive Failures Before Failover')}
            >
              <InputNumber
                value={allSetting.outboundWatchdogFailThreshold}
                min={1}
                max={20}
                style={{ width: '100%' }}
                onChange={(v) => updateSetting({ outboundWatchdogFailThreshold: Number(v) || 2 })}
              />
            </SettingListItem>
          </>
        ),
      },
      {
        key: '9',
        label: catTabLabel(<AuditOutlined />, t('pages.settings.auditLogTitle', 'Audit Log'), isMobile),
        children: (
          <div style={{ padding: '12px 0' }}>
            <Space wrap style={{ marginBottom: 12 }}>
              <Input.Search
                allowClear
                placeholder={t('pages.settings.auditSearchPlaceholder', 'Search actor, action, target, IP...')}
                value={auditSearch}
                onChange={(e) => setAuditSearch(e.target.value)}
                onSearch={() => loadAuditLogs()}
                style={{ width: 240 }}
              />
              <Select
                value={auditActorType}
                onChange={(v) => setAuditActorType(v)}
                style={{ width: 150 }}
                options={[
                  { value: '', label: t('pages.settings.auditAllActors', 'All Actors') },
                  { value: 'admin', label: 'Admin' },
                  { value: 'reseller', label: 'Reseller' },
                  { value: 'api_token', label: 'API Token' },
                  { value: 'system', label: 'System / Watchdog' },
                ]}
              />
              <Select
                value={auditModule}
                onChange={(v) => setAuditModule(v)}
                style={{ width: 150 }}
                options={[
                  { value: '', label: t('pages.settings.auditAllModules', 'All Modules') },
                  { value: 'inbounds', label: 'Inbounds' },
                  { value: 'clients', label: 'Clients' },
                  { value: 'resellers', label: 'Resellers' },
                  { value: 'xray', label: 'Xray' },
                  { value: 'settings', label: 'Settings' },
                  { value: 'server', label: 'Server' },
                  { value: 'nodes', label: 'Nodes' },
                ]}
              />
              <Button icon={<ReloadOutlined />} loading={auditLoading} onClick={loadAuditLogs}>
                {t('pages.settings.auditRefresh', 'Refresh')}
              </Button>
              <Button danger icon={<DeleteOutlined />} onClick={clearAuditLogsNow}>
                {t('pages.settings.auditClear', 'Clear Log')}
              </Button>
            </Space>
            <Table<AuditLogEntry>
              rowKey="id"
              size="small"
              loading={auditLoading}
              dataSource={auditLogs}
              pagination={{ pageSize: 15 }}
              scroll={{ x: true }}
              columns={[
                {
                  title: t('pages.settings.auditTime', 'Time'),
                  dataIndex: 'createdAt',
                  width: 170,
                  render: (v: number) => (v ? new Date(v).toLocaleString() : '-'),
                },
                {
                  title: t('pages.settings.auditActor', 'Actor'),
                  key: 'actor',
                  width: 160,
                  render: (_, r) => (
                    <Space size={4}>
                      <Tag color={r.actorType === 'reseller' ? 'purple' : r.actorType === 'system' ? 'orange' : 'blue'}>
                        {r.actorType}
                      </Tag>
                      <span>{r.actorName}</span>
                    </Space>
                  ),
                },
                {
                  title: t('pages.settings.auditModule', 'Module'),
                  dataIndex: 'module',
                  width: 100,
                  render: (m: string) => <Tag>{m}</Tag>,
                },
                {
                  title: t('pages.settings.auditAction', 'Action'),
                  dataIndex: 'action',
                },
                {
                  title: t('pages.settings.auditTarget', 'Target'),
                  dataIndex: 'target',
                  render: (v: string) => v || '-',
                },
                {
                  title: 'IP',
                  dataIndex: 'ip',
                  width: 130,
                  render: (v: string) => v || '-',
                },
                {
                  title: 'HTTP',
                  dataIndex: 'status',
                  width: 75,
                  render: (st: number) => (
                    <Tag color={st >= 200 && st < 400 ? 'green' : 'red'}>{st}</Tag>
                  ),
                },
              ]}
            />
          </div>
        ),
      },
      {
        key: '10',
        label: catTabLabel(<GlobalOutlined />, t('pages.settings.checkHostTitle', 'Iran Check-Host'), isMobile),
        children: (
          <div style={{ padding: '12px 0' }}>
            <SettingListItem
              paddings="small"
              title={t('pages.settings.checkHostTitle', 'Iran & International Filtering Check (Check-Host)')}
              description={t('pages.settings.checkHostDesc', 'Test TCP port reachability or ICMP ping from Iranian cities (Tehran, Shiraz, Tabriz, Isfahan) and Frankfurt to detect port/IP filtering.')}
            >
              <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                <Space wrap>
                  <Input
                    value={checkHostTarget}
                    onChange={(e) => setCheckHostTarget(e.target.value)}
                    placeholder={checkHostMode === 'ping' ? 'example.com' : `example.com:${defaultCheckPort}`}
                    style={{ width: 240 }}
                    onPressEnter={() => runCheckHostNow()}
                  />
                  <Select
                    value={checkHostMode}
                    onChange={(v: 'tcp' | 'ping') => handleCheckHostModeChange(v)}
                    style={{ width: 165 }}
                    options={[
                      { value: 'tcp', label: 'TCP Port Check' },
                      { value: 'ping', label: 'ICMP Ping (IP/Host)' },
                    ]}
                  />
                  <Button
                    type="primary"
                    icon={<GlobalOutlined />}
                    loading={checkHostLoading}
                    onClick={() => runCheckHostNow()}
                  >
                    {t('pages.settings.checkHostRunBtn', 'Run Check-Host Test')}
                  </Button>
                  {checkHostPermLink ? (
                    <Button
                      type="link"
                      href={checkHostPermLink}
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      check-host.net ↗
                    </Button>
                  ) : null}
                </Space>
                <Space wrap size={4}>
                  <Tag
                    color="blue"
                    style={{ cursor: 'pointer' }}
                    onClick={() => {
                      const nextTarget = `${extractHostOnly(checkHostTarget)}:${defaultCheckPort}`;
                      setCheckHostMode('tcp');
                      setCheckHostTarget(nextTarget);
                    }}
                  >
                    Panel :{defaultCheckPort}
                  </Tag>
                  {inboundPorts.map((ib) => (
                    <Tag
                      key={ib.port}
                      color="geekblue"
                      style={{ cursor: 'pointer' }}
                      onClick={() => {
                        const nextTarget = `${extractHostOnly(checkHostTarget)}:${ib.port}`;
                        setCheckHostMode('tcp');
                        setCheckHostTarget(nextTarget);
                      }}
                    >
                      {ib.label}
                    </Tag>
                  ))}
                  <Tag
                    color="purple"
                    style={{ cursor: 'pointer' }}
                    onClick={() => {
                      const nextTarget = extractHostOnly(checkHostTarget);
                      setCheckHostMode('ping');
                      setCheckHostTarget(nextTarget);
                    }}
                  >
                    ICMP Ping ({extractHostOnly(checkHostTarget)})
                  </Tag>
                </Space>
              </div>
            </SettingListItem>

            <Table<CheckHostNodeResult>
              rowKey="node"
              size="small"
              loading={checkHostLoading}
              dataSource={checkHostNodes}
              pagination={false}
              columns={[
                {
                  title: t('pages.settings.checkHostLocation', 'Probe Location'),
                  dataIndex: 'location',
                },
                {
                  title: t('pages.settings.checkHostStatus', 'Reachability'),
                  dataIndex: 'success',
                  width: 185,
                  render: (ok: boolean, r: CheckHostNodeResult) => {
                    if (r.status === 'closed_port') {
                      return (
                        <Tag color="cyan">
                          {t('pages.settings.checkHostClosedPort', 'IP Reachable (Port Closed)')}
                        </Tag>
                      );
                    }
                    if (r.status === 'pending') {
                      return (
                        <Tag color="orange">
                          {t('pages.settings.checkHostPending', 'Node Busy / Pending')}
                        </Tag>
                      );
                    }
                    return (
                      <Tag color={ok ? 'green' : 'red'}>
                        {ok ? t('pages.settings.checkHostOpen', 'Reachable (OK)') : t('pages.settings.checkHostBlocked', 'Filtered / Timeout')}
                      </Tag>
                    );
                  },
                },
                {
                  title: t('pages.settings.checkHostLatency', 'Latency'),
                  dataIndex: 'latencyMs',
                  width: 120,
                  render: (ms: number, r) => (r.success && ms > 0 ? `${ms} ms` : '-'),
                },
                {
                  title: t('pages.settings.checkHostDetail', 'Detail'),
                  dataIndex: 'message',
                  render: (v?: string) => v || '-',
                },
              ]}
            />
          </div>
        ),
      },
    ];

  const singleItem = onlySubTab ? tabItems.find((item) => item.key === onlySubTab) : undefined;
  const visibleTabs = useMemo(
    () => tabItems.filter((item) => item.key !== '6' && item.key !== '7'),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [allSetting, isMobile, lang, t, certStatus, cloudBackingUp, watchdogStatus, watchdogChecking, auditLogs, auditLoading, auditActorType, auditModule, auditSearch, checkHostTarget, checkHostMode, checkHostLoading, checkHostNodes, checkHostPermLink, inboundPorts, inboundOptions, outboundOptions],
  );

  return (
    <>
      {messageContextHolder}
      {singleItem ? (
        <div key={`only-subtab-${singleItem.key}`}>{singleItem.children}</div>
      ) : (
        <Tabs activeKey={activeSubTab} onChange={setActiveSubTab} destroyOnHidden items={visibleTabs} />
      )}

      <Modal
        open={issueOpen}
        title={t('pages.settings.oneClickSslTitle') || 'Issue SSL Certificate'}
        confirmLoading={issuing}
        okText={t('confirm')}
        cancelText={t('cancel')}
        onOk={confirmIssueCert}
        onCancel={() => setIssueOpen(false)}
      >
        <Form layout="vertical">
          <Form.Item label={t('pages.settings.certMode') || 'Certificate Type'}>
            <Select
              value={issueMode}
              onChange={(v) => setIssueMode(v)}
              options={[
                {
                  value: 'letsencrypt',
                  label: t('pages.settings.certModeLetsEncrypt') || "Let's Encrypt (HTTP-01 on port 80, auto-renewed)",
                },
                {
                  value: 'selfsigned',
                  label: t('pages.settings.certModeSelfSigned') || 'Self-Signed (instant, 1 year, no port 80 needed)',
                },
              ]}
            />
          </Form.Item>
          <Form.Item label={t('pages.settings.certDomain') || 'Domain'} required>
            <Input
              value={issueDomain}
              placeholder="panel.example.com"
              onChange={(e) => setIssueDomain(e.target.value)}
              onPressEnter={confirmIssueCert}
            />
          </Form.Item>
          {issueMode === 'letsencrypt' ? (
            <Form.Item
              label={t('pages.settings.certEmail') || 'Contact Email (optional)'}
              extra={t('pages.settings.certPort80Notice') || 'Port 80 must be reachable from the internet and point to this server.'}
            >
              <Input
                value={issueEmail}
                placeholder="admin@example.com"
                onChange={(e) => setIssueEmail(e.target.value)}
              />
            </Form.Item>
          ) : null}
        </Form>
      </Modal>
    </>
  );
}
