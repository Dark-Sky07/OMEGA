import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import {
  Alert,
  Button,
  Card,
  ConfigProvider,
  Empty,
  Input,
  Layout,
  Select,
  Space,
  Table,
  Tag,
  Typography,
  message,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { CheckCircleOutlined, ClockCircleOutlined, CopyOutlined, ExclamationCircleOutlined } from '@ant-design/icons';
import { PageHeader, StatStrip, type StatItem } from '@/components/ui';

import AppSidebar from '@/layouts/AppSidebar';
import { ClipboardManager, HttpUtil, SizeFormatter } from '@/utils';
import { setMessageInstance } from '@/utils/messageBus';
import { useAllSettings } from '@/api/queries/useAllSettings';
import { keys } from '@/api/queryKeys';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { useTheme } from '@/hooks/useTheme';
import type { ResellerClientRow, ResellerReport } from '@/api/queries/useSession';
import './ResellersPage.css';

type RenewalFilter = 'all' | 'expired' | 'upcoming' | 'later' | 'noExpiry';
const EMPTY_CLIENTS: ResellerClientRow[] = [];

function clientState(row: ResellerClientRow, now: number, cutoff: number, t: (key: string) => string) {
  if (row.expiryTime > 0 && row.expiryTime <= now) return { label: t('resellers.renewalExpiredLabel'), color: 'red' };
  if (row.expiryTime < 0) return { label: t('resellers.renewalPendingActivation'), color: 'blue' };
  if (row.expiryTime === 0) return { label: t(row.enable ? 'resellers.renewalNoExpiry' : 'resellers.renewalDisabled'), color: 'default' };
  if (!row.enable) return { label: t('resellers.renewalDisabled'), color: 'default' };
  if (row.expiryTime <= cutoff) return { label: t('resellers.renewalUpcomingLabel'), color: 'orange' };
  return { label: t('resellers.renewalActive'), color: 'green' };
}

function matchesFilter(row: ResellerClientRow, filter: RenewalFilter, now: number, cutoff: number): boolean {
  if (filter === 'expired') return row.expiryTime > 0 && row.expiryTime <= now;
  if (filter === 'upcoming') return row.expiryTime > now && row.expiryTime <= cutoff;
  if (filter === 'later') return row.expiryTime > cutoff;
  if (filter === 'noExpiry') return row.expiryTime <= 0;
  return true;
}

export default function ResellerRenewalsPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const { isMobile } = useMediaQuery();
  const { allSetting } = useAllSettings();
  const [messageApi, messageContextHolder] = message.useMessage();
  const [search, setSearch] = useState('');
  const [filter, setFilter] = useState<RenewalFilter>('all');
  const [windowDays, setWindowDays] = useState(30);
  const [copyingEmail, setCopyingEmail] = useState<string | null>(null);
  const now = Date.now();
  const cutoff = now + windowDays * 24 * 60 * 60 * 1000;

  useEffect(() => {
    setMessageInstance(messageApi);
  }, [messageApi]);

  const reportQuery = useQuery({
    queryKey: keys.resellers.selfReport(),
    queryFn: async (): Promise<ResellerReport> => {
      const msg = await HttpUtil.get<ResellerReport>('/panel/api/reseller/report');
      if (!msg.success) throw new Error(msg.msg || 'failed');
      return msg.obj as ResellerReport;
    },
    refetchInterval: 60_000,
  });

  const clients = reportQuery.data?.clients ?? EMPTY_CLIENTS;
  const upcomingCount = clients.filter((row) => row.expiryTime > now && row.expiryTime <= cutoff).length;
  const expiredCount = clients.filter((row) => row.expiryTime > 0 && row.expiryTime <= now).length;

  const filtered = useMemo(() => {
    const query = search.trim().toLowerCase();
    return clients.filter((row) => {
      if (!matchesFilter(row, filter, now, cutoff)) return false;
      if (!query) return true;
      return `${row.email} ${row.comment || ''} ${row.group || ''}`.toLowerCase().includes(query);
    });
  }, [clients, filter, now, cutoff, search]);

  const copyRenewalLink = async (email: string) => {
    setCopyingEmail(email);
    try {
      const msg = await HttpUtil.get<{ token?: string }>(
        `/panel/api/clients/checkout-link/${encodeURIComponent(email)}`,
        undefined,
        { silent: true },
      );
      const token = msg?.success ? msg.obj?.token : undefined;
      if (!token) {
        messageApi.error((msg?.msg as string) || t('somethingWentWrong'));
        return;
      }
      const basePath = (typeof window !== 'undefined' && window.X_UI_BASE_PATH) || '';
      const origin = typeof window !== 'undefined' ? window.location.origin : '';
      const link = `${origin}${basePath}/panel/checkout?ref=${encodeURIComponent(token)}`;
      const copied = await ClipboardManager.copyText(link);
      if (copied) messageApi.success(t('resellers.renewalLinkCopied'));
      else messageApi.error(t('somethingWentWrong'));
    } catch {
      messageApi.error(t('somethingWentWrong'));
    } finally {
      setCopyingEmail(null);
    }
  };

  const columns: ColumnsType<ResellerClientRow> = [
    {
      title: t('resellers.renewalClient'),
      key: 'client',
      sorter: (a, b) => a.email.localeCompare(b.email),
      render: (_value, row) => (
        <Space orientation="vertical" size={0}>
          <Typography.Text strong>{row.email}</Typography.Text>
          {row.comment && <Typography.Text type="secondary">{row.comment}</Typography.Text>}
        </Space>
      ),
    },
    {
      title: t('resellers.renewalExpiry'),
      dataIndex: 'expiryTime',
      key: 'expiryTime',
      width: 190,
      sorter: (a, b) => (a.expiryTime || Number.MAX_SAFE_INTEGER) - (b.expiryTime || Number.MAX_SAFE_INTEGER),
      render: (expiryTime: number) => (
        expiryTime > 0
          ? new Date(expiryTime).toLocaleString()
          : <Typography.Text type="secondary">{expiryTime < 0 ? t('resellers.renewalPendingActivation') : t('resellers.renewalNoExpiry')}</Typography.Text>
      ),
    },
    {
      title: t('resellers.renewalStatus'),
      key: 'status',
      width: 160,
      render: (_value, row) => {
        const state = clientState(row, now, cutoff, t);
        return <Tag color={state.color}>{state.label}</Tag>;
      },
    },
    {
      title: t('resellers.table.quota'),
      key: 'quota',
      width: 120,
      render: (_value, row) => row.totalBytes > 0 ? SizeFormatter.sizeFormat(row.totalBytes) : t('resellers.unlimited'),
    },
    {
      title: t('resellers.table.actions'),
      key: 'actions',
      width: 150,
      render: (_value, row) => (
        <Button
          size="small"
          icon={<CopyOutlined />}
          loading={copyingEmail === row.email}
          disabled={!allSetting.checkoutEnable}
          onClick={() => void copyRenewalLink(row.email)}
        >
          {t('resellers.renewalCopyLink')}
        </Button>
      ),
    },
  ];

  const pageClass = useMemo(() => {
    const classes = ['resellers-page'];
    if (isDark) classes.push('is-dark');
    if (isUltra) classes.push('is-ultra');
    return classes.join(' ');
  }, [isDark, isUltra]);

  const statItems: StatItem[] = [
    { key: 'upcoming', icon: <ClockCircleOutlined />, tone: upcomingCount > 0 ? 'warning' : 'muted', label: t('resellers.renewalUpcoming', { days: windowDays }), value: upcomingCount },
    { key: 'expired', icon: <ExclamationCircleOutlined />, tone: expiredCount > 0 ? 'danger' : 'muted', label: t('resellers.renewalExpired'), value: expiredCount },
    { key: 'other', icon: <CheckCircleOutlined />, tone: 'success', label: t('resellers.renewalCustomers'), value: clients.length },
  ];

  return (
    <ConfigProvider theme={antdThemeConfig}>
      {messageContextHolder}
      <Layout className={pageClass}>
        <AppSidebar />
        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            {reportQuery.isError ? (
              <Alert type="error" showIcon title={t('somethingWentWrong')} />
            ) : (
              <>
                <PageHeader
                  eyebrow={t('menu.groupManage', 'Manage')}
                  title={t('resellers.renewalCenter')}
                  subtitle={t('resellers.renewalCenterDesc')}
                />
                <StatStrip size={isMobile ? 'compact' : 'default'} items={statItems} />
                {!allSetting.checkoutEnable && (
                  <Alert
                    showIcon
                    type="info"
                    style={{ marginBottom: 16 }}
                    title={t('resellers.renewalCheckoutUnavailable')}
                  />
                )}
                <Card className="phoenix-table-card">
                  <div className="phoenix-toolbar">
                    <Input
                      allowClear
                      value={search}
                      onChange={(event) => setSearch(event.target.value)}
                      placeholder={t('resellers.renewalSearch')}
                      style={{ maxWidth: 320 }}
                    />
                    <Select<RenewalFilter>
                      value={filter}
                      onChange={setFilter}
                      style={{ width: 180 }}
                      options={[
                        { value: 'all', label: t('resellers.renewalAll') },
                        { value: 'expired', label: t('resellers.renewalExpired') },
                        { value: 'upcoming', label: t('resellers.renewalUpcoming', { days: windowDays }) },
                        { value: 'later', label: t('resellers.renewalLater') },
                        { value: 'noExpiry', label: t('resellers.renewalNoExpiry') },
                      ]}
                    />
                    <Select<number>
                      value={windowDays}
                      onChange={setWindowDays}
                      style={{ width: 150 }}
                      aria-label={t('resellers.renewalWindow')}
                      options={[7, 14, 30, 60].map((days) => ({ value: days, label: t('resellers.renewalDays', { days }) }))}
                    />
                    <span className="phoenix-toolbar-spacer" />
                  </div>
                  <Table<ResellerClientRow>
                    rowKey="email"
                    size="middle"
                    loading={reportQuery.isLoading}
                    columns={columns}
                    dataSource={filtered}
                    pagination={filtered.length > 20 ? { pageSize: 20 } : false}
                    scroll={{ x: 'max-content' }}
                    locale={{ emptyText: <Empty description={t('resellers.renewalNoClients')} /> }}
                  />
                </Card>
              </>
            )}
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
