import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Divider, Modal, Popover, Space, Tag, Tooltip, message } from 'antd';
import { CopyOutlined, DownloadOutlined, EyeOutlined, QrcodeOutlined, ReloadOutlined } from '@ant-design/icons';

import { ClipboardManager, HttpUtil, IntlUtil, SizeFormatter } from '@/utils';
import { formatInboundLabel } from '@/lib/inbounds/label';
import { useDatepicker } from '@/hooks/useDatepicker';
import type { ClientRecord, InboundOption } from '@/hooks/useClients';
import { isPostQuantumLink } from '@/lib/xray/inbound-link';
import { LinkTags, linkMetaText, parseLinkParts } from '@/lib/xray/link-label';
import { QrPanel } from '@/pages/inbounds/qr';
import './ClientInfoModal.css';

const INBOUND_PROTOCOL_COLORS: Record<string, string> = {
  vless: 'blue',
  vmess: 'geekblue',
  trojan: 'volcano',
  shadowsocks: 'magenta',
  hysteria: 'cyan',
  hysteria2: 'green',
  wireguard: 'gold',
  http: 'purple',
  mixed: 'lime',
  tunnel: 'orange',
  openvpn: 'red',
  l2tp: 'cyan',
};

const INBOUND_CHIP_LIMIT = 1;

interface SubSettings {
  enable: boolean;
  subURI: string;
  subJsonURI: string;
  subJsonEnable: boolean;
  subClashURI: string;
  subClashEnable: boolean;
}

interface ClientInfoModalProps {
  open: boolean;
  client: ClientRecord | null;
  inboundsById: Record<number, InboundOption>;
  isOnline: boolean;
  subSettings?: SubSettings;
  onOpenChange: (open: boolean) => void;
}

interface ApiMsg<T = unknown> {
  success?: boolean;
  msg?: string;
  obj?: T;
}

const DEFAULT_SUB: SubSettings = {
  enable: false,
  subURI: '',
  subJsonURI: '',
  subJsonEnable: false,
  subClashURI: '',
  subClashEnable: false,
};

type L2TPOption = NonNullable<InboundOption['l2tp']>;

const DEFAULT_L2TP_CONFIG: L2TPOption = {
  fixedPorts: [500, 4500, 1701],
  poolCIDR: '10.252.0.0/24',
  localIP: '10.252.0.1',
  poolStart: '10.252.0.10',
  poolEnd: '10.252.0.250',
  dns1: '1.1.1.1',
  dns2: '8.8.8.8',
  redirectGateway: true,
};

function parseL2TPSettings(value: unknown): L2TPOption {
  let raw: Record<string, unknown> = {};
  if (typeof value === 'string') {
    try {
      const parsed: unknown = JSON.parse(value);
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        raw = parsed as Record<string, unknown>;
      }
    } catch {
      // Keep daemon defaults when a legacy inbound has malformed settings.
    }
  } else if (value && typeof value === 'object' && !Array.isArray(value)) {
    raw = value as Record<string, unknown>;
  }

  const text = (key: string) => typeof raw[key] === 'string' ? raw[key] as string : undefined;
  return {
    ...DEFAULT_L2TP_CONFIG,
    psk: text('psk'),
    poolCIDR: text('poolCIDR') || DEFAULT_L2TP_CONFIG.poolCIDR,
    localIP: text('localIP') || DEFAULT_L2TP_CONFIG.localIP,
    poolStart: text('poolStart') || DEFAULT_L2TP_CONFIG.poolStart,
    poolEnd: text('poolEnd') || DEFAULT_L2TP_CONFIG.poolEnd,
    dns1: text('dns1') || DEFAULT_L2TP_CONFIG.dns1,
    dns2: text('dns2') || DEFAULT_L2TP_CONFIG.dns2,
    redirectGateway: raw.redirectGateway === false ? false : true,
  };
}

