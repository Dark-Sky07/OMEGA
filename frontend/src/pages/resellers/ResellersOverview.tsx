import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Empty,
  Input,
  Progress,
  Row,
  Col,
  Select,
  Space,
  Statistic,
  Table,
  Tag,
  Tooltip,
  Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { AlertOutlined, ShopOutlined, TeamOutlined, WifiOutlined } from '@ant-design/icons';

import { SizeFormatter } from '@/utils';
import type { ResellerStat } from '@/api/queries/useSession';

type StatusFilter = 'all' | 'active' | 'disabled' | 'expired' | 'overQuota';

function formatDate(ts?: number): string {
  if (!ts) return '—';
  return new Date(ts).toLocaleDateString();
}

/** Coarse health bucket for one reseller, most severe first. */
function statusOf(stat: ResellerStat, t: (key: string) => string): { label: string; color: string } {
  if (!stat.reseller.enable) return { label: t('resellers.statusDisabled'), color: 'red' };
  if (stat.expired) return { label: t('resellers.expired'), color: 'orange' };
  if (stat.overQuota) return { label: t('resellers.overQuota'), color: 'orange' };
  return { label: t('resellers.statusActive'), color: 'green' };
}

interface ResellersOverviewProps {
  stats: ResellerStat[];
  loading?: boolean;
}

/**
 * Live, read-only overview of every reseller (نمایندگی) for the admin:
 * aggregate health numbers on top, a sortable/filterable per-reseller table
 * below. All data comes from the same /panel/api/resellers/list snapshot the
 * management table uses, so it refreshes with it (websocket + periodic
 * refetch) — no extra backend endpoint is needed.
 */
