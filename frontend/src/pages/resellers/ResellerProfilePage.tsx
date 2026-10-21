import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Alert,
  Badge,
  Button,
  Card,
  Col,
  ConfigProvider,
  Descriptions,
  Form,
  Input,
  Layout,
  List,
  Progress,
  Row,
  Space,
  Statistic,
  Tag,
  Select,
  Typography,
  message,
} from 'antd';
import { BellOutlined, CheckOutlined, CopyOutlined, CustomerServiceOutlined, GiftOutlined, SendOutlined, ShopOutlined, UserOutlined, WalletOutlined } from '@ant-design/icons';

import AppSidebar from '@/layouts/AppSidebar';
import { PageHeader } from '@/components/ui';
import { HttpUtil } from '@/utils';
import { SizeFormatter } from '@/utils';
import { setMessageInstance } from '@/utils/messageBus';
import { keys } from '@/api/queryKeys';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { useTheme } from '@/hooks/useTheme';
import type { ResellerStat } from '@/api/queries/useSession';
import SupportTicketsModal from './SupportTicketsModal';
import './ResellersPage.css';

const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } } as const;

type ResellerNotificationRow = {
  id: number;
  kind: string;
  params: string;
  isRead: boolean;
  createdAt: number;
};

type ResellerNotificationFeed = {
  list: ResellerNotificationRow[];
  unreadCount: number;
};

type TelegramLinkInfo = {
  code: string;
  expiresAt: number;
  botUsername?: string;
};

type ResellerBrandingValues = {
  brandName: string;
  brandLogo: string;
  brandColor: string;
};

const NOTIF_TAG_COLOR: Record<string, string> = {
  quota80: 'gold',
  quota95: 'orange',
  quota_out: 'red',
  climit80: 'gold',
  climit_max: 'red',
  expiry_soon: 'orange',
  expired: 'red',
};

function notifKindGroup(kind: string): 'quota' | 'clients' | 'expiry' {
  if (kind.startsWith('quota')) return 'quota';
  if (kind.startsWith('climit')) return 'clients';
  return 'expiry';
}

function parseNotifParams(params: string): Record<string, unknown> {
  if (!params) return {};
  try {
    const parsed = JSON.parse(params) as Record<string, unknown>;
    return parsed && typeof parsed === 'object' ? parsed : {};
  } catch {
    return {};
  }
}

function notifTitle(
  t: (key: string, opts?: Record<string, unknown>) => string,
  kind: string,
  params: string,
): string {
  return t(`resellers.notif.${kind}.title`, parseNotifParams(params));
}