export default function ClientInfoModal({
  open,
  client,
  inboundsById,
  isOnline,
  subSettings = DEFAULT_SUB,
  onOpenChange,
}: ClientInfoModalProps) {
  const { datepicker } = useDatepicker();
  const { t } = useTranslation();
  const expiryLabel = (ts?: number) => {
    if (!ts) return '∞';
    if (ts < 0) {
      const days = Math.round(ts / -86400000);
      return `${t('pages.clients.delayedStart')}: ${days}d`;
    }
    return IntlUtil.formatDate(ts, datepicker);
  };
  const dateLabel = (ts?: number) => (!ts || ts <= 0 ? '-' : IntlUtil.formatDate(ts, datepicker));
  const [messageApi, messageContextHolder] = message.useMessage();
  const [links, setLinks] = useState<string[]>([]);
  const [clientIps, setClientIps] = useState<string[]>([]);
  const [ipsLoading, setIpsLoading] = useState(false);
  const [ipsClearing, setIpsClearing] = useState(false);
  const [ipsModalOpen, setIpsModalOpen] = useState(false);

  useEffect(() => {
    if (!open) {
      setLinks([]);
      setClientIps([]);
      setIpsModalOpen(false);
      return;
    }
    if (!client?.subId) return;
    let cancelled = false;
    (async () => {
      const msg = await HttpUtil.get(
        `/panel/api/clients/subLinks/${encodeURIComponent(client.subId!)}`,
      ) as ApiMsg<string[]>;
      if (cancelled) return;
      setLinks(msg?.success && Array.isArray(msg.obj) ? msg.obj : []);
    })();
    return () => { cancelled = true; };
  }, [open, client?.subId]);

  // OpenVPN inbounds are daemon-served (no share links), so the only thing
  // worth surfacing here is the per-client .ovpn profile.
  const openvpnInboundCount = useMemo(() => {
    const ids = client?.inboundIds ?? [];
    return ids.filter((id) => (inboundsById[id]?.protocol || '').toLowerCase() === 'openvpn').length;
  }, [client?.inboundIds, inboundsById]);

  const l2tpInboundIds = useMemo(
    () => (client?.inboundIds ?? []).filter((id) =>
      (inboundsById[id]?.protocol || '').toLowerCase() === 'l2tp',
    ),
    [client?.inboundIds, inboundsById],
  );
  const [legacyL2TPConfigs, setLegacyL2TPConfigs] = useState<Record<number, L2TPOption>>({});

  // Older panel binaries expose L2TP in the picker but do not yet include
  // the l2tp metadata projection. Hydrate those specific inbound settings so
  // Client Information remains useful during a rolling upgrade.
  useEffect(() => {
    if (!open) {
      setLegacyL2TPConfigs({});
      return;
    }
    const pendingIds = l2tpInboundIds.filter((id) => !inboundsById[id]?.l2tp);
    if (pendingIds.length === 0) return;
    let cancelled = false;
    (async () => {
      const entries = await Promise.all(pendingIds.map(async (id) => {
        const msg = await HttpUtil.get(`/panel/api/inbounds/get/${id}`, undefined, { silent: true }) as ApiMsg<{ settings?: unknown }>;
        if (!msg?.success || !msg.obj) return null;
        return [id, parseL2TPSettings(msg.obj.settings)] as const;
      }));
      if (cancelled) return;
      setLegacyL2TPConfigs((previous) => {
        const next = { ...previous };
        for (const entry of entries) {
          if (entry) next[entry[0]] = entry[1];
        }
        return next;
      });
    })();
    return () => { cancelled = true; };
  }, [open, l2tpInboundIds, inboundsById]);

  const l2tpInbounds = useMemo(
    () => (client?.inboundIds ?? [])
      .map((id) => inboundsById[id])
      .filter((ib): ib is InboundOption =>
        !!ib && (ib.protocol || '').toLowerCase() === 'l2tp',
      )
      .map((ib) => ({
        ...ib,
        l2tp: ib.l2tp || legacyL2TPConfigs[ib.id] || DEFAULT_L2TP_CONFIG,
      })),
    [client?.inboundIds, inboundsById, legacyL2TPConfigs],
  );
  const [ovpnProfile, setOvpnProfile] = useState('');
  const [ovpnLoading, setOvpnLoading] = useState(false);

  useEffect(() => {
    if (!open) setOvpnProfile('');
  }, [open]);

  async function fetchOvpnProfile() {
    if (!client) return;
    setOvpnLoading(true);
    try {
      const msg = await HttpUtil.get(
        `/panel/api/clients/openvpn/${encodeURIComponent(client.email)}`,
      ) as ApiMsg<{ profile: string }>;
      if (msg?.success && typeof msg.obj?.profile === 'string' && msg.obj.profile) {
        setOvpnProfile(msg.obj.profile);
      } else {
        messageApi.error(msg?.msg || t('error'));
      }
    } finally {
      setOvpnLoading(false);
    }
  }

  function downloadOvpnProfile() {
    if (!ovpnProfile || !client) return;
    const safeName = client.email.replace(/[^a-zA-Z0-9._-]/g, '_') || 'client';
    const blob = new Blob([ovpnProfile], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${safeName}.ovpn`;
    a.click();
    URL.revokeObjectURL(url);
  }

  const traffic = client?.traffic || null;
  const totalBytes = client?.totalGB || 0;
  const used = (traffic?.up || 0) + (traffic?.down || 0);
  const remaining = useMemo(() => {
    if (totalBytes <= 0) return -1;
    const r = totalBytes - used;
    return r > 0 ? r : 0;
  }, [totalBytes, used]);

  const subLink = useMemo(() => {
    if (!client?.subId || !subSettings?.subURI) return '';
    return subSettings.subURI + client.subId;
  }, [client?.subId, subSettings?.subURI]);

  const subJsonLink = useMemo(() => {
    if (!client?.subId) return '';
    if (!subSettings?.subJsonEnable || !subSettings?.subJsonURI) return '';
    return subSettings.subJsonURI + client.subId;
  }, [client?.subId, subSettings?.subJsonEnable, subSettings?.subJsonURI]);

  const subClashLink = useMemo(() => {
    if (!client?.subId) return '';
    if (!subSettings?.subClashEnable || !subSettings?.subClashURI) return '';
    return subSettings.subClashURI + client.subId;
  }, [client?.subId, subSettings?.subClashEnable, subSettings?.subClashURI]);

  const hasXraySubscriptionInbound = useMemo(
    () => (client?.inboundIds ?? []).some((id) => {
      const protocol = (inboundsById[id]?.protocol || '').toLowerCase();
      return protocol !== '' && protocol !== 'openvpn' && protocol !== 'l2tp';
    }),
    [client?.inboundIds, inboundsById],
  );
  const showSubscription = !!(subSettings?.enable && client?.subId && hasXraySubscriptionInbound);

  async function copyValue(text: string) {
    if (!text) return;
    const ok = await ClipboardManager.copyText(String(text));
    if (ok) messageApi.success(t('copied'));
  }

  function nativeValue(value?: string, allowCopy = true) {
    const text = value || '-';
    return (
      <Space size={4} wrap>
        <Tag className="info-large-tag">{text}</Tag>
        {allowCopy && value && (
          <Button size="small" type="text" icon={<CopyOutlined />} onClick={() => copyValue(value)} />
        )}
      </Space>
    );
  }

  async function loadIps() {
    if (!client?.email) return;
    setIpsLoading(true);
    try {
      const msg = await HttpUtil.post(`/panel/api/clients/ips/${encodeURIComponent(client.email)}`) as ApiMsg<unknown[]>;
      if (!msg?.success) { setClientIps([]); return; }
      const arr = Array.isArray(msg.obj) ? msg.obj : [];
      setClientIps(arr.filter((x): x is string => typeof x === 'string' && x.length > 0));
    } finally {
      setIpsLoading(false);
    }
  }

  async function clearIps() {
    if (!client?.email) return;
    setIpsClearing(true);
    try {
      const msg = await HttpUtil.post(`/panel/api/clients/clearIps/${encodeURIComponent(client.email)}`) as ApiMsg;
      if (msg?.success) setClientIps([]);
    } finally {
      setIpsClearing(false);
    }
  }

  function openIpsModal() {
    setIpsModalOpen(true);
    if (clientIps.length === 0) void loadIps();
  }

  return (
    <>
      {messageContextHolder}
      <Modal
        open={open}
        title={client ? `${t('pages.clients.clientInfo')} — ${client.email}` : t('pages.clients.clientInfo')}
        footer={null}
        width={640}
        onCancel={() => onOpenChange(false)}
      >
        {client && (
          <>
            <table className="info-table block">
              <tbody>
                <tr>
                  <td>{t('pages.clients.online')}</td>
                  <td>
                    {client.enable && isOnline
                      ? <Tag color="green">{t('pages.clients.online')}</Tag>
                      : <Tag>{t('pages.clients.offline')}</Tag>}
                    <span className="hint">{t('lastOnline')}: {dateLabel(traffic?.lastOnline)}</span>
                  </td>
                </tr>
                <tr>
                  <td>{t('status')}</td>
                  <td>
                    <Tag color={client.enable ? 'green' : 'default'}>
                      {client.enable ? t('enabled') : t('disabled')}
                    </Tag>
                  </td>
                </tr>
                <tr>
                  <td>{t('pages.clients.email')}</td>
                  <td>
                    {client.email
                      ? <Tag color="green">{client.email}</Tag>
                      : <Tag color="red">{t('none')}</Tag>}
                  </td>
                </tr>
                <tr>
                  <td>{t('pages.clients.subId')}</td>
                  <td>
                    <Tag className="info-large-tag">{client.subId || '-'}</Tag>
                    {client.subId && (
                      <Button size="small" type="text" icon={<CopyOutlined />} onClick={() => copyValue(client.subId!)} />
                    )}
                  </td>
                </tr>
                {client.uuid && (
                  <tr>
                    <td>{t('pages.clients.uuid')}</td>
                    <td>
                      <Tag className="info-large-tag">{client.uuid}</Tag>
                      <Button size="small" type="text" icon={<CopyOutlined />} onClick={() => copyValue(client.uuid!)} />
                    </td>
                  </tr>
                )}
                {client.password && (
                  <tr>
                    <td>{t('password')}</td>
                    <td>
                      <Tag className="info-large-tag">{client.password}</Tag>
                      <Button size="small" type="text" icon={<CopyOutlined />} onClick={() => copyValue(client.password!)} />
                    </td>
                  </tr>
                )}
                {client.auth && (
                  <tr>
                    <td>{t('pages.clients.auth')}</td>
                    <td>
                      <Tag className="info-large-tag">{client.auth}</Tag>
                      <Button size="small" type="text" icon={<CopyOutlined />} onClick={() => copyValue(client.auth!)} />
                    </td>
                  </tr>
                )}
                <tr>
                  <td>{t('pages.clients.flow')}</td>
                  <td>
                    {client.flow ? <Tag>{client.flow}</Tag> : <Tag color="orange">{t('none')}</Tag>}
                  </td>
                </tr>
                <tr>
                  <td>{t('pages.inbounds.traffic')}</td>
                  <td>
                    <Tag>
                      ↑ {SizeFormatter.sizeFormat(traffic?.up || 0)}
                      {' '}/ ↓ {SizeFormatter.sizeFormat(traffic?.down || 0)}
                    </Tag>
                    <span className="hint">
                      {SizeFormatter.sizeFormat(used)} / {totalBytes > 0 ? SizeFormatter.sizeFormat(totalBytes) : '∞'}
                    </span>
                  </td>
                </tr>
                <tr>
                  <td>{t('remained')}</td>
                  <td>
                    {remaining < 0
                      ? <Tag color="purple">∞</Tag>
                      : <Tag color={remaining > 0 ? '' : 'red'}>{SizeFormatter.sizeFormat(remaining)}</Tag>}
                  </td>
                </tr>
                <tr>
                  <td>{t('pages.inbounds.expireDate')}</td>
                  <td>
                    {!client.expiryTime
                      ? <Tag color="purple">∞</Tag>
                      : <Tag color={client.expiryTime < 0 ? 'blue' : undefined}>{expiryLabel(client.expiryTime)}</Tag>}
                    {(client.expiryTime ?? 0) > 0 && (
                      <span className="hint">{IntlUtil.formatRelativeTime(client.expiryTime)}</span>
                    )}
                  </td>
                </tr>
                <tr>
                  <td>{t('pages.clients.ipLimit')}</td>
                  <td>{!client.limitIp ? <Tag>∞</Tag> : <Tag>{client.limitIp}</Tag>}</td>
                </tr>
                <tr>
                  <td>{t('pages.inbounds.IPLimitlog')}</td>
                  <td>
                    <Button size="small" icon={<EyeOutlined />} loading={ipsLoading} onClick={openIpsModal}>
                      {clientIps.length > 0 ? clientIps.length : ''}
                    </Button>
                  </td>
                </tr>
                <tr>
                  <td>{t('pages.inbounds.createdAt')}</td>
                  <td><Tag>{dateLabel(client.createdAt)}</Tag></td>
                </tr>
                <tr>
                  <td>{t('pages.inbounds.updatedAt')}</td>
                  <td><Tag>{dateLabel(client.updatedAt)}</Tag></td>
                </tr>
                {client.comment && (
                  <tr>
                    <td>{t('pages.clients.comment')}</td>
                    <td><Tag className="info-large-tag">{client.comment}</Tag></td>
                  </tr>
                )}
                <tr>
                  <td>{t('pages.clients.attachedInbounds')}</td>
                  <td>
                    {(() => {
                      const ids = client.inboundIds || [];
                      if (ids.length === 0) return <span className="hint">—</span>;
                      const visible = ids.slice(0, INBOUND_CHIP_LIMIT);
                      const overflow = ids.slice(INBOUND_CHIP_LIMIT);
                      const inboundChip = (id: number) => {
                        const ib = inboundsById[id];
                        const proto = (ib?.protocol || '').toLowerCase();
                        const color = INBOUND_PROTOCOL_COLORS[proto] ?? 'default';
                        const label = formatInboundLabel(ib?.tag, ib?.remark);
                        return (
                          <Tooltip key={id} title={label}>
                            <Tag color={color}>{label}</Tag>
                          </Tooltip>
                        );
                      };
                      return (
                        <div className="chips">
                          {visible.map((id) => inboundChip(id))}
                          {overflow.length > 0 && (
                            <Popover
                              trigger="click"
                              placement="bottomRight"
                              content={
                                <div className="chips chips-stack">
                                  {overflow.map((id) => inboundChip(id))}
                                </div>
                              }
                            >
                              <Tag color="default" className="chip-more">
                                +{overflow.length} {t('more') !== 'more' ? t('more') : 'more'}
                              </Tag>
                            </Popover>
                          )}
                        </div>
                      );
                    })()}
                  </td>
                </tr>
              </tbody>
            </table>

            {links.length > 0 && hasXraySubscriptionInbound && (
              <>
                <Divider>{t('pages.inbounds.copyLink')}</Divider>
                {links.map((link, idx) => {
                  const parts = parseLinkParts(link, client.email);
                  const fallback = `${t('pages.clients.link')} ${idx + 1}`;
                  const rowTitle = (parts && linkMetaText(parts)) || fallback;
                  const qrRemark = [parts?.remark, client.email].filter(Boolean).join('-') || rowTitle;
                  const canQr = !isPostQuantumLink(link);
                  return (
                    <div key={idx} className="link-row">
                      {parts
                        ? <LinkTags parts={parts} />
                        : <Tag className="link-row-tag">LINK</Tag>}
                      <span className="link-row-title" title={rowTitle}>{rowTitle}</span>
                      <div className="link-row-actions">
                        <Tooltip title={t('copy')}>
                          <Button size="small" icon={<CopyOutlined />} onClick={() => copyValue(link)} />
                        </Tooltip>
                        {canQr && (
                          <Popover
                            trigger="click"
                            placement="left"
                            destroyOnHidden
                            content={<QrPanel value={link} remark={qrRemark} size={220} />}
                          >
                            <Tooltip title={t('pages.clients.qrCode')}>
                              <Button size="small" icon={<QrcodeOutlined />} />
                            </Tooltip>
                          </Popover>
                        )}
                      </div>
                    </div>
                  );
                })}
              </>
            )}

            {openvpnInboundCount > 0 && (
              <>
                <Divider>{t('pages.clients.openvpnConfig')}</Divider>
                <div className="link-row">
                  <Tag color="red" className="link-row-tag">OVPN</Tag>
                  <span className="link-row-title" title={t('pages.clients.openvpnConfigHint')}>
                    {t('pages.clients.openvpnConfig')}
                  </span>
                  <div className="link-row-actions">
                    <Tooltip title={t('pages.clients.openvpnFetch')}>
                      <Button
                        size="small"
                        icon={<ReloadOutlined />}
                        loading={ovpnLoading}
                        onClick={fetchOvpnProfile}
                      />
                    </Tooltip>
                    {ovpnProfile && (
                      <>
                        <Tooltip title={t('copy')}>
                          <Button
                            size="small"
                            icon={<CopyOutlined />}
                            onClick={() => copyValue(ovpnProfile)}
                          />
                        </Tooltip>
                        <Tooltip title={t('pages.clients.openvpnDownload')}>
                          <Button size="small" icon={<DownloadOutlined />} onClick={downloadOvpnProfile} />
                        </Tooltip>
                      </>
                    )}
                  </div>
                </div>
              </>
            )}

            {l2tpInbounds.length > 0 && (
              <>
                <Divider>L2TP/IPsec</Divider>
                {l2tpInbounds.map((ib) => {
                  const config = ib.l2tp;
                  const serverAddress = config.serverAddress || window.location.hostname;
                  const fixedPorts = config.fixedPorts?.length ? config.fixedPorts : [500, 4500, 1701];
                  return (
                    <div className="l2tp-client-panel" key={ib.id}>
                      <div className="link-row l2tp-client-header">
                        <Tag color="cyan" className="link-row-tag">L2TP</Tag>
                        <span className="link-row-title">
                          {formatInboundLabel(ib.tag, ib.remark)}
                        </span>
                      </div>
                      <table className="info-table block l2tp-client-table">
                        <tbody>
                          <tr>
                            <td>{t('pages.inbounds.form.l2tpServerAddress')}</td>
                            <td>{nativeValue(serverAddress || undefined)}</td>
                          </tr>
                          <tr>
                            <td>{t('pages.inbounds.form.l2tpFixedPorts')}</td>
                            <td>{nativeValue(fixedPorts.join(', '))}</td>
                          </tr>
                          <tr>
                            <td>{t('pages.clients.email')}</td>
                            <td>{nativeValue(client.email)}</td>
                          </tr>
                          <tr>
                            <td>{t('password')}</td>
                            <td>{nativeValue(client.password)}</td>
                          </tr>
                          <tr>
                            <td>{t('pages.inbounds.form.l2tpPsk')}</td>
                            <td>{nativeValue(config.psk)}</td>
                          </tr>
                          <tr>
                            <td>{t('pages.inbounds.form.l2tpPoolCIDR')}</td>
                            <td>{nativeValue(config.poolCIDR)}</td>
                          </tr>
                          <tr>
                            <td>{t('pages.inbounds.form.l2tpLocalIP')}</td>
                            <td>{nativeValue(config.localIP)}</td>
                          </tr>
                          <tr>
                            <td>{t('pages.inbounds.form.l2tpPoolRange')}</td>
                            <td>{nativeValue([config.poolStart, config.poolEnd].filter(Boolean).join(' — '))}</td>
                          </tr>
                          <tr>
                            <td>{t('pages.inbounds.form.l2tpDnsServers')}</td>
                            <td>{nativeValue([config.dns1, config.dns2].filter(Boolean).join(' / '))}</td>
                          </tr>
                          <tr>
                            <td>{t('pages.inbounds.form.l2tpFullTunnelHint')}</td>
                            <td>
                              <Tag color={config.redirectGateway ? 'green' : 'default'}>
                                {config.redirectGateway ? t('enabled') : t('disabled')}
                              </Tag>
                            </td>
                          </tr>
                        </tbody>
                      </table>
                      <div className="l2tp-client-note">
                        {t('pages.inbounds.form.l2tpManualParameters')}
                      </div>
                    </div>
                  );
                })}
              </>
            )}

            {showSubscription && subLink && (
              <>
                <Divider>{t('subscription.title')}</Divider>
                <div className="link-row">
                  <Tag color="green" className="link-row-tag">SUB</Tag>
                  <a
                    href={subLink}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="link-row-title link-row-title-anchor"
                    title={subLink}
                  >
                    {client.subId}
                  </a>
                  <div className="link-row-actions">
                    <Tooltip title={t('copy')}>
                      <Button size="small" icon={<CopyOutlined />} onClick={() => copyValue(subLink)} />
                    </Tooltip>
                    <Popover
                      trigger="click"
                      placement="left"
                      destroyOnHidden
                      content={<QrPanel value={subLink} remark={`${client.email} — ${t('subscription.title')}`} size={220} />}
                    >
                      <Tooltip title={t('pages.clients.qrCode')}>
                        <Button size="small" icon={<QrcodeOutlined />} />
                      </Tooltip>
                    </Popover>
                  </div>
                </div>
                {subJsonLink && (
                  <div className="link-row">
                    <Tag color="purple" className="link-row-tag">JSON</Tag>
                    <a
                      href={subJsonLink}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="link-row-title link-row-title-anchor"
                      title={subJsonLink}
                    >
                      {client.subId}
                    </a>
                    <div className="link-row-actions">
                      <Tooltip title={t('copy')}>
                        <Button size="small" icon={<CopyOutlined />} onClick={() => copyValue(subJsonLink)} />
                      </Tooltip>
                      <Popover
                        trigger="click"
                        placement="left"
                        destroyOnHidden
                        content={<QrPanel value={subJsonLink} remark={`${client.email} — JSON`} size={220} />}
                      >
                        <Tooltip title={t('pages.clients.qrCode')}>
                          <Button size="small" icon={<QrcodeOutlined />} />
                        </Tooltip>
                      </Popover>
                    </div>
                  </div>
                )}
                {subClashLink && (
                  <div className="link-row">
                    <Tooltip title="Clash / Mihomo">
                      <Tag color="gold" className="link-row-tag">CLASH</Tag>
                    </Tooltip>
                    <a
                      href={subClashLink}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="link-row-title link-row-title-anchor"
                      title={subClashLink}
                    >
                      {client.subId}
                    </a>
                    <div className="link-row-actions">
                      <Tooltip title={t('copy')}>
                        <Button size="small" icon={<CopyOutlined />} onClick={() => copyValue(subClashLink)} />
                      </Tooltip>
                      <Popover
                        trigger="click"
                        placement="left"
                        destroyOnHidden
                        content={<QrPanel value={subClashLink} remark={`${client.email} — Clash / Mihomo`} size={220} />}
                      >
                        <Tooltip title={t('pages.clients.qrCode')}>
                          <Button size="small" icon={<QrcodeOutlined />} />
                        </Tooltip>
                      </Popover>
                    </div>
                  </div>
                )}
              </>
            )}
          </>
        )}
      </Modal>

      <Modal
        open={ipsModalOpen}
        title={`${t('pages.inbounds.IPLimitlog')}${client?.email ? ` — ${client.email}` : ''}`}
        width={440}
        onCancel={() => setIpsModalOpen(false)}
        footer={[
          <Button key="refresh" icon={<ReloadOutlined />} loading={ipsLoading} onClick={loadIps}>
            {t('refresh')}
          </Button>,
          <Button key="clear" danger loading={ipsClearing} disabled={clientIps.length === 0} onClick={clearIps}>
            {t('pages.clients.clearAll')}
          </Button>,
          <Button key="close" type="primary" onClick={() => setIpsModalOpen(false)}>
            {t('close')}
          </Button>,
        ]}
      >
        {clientIps.length > 0 ? (
          <div style={{ maxHeight: 360, overflowY: 'auto' }}>
            {clientIps.map((ip, idx) => (
              <Tag
                key={idx}
                color="blue"
                style={{
                  display: 'block',
                  width: 'fit-content',
                  maxWidth: '100%',
                  marginBottom: 6,
                  padding: '2px 8px',
                  fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
                }}
              >
                {ip}
              </Tag>
            ))}
          </div>
        ) : (
          <Tag>{t('tgbot.noIpRecord')}</Tag>
        )}
      </Modal>
    </>
  );
}