export default function ResellersOverview({ stats, loading }: ResellersOverviewProps) {
  const { t } = useTranslation();
  const [search, setSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all');

  const summary = useMemo(() => {
    const total = stats.length;
    const active = stats.filter((s) => s.reseller.enable && !s.expired && !s.overQuota).length;
    const disabled = stats.filter((s) => !s.reseller.enable).length;
    const inbounds = stats.reduce((sum, s) => sum + s.inboundCount, 0);
    const clients = stats.reduce((sum, s) => sum + s.clientCount, 0);
    const online = stats.reduce((sum, s) => sum + s.onlineCount, 0);
    const used = stats.reduce((sum, s) => sum + s.usedTraffic, 0);
    const allocated = stats.reduce((sum, s) => sum + s.allocatedTraffic, 0);
    const attention = stats.filter((s) => !s.reseller.enable || s.expired || s.overQuota).length;
    return { total, active, disabled, inbounds, clients, online, used, allocated, attention };
  }, [stats]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return stats.filter((stat) => {
      if (statusFilter === 'active' && (!stat.reseller.enable || stat.expired || stat.overQuota)) return false;
      if (statusFilter === 'disabled' && stat.reseller.enable) return false;
      if (statusFilter === 'expired' && !stat.expired) return false;
      if (statusFilter === 'overQuota' && !stat.overQuota) return false;
      if (!q) return true;
      const haystack = `${stat.reseller.name} ${stat.reseller.username} ${stat.reseller.comment ?? ''}`.toLowerCase();
      return haystack.includes(q);
    });
  }, [stats, search, statusFilter]);

  const trafficPercent = (stat: ResellerStat): number =>
    stat.reseller.trafficLimit ? Math.min(100, Math.round((stat.allocatedTraffic / stat.reseller.trafficLimit) * 100)) : 0;

  const columns: ColumnsType<ResellerStat> = [
    {
      title: t('resellers.table.reseller'),
      key: 'reseller',
      render: (_value, stat) => (
        <Space orientation="vertical" size={0}>
          <b>{stat.reseller.name || stat.reseller.username}</b>
          <Typography.Text type="secondary">{stat.reseller.username}</Typography.Text>
        </Space>
      ),
    },
    {
      title: t('resellers.table.status'),
      key: 'status',
      width: 130,
      render: (_value, stat) => {
        const st = statusOf(stat, t);
        return <Tag color={st.color}>{st.label}</Tag>;
      },
    },
    {
      title: t('resellers.table.inbounds'),
      key: 'inbounds',
      width: 100,
      sorter: (a, b) => a.inboundCount - b.inboundCount,
      render: (_value, stat) => stat.inboundCount,
    },
    {
      title: t('resellers.table.clients'),
      key: 'clients',
      width: 120,
      sorter: (a, b) => a.clientCount - b.clientCount,
      render: (_value, stat) =>
        stat.reseller.clientLimit ? `${stat.clientCount} / ${stat.reseller.clientLimit}` : stat.clientCount,
    },
    {
      title: t('resellers.online'),
      key: 'online',
      width: 100,
      sorter: (a, b) => a.onlineCount - b.onlineCount,
      render: (_value, stat) => (
        <Typography.Text type={stat.onlineCount > 0 ? 'success' : undefined}>
          {stat.onlineCount > 0 ? `● ${stat.onlineCount}` : stat.onlineCount}
        </Typography.Text>
      ),
    },
    {
      title: t('resellers.table.used'),
      key: 'used',
      width: 130,
      sorter: (a, b) => a.usedTraffic - b.usedTraffic,
      render: (_value, stat) => SizeFormatter.sizeFormat(stat.usedTraffic),
    },
    {
      title: t('resellers.table.traffic'),
      key: 'traffic',
      width: 230,
      sorter: (a, b) => {
        const ra = a.reseller.trafficLimit ? a.allocatedTraffic / a.reseller.trafficLimit : 0;
        const rb = b.reseller.trafficLimit ? b.allocatedTraffic / b.reseller.trafficLimit : 0;
        return ra - rb;
      },
      render: (_value, stat) => (
        <Space orientation="vertical" size={2} style={{ width: '100%' }}>
          <Typography.Text>
            {SizeFormatter.sizeFormat(stat.allocatedTraffic)}
            {stat.reseller.trafficLimit
              ? ` / ${SizeFormatter.sizeFormat(stat.reseller.trafficLimit)}`
              : ` / ${t('resellers.unlimited')}`}
          </Typography.Text>
          {stat.reseller.trafficLimit > 0 && (
            <Progress percent={trafficPercent(stat)} size="small" showInfo={false} status={stat.overQuota ? 'exception' : 'normal'} />
          )}
        </Space>
      ),
    },
    {
      title: t('resellers.table.remaining'),
      key: 'remaining',
      width: 130,
      sorter: (a, b) => {
        const ra = a.reseller.trafficLimit ? a.remainingTraffic : Number.MAX_SAFE_INTEGER;
        const rb = b.reseller.trafficLimit ? b.remainingTraffic : Number.MAX_SAFE_INTEGER;
        return ra - rb;
      },
      render: (_value, stat) =>
        stat.reseller.trafficLimit ? SizeFormatter.sizeFormat(Math.max(0, stat.remainingTraffic)) : '—',
    },
    {
      title: t('resellers.expiry'),
      key: 'expiry',
      width: 120,
      sorter: (a, b) =>
        (a.reseller.expiryTime || Number.MAX_SAFE_INTEGER) - (b.reseller.expiryTime || Number.MAX_SAFE_INTEGER),
      render: (_value, stat) =>
        stat.reseller.expiryTime ? (
          <Typography.Text type={stat.expired ? 'danger' : undefined}>
            {formatDate(stat.reseller.expiryTime)}
          </Typography.Text>
        ) : (
          <Typography.Text type="secondary">{t('resellers.never')}</Typography.Text>
        ),
    },
    {
      title: t('resellers.table.created'),
      key: 'created',
      width: 120,
      sorter: (a, b) => (a.reseller.createdAt ?? 0) - (b.reseller.createdAt ?? 0),
      render: (_value, stat) => formatDate(stat.reseller.createdAt),
    },
    {
      title: t('resellers.table.updated'),
      key: 'updated',
      width: 120,
      sorter: (a, b) => (a.reseller.updatedAt ?? 0) - (b.reseller.updatedAt ?? 0),
      render: (_value, stat) => formatDate(stat.reseller.updatedAt),
    },
  ];

  return (
    <Space orientation="vertical" size={12} style={{ width: '100%' }}>
      <Row gutter={[8, 12]}>
        <Col xs={12} md={6} xl={4}>
          <Statistic
            title={t('resellers.total')}
            value={summary.total}
            prefix={<ShopOutlined />}
            suffix={
              <Typography.Text type="secondary" style={{ fontSize: 13 }}>
                {summary.active} {t('resellers.statusActive')} / {summary.disabled} {t('resellers.statusDisabled')}
              </Typography.Text>
            }
          />
        </Col>
        <Col xs={12} md={6} xl={4}>
          <Statistic title={t('resellers.assignedInbounds')} value={summary.inbounds} />
        </Col>
        <Col xs={12} md={6} xl={4}>
          <Statistic title={t('resellers.totalClients')} value={summary.clients} prefix={<TeamOutlined />} />
        </Col>
        <Col xs={12} md={6} xl={4}>
          <Statistic
            title={t('resellers.onlineNow')}
            value={summary.online}
            prefix={<WifiOutlined />}
            valueStyle={summary.online > 0 ? { color: '#52c41a' } : undefined}
          />
        </Col>
        <Col xs={12} md={6} xl={4}>
          <Statistic title={t('resellers.totalUsedTraffic')} value={SizeFormatter.sizeFormat(summary.used)} />
        </Col>
        <Col xs={12} md={6} xl={4}>
          <Statistic title={t('resellers.totalAllocatedTraffic')} value={SizeFormatter.sizeFormat(summary.allocated)} />
        </Col>
        <Col xs={12} md={12} xl={4}>
          <Tooltip title={t('resellers.needsAttentionHint')}>
            <Statistic
              title={t('resellers.needsAttention')}
              value={summary.attention}
              prefix={<AlertOutlined />}
              valueStyle={summary.attention > 0 ? { color: '#fa8c16' } : undefined}
            />
          </Tooltip>
        </Col>
      </Row>

      <Space style={{ width: '100%', justifyContent: 'space-between' }} wrap>
        <Input
          allowClear
          placeholder={t('resellers.searchPlaceholder')}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          style={{ maxWidth: 320 }}
        />
        <Select<StatusFilter>
          value={statusFilter}
          onChange={setStatusFilter}
          style={{ width: 180 }}
          options={[
            { value: 'all', label: t('resellers.allStatuses') },
            { value: 'active', label: t('resellers.statusActive') },
            { value: 'disabled', label: t('resellers.statusDisabled') },
            { value: 'expired', label: t('resellers.expired') },
            { value: 'overQuota', label: t('resellers.overQuota') },
          ]}
        />
      </Space>

      <Table<ResellerStat>
        rowKey={(stat) => String(stat.reseller.id)}
        size="small"
        loading={loading}
        columns={columns}
        dataSource={filtered}
        pagination={filtered.length > 20 ? { pageSize: 20 } : false}
        scroll={{ x: 'max-content' }}
        locale={{ emptyText: <Empty description={t('resellers.noDataForFilter')} /> }}
      />
    </Space>
  );
}