export default function ResellerProfilePage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const { isMobile } = useMediaQuery();
  const queryClient = useQueryClient();
  const [messageApi, messageContextHolder] = message.useMessage();
  const [saving, setSaving] = useState(false);
  const [brandingSaving, setBrandingSaving] = useState(false);
  const [ticketsOpen, setTicketsOpen] = useState(false);
  const [voucherCode, setVoucherCode] = useState('');
  const [redeeming, setRedeeming] = useState(false);
  const [tgLink, setTgLink] = useState<TelegramLinkInfo | null>(null);
  const [tgLinking, setTgLinking] = useState(false);
  const [tgUnlinking, setTgUnlinking] = useState(false);
  const [signupTemplateId, setSignupTemplateId] = useState<number>();
  const [signupLink, setSignupLink] = useState('');
  const [signupLinkLoading, setSignupLinkLoading] = useState(false);
  const [depositLink, setDepositLink] = useState('');
  const [depositLinkLoading, setDepositLinkLoading] = useState(false);
  const [passwordForm] = Form.useForm<{ oldPassword: string; newPassword: string; confirmPassword: string }>();
  const [brandingForm] = Form.useForm<ResellerBrandingValues>();

  const redeemVoucher = useCallback(async () => {
    const code = voucherCode.trim();
    if (!code) return;
    setRedeeming(true);
    try {
      const msg = await HttpUtil.post('/panel/api/reseller/vouchers/redeem', { code }, JSON_HEADERS as never);
      if (msg?.success) {
        messageApi.success(t('resellers.voucherRedeemed', 'Voucher code redeemed successfully!'));
        setVoucherCode('');
        queryClient.invalidateQueries({ queryKey: keys.resellers.selfReport() });
      }
    } finally {
      setRedeeming(false);
    }
  }, [messageApi, queryClient, t, voucherCode]);

  useEffect(() => {
    setMessageInstance(messageApi);
  }, [messageApi]);

  const pageClass = useMemo(() => {
    const classes = ['resellers-page'];
    if (isDark) classes.push('is-dark');
    if (isUltra) classes.push('is-ultra');
    return classes.join(' ');
  }, [isDark, isUltra]);

  const profileQuery = useQuery({
    queryKey: [...keys.resellers.selfReport(), 'profile'],
    queryFn: async (): Promise<ResellerStat> => {
      const msg = await HttpUtil.get<ResellerStat>('/panel/api/reseller/profile');
      if (!msg.success) throw new Error(msg.msg || 'failed');
      return msg.obj as ResellerStat;
    },
  });

  const stat = profileQuery.data;
  useEffect(() => {
    if (!stat) return;
    brandingForm.setFieldsValue({
      brandName: stat.reseller.brandName || '',
      brandLogo: stat.reseller.brandLogo || '',
      brandColor: stat.reseller.brandColor || '',
    });
  }, [brandingForm, stat]);

  const signupTemplatesQuery = useQuery({
    queryKey: [...keys.resellers.selfReport(), 'signup-templates'],
    queryFn: async (): Promise<Array<{ id: number; name: string; totalGB: number; expireDurationDays: number }>> => {
      const response = await HttpUtil.get('/panel/api/reseller/signup/templates');
      if (!response.success) throw new Error(response.msg || 'failed');
      return (response.obj || []) as Array<{ id: number; name: string; totalGB: number; expireDurationDays: number }>;
    },
  });
  const trafficPercent = stat?.reseller.trafficLimit
    ? Math.min(100, Math.round((stat.allocatedTraffic / stat.reseller.trafficLimit) * 100))
    : 0;

  const notificationsQuery = useQuery({
    queryKey: keys.resellers.notifications(),
    queryFn: async (): Promise<ResellerNotificationFeed> => {
      const msg = await HttpUtil.get<ResellerNotificationFeed>('/panel/api/reseller/notifications');
      if (!msg.success) throw new Error(msg.msg || 'failed');
      return msg.obj as ResellerNotificationFeed;
    },
  });
  const notifications = notificationsQuery.data?.list ?? [];
  const unreadCount = notificationsQuery.data?.unreadCount ?? 0;

  const markAllNotificationsRead = useCallback(async () => {
    const msg = await HttpUtil.post('/panel/api/reseller/notifications/read', {}, JSON_HEADERS as never);
    if (msg?.success) {
      queryClient.invalidateQueries({ queryKey: keys.resellers.notifications() });
    }
  }, [queryClient]);

  const saveBranding = useCallback(async () => {
    const values = await brandingForm.validateFields().catch(() => null);
    if (!values) return;
    setBrandingSaving(true);
    try {
      const response = await HttpUtil.post('/panel/api/reseller/branding', values, { ...JSON_HEADERS, silent: true } as never);
      if (!response.success || !response.obj) {
        messageApi.error(response.msg || t('somethingWentWrong'));
        return;
      }
      queryClient.setQueryData([...keys.resellers.selfReport(), 'profile'], response.obj as ResellerStat);
      queryClient.invalidateQueries({ queryKey: keys.session.me() });
      messageApi.success(t('resellers.branding.saved'));
    } finally {
      setBrandingSaving(false);
    }
  }, [brandingForm, messageApi, queryClient, t]);

  const generateTgLink = useCallback(async () => {
    setTgLinking(true);
    try {
      const msg = await HttpUtil.post('/panel/api/reseller/telegram/link', {}, JSON_HEADERS as never);
      if (msg?.success) {
        setTgLink(msg.obj as TelegramLinkInfo);
      } else {
        messageApi.error(msg?.msg || t('somethingWentWrong'));
      }
    } finally {
      setTgLinking(false);
    }
  }, [messageApi, t]);

  const unlinkTelegram = useCallback(async () => {
    setTgUnlinking(true);
    try {
      const msg = await HttpUtil.post('/panel/api/reseller/telegram/unlink', {}, JSON_HEADERS as never);
      if (msg?.success) {
        setTgLink(null);
        messageApi.success(t('resellers.telegram.unlinked', 'Telegram disconnected.'));
        queryClient.invalidateQueries({ queryKey: [...keys.resellers.selfReport(), 'profile'] });
      } else {
        messageApi.error(msg?.msg || t('somethingWentWrong'));
      }
    } finally {
      setTgUnlinking(false);
    }
  }, [messageApi, queryClient, t]);

  const copyTgCode = useCallback(async () => {
    if (!tgLink?.code) return;
    await navigator.clipboard.writeText(`/start reseller_${tgLink.code}`);
    messageApi.success(t('resellers.telegram.copied', 'Copied to clipboard'));
  }, [messageApi, t, tgLink]);

  const createSignupLink = useCallback(async () => {
    if (!signupTemplateId) return;
    setSignupLinkLoading(true);
    try {
      const response = await HttpUtil.post(
        '/panel/api/reseller/signup/link',
        { templateId: signupTemplateId },
        { headers: { 'Content-Type': 'application/json' }, silent: true },
      );
      const token = (response.obj as { token?: string } | null)?.token;
      if (!response.success || !token) {
        messageApi.error(response.msg || t('somethingWentWrong'));
        return;
      }
      const base = (window.X_UI_BASE_PATH || '/').replace(/\/+$/, '');
      setSignupLink(`${window.location.origin}${base}/panel/reseller-signup?ref=${encodeURIComponent(token)}`);
    } finally {
      setSignupLinkLoading(false);
    }
  }, [messageApi, signupTemplateId, t]);

  const copySignupLink = useCallback(async () => {
    if (!signupLink) return;
    await navigator.clipboard.writeText(signupLink);
    messageApi.success(t('resellers.signupLinkCopied', 'Registration link copied'));
  }, [messageApi, signupLink, t]);

  const createDepositLink = useCallback(async () => {
    setDepositLinkLoading(true);
    try {
      const response = await HttpUtil.post('/panel/api/reseller/deposit/link', {}, { silent: true } as never);
      const token = (response.obj as { token?: string } | null)?.token;
      if (!response.success || !token) {
        messageApi.error(response.msg || t('somethingWentWrong'));
        return;
      }
      const base = (window.X_UI_BASE_PATH || '/').replace(/\/+$/, '');
      setDepositLink(`${window.location.origin}${base}/panel/reseller-deposit?ref=${encodeURIComponent(token)}`);
    } finally {
      setDepositLinkLoading(false);
    }
  }, [messageApi, t]);

  const copyDepositLink = useCallback(async () => {
    if (!depositLink) return;
    await navigator.clipboard.writeText(depositLink);
    messageApi.success(t('resellers.depositLinkCopied', 'Wallet top-up link copied'));
  }, [depositLink, messageApi, t]);

  const changePassword = useCallback(async () => {
    const values = await passwordForm.validateFields().catch(() => null);
    if (!values) return;
    setSaving(true);
    try {
      const msg = await HttpUtil.post(
        '/panel/api/reseller/password',
        {
          oldPassword: values.oldPassword,
          newPassword: values.newPassword,
        },
        { ...JSON_HEADERS, silent: true } as never,
      );
      if (msg.success) {
        messageApi.success(t('resellers.toasts.passwordChanged'));
        passwordForm.resetFields();
        queryClient.invalidateQueries({ queryKey: keys.session.me() });
      } else {
        messageApi.error(msg.msg || t('somethingWentWrong'));
      }
    } finally {
      setSaving(false);
    }
  }, [messageApi, passwordForm, queryClient, t]);

  return (
    <ConfigProvider theme={antdThemeConfig}>
      {messageContextHolder}
      <Layout className={pageClass}>
        <AppSidebar />
        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            {profileQuery.isError ? (
              <Alert type="error" showIcon message={t('somethingWentWrong')} />
            ) : (
              <>
              <PageHeader
                eyebrow={t('menu.groupOverview', 'Overview')}
                title={t('menu.profile', 'Profile')}
                subtitle={stat ? <span><strong>{stat.reseller.name || stat.reseller.username}</strong></span> : undefined}
                badge={stat && (
                  <span className={`phoenix-pill ${stat.reseller.enable ? 'is-running' : 'is-error'}`}>
                    <span className="phoenix-dot" />
                    {stat.reseller.enable ? t('enable') : t('disabled')}
                  </span>
                )}
              />
              <Row gutter={[isMobile ? 8 : 16, isMobile ? 8 : 12]}>
                <Col xs={24} lg={16}>
                  <Card
                    size="small"
                    className="phoenix-rise phoenix-rise-2"
                    title={
                      <Space>
                        <ShopOutlined />
                        {t('resellers.profile')}
                      </Space>
                    }
                    extra={
                      stat && (
                        <Tag color={stat.reseller.enable ? 'green' : 'red'}>
                          {stat.reseller.enable ? t('enable') : t('disabled')}
                        </Tag>
                      )
                    }
                  >
                    <Descriptions size="small" bordered column={isMobile ? 1 : 2}>
                      <Descriptions.Item label={t('resellers.name')}>{stat?.reseller.name || '-'}</Descriptions.Item>
                      <Descriptions.Item label={t('username')}>{stat?.reseller.username || '-'}</Descriptions.Item>
                      <Descriptions.Item label={t('resellers.comment')}>{stat?.reseller.comment || '-'}</Descriptions.Item>
                      <Descriptions.Item label={t('resellers.expiry')}>
                        {stat?.reseller.expiryTime ? new Date(stat.reseller.expiryTime).toLocaleString() : t('resellers.never')}
                      </Descriptions.Item>
                    </Descriptions>
                  </Card>
                </Col>

                <Col span={24}>
                  <Card size="small" className="phoenix-rise phoenix-rise-2" title={t('resellers.branding.title')}>
                    <Typography.Paragraph type="secondary">
                      {t('resellers.branding.intro')}
                    </Typography.Paragraph>
                    <Form form={brandingForm} layout="vertical">
                      <Row gutter={12}>
                        <Col xs={24} md={8}>
                          <Form.Item name="brandName" label={t('resellers.branding.name')}>
                            <Input maxLength={80} />
                          </Form.Item>
                        </Col>
                        <Col xs={24} md={10}>
                          <Form.Item name="brandLogo" label={t('resellers.branding.logo')} help={t('resellers.branding.logoHint')}>
                            <Input maxLength={2048} />
                          </Form.Item>
                        </Col>
                        <Col xs={24} md={6}>
                          <Form.Item name="brandColor" label={t('resellers.branding.color')}>
                            <Input maxLength={7} placeholder="#1677FF" />
                          </Form.Item>
                        </Col>
                      </Row>
                      <Button type="primary" loading={brandingSaving} onClick={() => void saveBranding()}>
                        {t('resellers.branding.save')}
                      </Button>
                    </Form>
                  </Card>
                </Col>

                <Col xs={24} lg={8}>
                  <Card size="small" hoverable title={t('resellers.quotas')}>
                    <Space orientation="vertical" size={12} style={{ width: '100%' }}>
                      <div>
                        <Typography.Text type="secondary">{t('resellers.table.traffic')}</Typography.Text>
                        <Typography.Text style={{ float: 'right' }}>
                          {SizeFormatter.sizeFormat(stat?.allocatedTraffic || 0)}
                          {stat?.reseller.trafficLimit
                            ? ` / ${SizeFormatter.sizeFormat(stat.reseller.trafficLimit)}`
                            : ` / ${t('resellers.unlimited')}`}
                        </Typography.Text>
                        <Progress
                          percent={stat?.reseller.trafficLimit ? trafficPercent : 0}
                          showInfo={false}
                          status={stat?.overQuota ? 'exception' : 'normal'}
                        />
                      </div>
                      <Row gutter={8}>
                        <Col span={12}>
                          <Statistic
                            title={t('resellers.table.clients')}
                            value={
                              stat?.reseller.clientLimit
                                ? `${stat.clientCount} / ${stat.reseller.clientLimit}`
                                : `${stat?.clientCount ?? 0} / ${t('resellers.unlimited')}`
                            }
                          />
                        </Col>
                        <Col span={12}>
                          <Statistic title={t('resellers.table.inbounds')} value={stat?.inboundCount ?? 0} />
                        </Col>
                      </Row>
                      <Row gutter={8}>
                        <Col span={12}>
                          <Statistic title={t('resellers.online')} value={stat?.onlineCount || 0} />
                        </Col>
                        <Col span={12}>
                          <Statistic title={t('resellers.usedTraffic')} value={SizeFormatter.sizeFormat(stat?.usedTraffic || 0)} />
                        </Col>
                      </Row>
                    </Space>
                  </Card>
                </Col>

                <Col span={24}>
                  <Card
                    size="small"
                    hoverable
                    className="phoenix-rise phoenix-rise-2"
                    title={
                      <Space>
                        <Badge count={unreadCount} size="small" offset={[6, -2]}>
                          <BellOutlined />
                        </Badge>
                        {t('resellers.notifications')}
                      </Space>
                    }
                    extra={
                      unreadCount > 0 ? (
                        <Button size="small" type="link" icon={<CheckOutlined />} onClick={markAllNotificationsRead}>
                          {t('resellers.notif.markAllRead')}
                        </Button>
                      ) : undefined
                    }
                  >
                    {notifications.length === 0 ? (
                      <Typography.Text type="secondary">{t('resellers.notif.empty')}</Typography.Text>
                    ) : (
                      <List
                        size="small"
                        dataSource={notifications}
                        rowKey={(item) => String(item.id)}
                        renderItem={(item) => (
                          <List.Item style={{ opacity: item.isRead ? 0.55 : 1 }}>
                            <Space wrap size={12}>
                              <Tag color={NOTIF_TAG_COLOR[item.kind] || 'blue'}>
                                {t(`resellers.notif.kind.${notifKindGroup(item.kind)}`)}
                              </Tag>
                              <Typography.Text>{notifTitle(t, item.kind, item.params)}</Typography.Text>
                              <Typography.Text type="secondary">{new Date(item.createdAt).toLocaleString()}</Typography.Text>
                            </Space>
                          </List.Item>
                        )}
                      />
                    )}
                  </Card>
                </Col>

                <Col span={24}>
                  <Card
                    size="small"
                    hoverable
                    title={<Space><UserOutlined />{t('resellers.signup.title', 'Customer self-registration')}</Space>}
                  >
                    <Space direction="vertical" size={10} style={{ width: '100%' }}>
                      <Typography.Text type="secondary">
                        {t('resellers.signup.intro', 'Create a shareable link that registers a customer on one of your templates. Your client and traffic limits are checked at registration.')}
                      </Typography.Text>
                      {signupTemplatesQuery.data?.length ? (
                        <Select
                          value={signupTemplateId}
                          onChange={(value: number) => { setSignupTemplateId(value); setSignupLink(''); }}
                          placeholder={t('resellers.signup.choosePlan', 'Choose a plan template')}
                          style={{ width: '100%', maxWidth: 420 }}
                          options={signupTemplatesQuery.data.map((template) => ({
                            value: template.id,
                            label: `${template.name} · ${template.totalGB > 0 ? `${(template.totalGB / (1024 ** 3)).toFixed(2)} GB` : t('unlimited', 'Unlimited')}${template.expireDurationDays > 0 ? ` · ${template.expireDurationDays}d` : ''}`,
                          }))}
                        />
                      ) : (
                        <Alert
                          type="info"
                          showIcon
                          message={t('resellers.signup.noPlans', 'No eligible plan templates. Ask the administrator to create a template with inbounds assigned to your account.')}
                        />
                      )}
                      <Space wrap>
                        <Button type="primary" loading={signupLinkLoading} disabled={!signupTemplateId} onClick={createSignupLink}>
                          {t('resellers.signup.createLink', 'Generate registration link')}
                        </Button>
                        {signupLink ? <Button icon={<CopyOutlined />} onClick={copySignupLink}>{t('copy', 'Copy link')}</Button> : null}
                      </Space>
                      {signupLink ? <Input.TextArea value={signupLink} readOnly autoSize={{ minRows: 2, maxRows: 4 }} /> : null}
                    </Space>
                  </Card>
                </Col>

                <Col span={24}>
                  <Card
                    size="small"
                    hoverable
                    title={<Space><WalletOutlined />{t('resellers.deposit.title', 'Public wallet top-up')}</Space>}
                  >
                    <Space direction="vertical" size={10} style={{ width: '100%' }}>
                      <Typography.Text type="secondary">
                        {t('resellers.deposit.description', 'Share a payment link so a customer can add credit directly to your reseller wallet. Payment is confirmed by the configured webhook.')}
                      </Typography.Text>
                      <Space wrap>
                        <Button type="primary" icon={<WalletOutlined />} loading={depositLinkLoading} onClick={createDepositLink}>
                          {t('resellers.deposit.createLink', 'Generate wallet top-up link')}
                        </Button>
                        {depositLink ? <Button icon={<CopyOutlined />} onClick={copyDepositLink}>{t('copy', 'Copy link')}</Button> : null}
                      </Space>
                      {depositLink ? <Input.TextArea value={depositLink} readOnly autoSize={{ minRows: 2, maxRows: 4 }} /> : null}
                    </Space>
                  </Card>
                </Col>

                <Col span={24}>
                  <Card
                    size="small"
                    hoverable
                    className="phoenix-rise phoenix-rise-2"
                    title={
                      <Space>
                        <SendOutlined />
                        {t('resellers.telegram.title', 'Telegram Bot')}
                      </Space>
                    }
                    extra={
                      stat?.reseller.tgId ? (
                        <Tag color="green">{t('resellers.telegram.linked', 'Linked')}</Tag>
                      ) : undefined
                    }
                  >
                    {stat?.reseller.tgId ? (
                      <Space direction="vertical" size={8}>
                        <span style={{ color: 'var(--ant-color-text-secondary)' }}>
                          {t(
                            'resellers.telegram.linkedDesc',
                            'This reseller account is linked to your Telegram. Quota, client and expiry alerts are delivered straight to your chat.',
                          )}
                        </span>
                        <Button danger loading={tgUnlinking} onClick={unlinkTelegram}>
                          {t('resellers.telegram.unlink', 'Disconnect Telegram')}
                        </Button>
                      </Space>
                    ) : (
                      <Space direction="vertical" size={8}>
                        <span style={{ color: 'var(--ant-color-text-secondary)' }}>
                          {t(
                            'resellers.telegram.desc',
                            'Link your Telegram to receive quota, client and expiry alerts and check your usage right inside the bot.',
                          )}
                        </span>
                        {tgLink ? (
                          <>
                            <Space wrap>
                              <Typography.Text code style={{ fontSize: 15 }}>
                                /start reseller_{tgLink.code}
                              </Typography.Text>
                              <Button size="small" icon={<CopyOutlined />} onClick={copyTgCode}>
                                {t('resellers.telegram.copy', 'Copy command')}
                              </Button>
                              {tgLink.botUsername ? (
                                <Button
                                  size="small"
                                  type="primary"
                                  icon={<SendOutlined />}
                                  href={`https://t.me/${tgLink.botUsername}?start=reseller_${tgLink.code}`}
                                  target="_blank"
                                  rel="noreferrer"
                                >
                                  {t('resellers.telegram.openBot', 'Open in Telegram')}
                                </Button>
                              ) : undefined}
                            </Space>
                            <Typography.Text type="secondary">
                              {t('resellers.telegram.expires', 'Code expires at')}:{' '}
                              {new Date(tgLink.expiresAt).toLocaleTimeString()}
                            </Typography.Text>
                          </>
                        ) : undefined}
                        <Button type="primary" icon={<SendOutlined />} loading={tgLinking} onClick={generateTgLink}>
                          {tgLink
                            ? t('resellers.telegram.regenerate', 'Generate a new code')
                            : t('resellers.telegram.generate', 'Generate link code')}
                        </Button>
                      </Space>
                    )}
                  </Card>
                </Col>

                <Col span={24}>
                  <Card
                    size="small"
                    hoverable
                    title={
                      <Space>
                        <UserOutlined />
                        {t('resellers.changePassword')}
                      </Space>
                    }
                  >
                    <Form form={passwordForm} layout="vertical" style={{ maxWidth: 420 }}>
                      <Form.Item
                        name="oldPassword"
                        label={t('resellers.oldPassword')}
                        rules={[{ required: true, message: t('resellers.validation.password') }]}
                      >
                        <Input.Password autoComplete="current-password" />
                      </Form.Item>
                      <Form.Item
                        name="newPassword"
                        label={t('resellers.newPassword')}
                        rules={[{ required: true, min: 6, message: t('resellers.validation.newPassword') }]}
                      >
                        <Input.Password autoComplete="new-password" />
                      </Form.Item>
                      <Form.Item
                        name="confirmPassword"
                        label={t('resellers.confirmPassword')}
                        dependencies={['newPassword']}
                        rules={[
                          { required: true, message: t('resellers.validation.confirmPassword') },
                          ({ getFieldValue }) => ({
                            validator(_rule, value) {
                              if (!value || getFieldValue('newPassword') === value) return Promise.resolve();
                              return Promise.reject(new Error(t('resellers.validation.passwordMismatch')));
                            },
                          }),
                        ]}
                      >
                        <Input.Password autoComplete="new-password" />
                      </Form.Item>
                      <Button type="primary" loading={saving} onClick={changePassword}>
                        {t('save')}
                      </Button>
                    </Form>
                  </Card>
                </Col>

                <Col span={24}>
                  <Card
                    size="small"
                    hoverable
                    title={
                      <Space>
                        <GiftOutlined />
                        {t('resellers.redeemVoucherTitle', 'Redeem Gift / Voucher Code')}
                      </Space>
                    }
                  >
                    <Space wrap>
                      <Input
                        value={voucherCode}
                        onChange={(e) => setVoucherCode(e.target.value.toUpperCase())}
                        placeholder="PHOENIX-XXXX-XXXX"
                        style={{ width: 260 }}
                        onPressEnter={redeemVoucher}
                      />
                      <Button type="primary" icon={<GiftOutlined />} loading={redeeming} onClick={redeemVoucher}>
                        {t('resellers.redeemVoucherBtn', 'Redeem Code')}
                      </Button>
                    </Space>
                  </Card>
                </Col>

                <Col span={24}>
                  <Card
                    size="small"
                    hoverable
                    title={
                      <Space>
                        <CustomerServiceOutlined />
                        {t('resellers.supportTickets') || 'Support Tickets'}
                      </Space>
                    }
                  >
                    <Space direction="vertical" size={8}>
                      <span style={{ color: 'var(--ant-color-text-secondary)' }}>
                        {t('resellers.ticketsDesc') || 'Open a support ticket or view replies from the panel administrator.'}
                      </span>
                      <Button type="primary" icon={<CustomerServiceOutlined />} onClick={() => setTicketsOpen(true)}>
                        {t('resellers.supportTickets') || 'Support Tickets'}
                      </Button>
                    </Space>
                  </Card>
                </Col>
              </Row>
              <SupportTicketsModal
                open={ticketsOpen}
                mode="reseller"
                onOpenChange={setTicketsOpen}
              />
              </>
            )}
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
