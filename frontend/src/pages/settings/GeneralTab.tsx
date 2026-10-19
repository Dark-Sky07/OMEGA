import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Form,
  Input,
  InputNumber,
  Modal,
  Select,
  Space,
  Switch,
  Tabs,
  Tag,
  message,
} from 'antd';
import {
  ApartmentOutlined,
  BellOutlined,
  ClockCircleOutlined,
  CloudUploadOutlined,
  GlobalOutlined,
  SafetyCertificateOutlined,
  SettingOutlined,
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

interface GeneralTabProps {
  allSetting: AllSetting;
  updateSetting: (patch: Partial<AllSetting>) => void;
}

const DATEPICKER_LIST: { name: string; value: 'gregorian' | 'jalalian' }[] = [
  { name: 'Gregorian (Standard)', value: 'gregorian' },
  { name: 'Jalalian (شمسی)', value: 'jalalian' },
];

export default function GeneralTab({ allSetting, updateSetting }: GeneralTabProps) {
  const { t } = useTranslation();
  const { isMobile } = useMediaQuery();

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
  const [messageApi, messageContextHolder] = message.useMessage();

  async function runCloudBackupNow() {
    setCloudBackingUp(true);
    try {
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
      } else {
        setInboundOptions([]);
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

  return (
    <>
      {messageContextHolder}
      <Tabs defaultActiveKey="1" items={[
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
          <>
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
          </>
        ),
      },
      {
        key: '6',
        label: catTabLabel(<CloudUploadOutlined />, t('pages.settings.cloudBackupTitle', 'Cloud Backup'), isMobile),
        children: (
          <>
            <SettingListItem
              paddings="small"
              title={t('pages.settings.cloudBackupEnable', 'Scheduled Cloud Backup')}
              description={t('pages.settings.cloudBackupEnableDesc', 'Automatically upload database snapshots to S3 / Cloudflare R2 / MinIO or WebDAV and prune older backups.')}
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
                value={allSetting.cloudBackupProvider || 's3'}
                style={{ width: '100%' }}
                onChange={(v) => updateSetting({ cloudBackupProvider: v })}
                options={[
                  { value: 's3', label: 'S3 / Cloudflare R2 / MinIO (AWS Signature v4)' },
                  { value: 'webdav', label: 'WebDAV (Nextcloud / Hetzner Storage Box / Koofr)' },
                ]}
              />
            </SettingListItem>
            <SettingListItem
              paddings="small"
              title={t('pages.settings.cloudBackupEndpoint', 'Endpoint URL')}
              description={allSetting.cloudBackupProvider === 'webdav'
                ? 'e.g. https://u12345.your-storagebox.de/'
                : 'e.g. https://<account-id>.r2.cloudflarestorage.com or https://s3.amazonaws.com'}
            >
              <Input
                value={allSetting.cloudBackupEndpoint}
                placeholder="https://..."
                onChange={(e) => updateSetting({ cloudBackupEndpoint: e.target.value })}
              />
            </SettingListItem>
            {allSetting.cloudBackupProvider !== 'webdav' && (
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
            <SettingListItem paddings="small" title={t('pages.settings.cloudBackupPrefix', 'Remote Folder / Prefix')}>
              <Input
                value={allSetting.cloudBackupPrefix}
                placeholder="omega-backups/"
                onChange={(e) => updateSetting({ cloudBackupPrefix: e.target.value })}
              />
            </SettingListItem>
            <SettingListItem
              paddings="small"
              title={allSetting.cloudBackupProvider === 'webdav'
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
              title={allSetting.cloudBackupProvider === 'webdav'
                ? t('pages.settings.cloudBackupWebdavPass', 'WebDAV Password')
                : t('pages.settings.cloudBackupSecretKey', 'S3 Secret Access Key')}
            >
              <Input.Password
                value={allSetting.cloudBackupSecretKey}
                onChange={(e) => updateSetting({ cloudBackupSecretKey: e.target.value })}
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
          </>
        ),
      },
    ]} />

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
