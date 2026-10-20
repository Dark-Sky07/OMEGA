import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useSearchParams } from 'react-router-dom';
import {
  Alert,
  Button,
  Card,
  Col,
  Descriptions,
  InputNumber,
  Progress,
  Result,
  Row,
  Space,
  Spin,
  Tag,
  Typography,
} from 'antd';
import {
  CheckCircleOutlined,
  LinkOutlined,
  PlusSquareOutlined,
  ShoppingOutlined,
} from '@ant-design/icons';

import { HttpUtil, IntlUtil } from '@/utils';
import { useTheme } from '@/hooks/useTheme';

interface CheckoutInfo {
  email: string;
  enabled: boolean;
  expiryTime: number;
  totalBytes: number;
  up: number;
  down: number;
  currency: string;
  pricePerGb: number;
  pricePerMonth: number;
}

interface CheckoutOrderView {
  orderId: string;
  amount: number;
  currency: string;
  gatewayUrl?: string;
  payInfo?: string;
}

const POLL_INTERVAL_MS = 4000;

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value >= 100 ? Math.round(value) : value.toFixed(2)} ${units[unit]}`;
}

function formatAmount(value: number): string {
  return Number.isInteger(value) ? String(value) : value.toFixed(2);
}

/** Public client checkout page — opened from a signed per-client link. */
export default function CheckoutPage() {
  const { t } = useTranslation();
  const { isDark } = useTheme();
  const queryClient = useQueryClient();
  const [searchParams] = useSearchParams();
  const ref = (searchParams.get('ref') || '').trim();

  const [gb, setGb] = useState<number>(0);
  const [months, setMonths] = useState<number>(0);
  const [order, setOrder] = useState<CheckoutOrderView | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const infoQuery = useQuery({
    queryKey: ['checkout', 'info', ref],
    queryFn: async (): Promise<CheckoutInfo> => {
      const msg = await HttpUtil.get('/api/checkout/info', { ref }, { silent: true });
      if (!msg?.success || !msg.obj) throw new Error(msg?.msg || 'checkout unavailable');
      return msg.obj as CheckoutInfo;
    },
    enabled: !!ref,
    retry: false,
    staleTime: 10_000,
  });

  const statusQuery = useQuery({
    queryKey: ['checkout', 'status', ref, order?.orderId],
    queryFn: async (): Promise<string> => {
      const msg = await HttpUtil.get(
        '/api/checkout/status',
        { ref, orderId: order?.orderId },
        { silent: true },
      );
      if (!msg?.success) throw new Error(msg?.msg || 'status failed');
      return String(msg.obj ?? 'unknown');
    },
    enabled: !!ref && !!order?.orderId,
    refetchInterval: (query) => (query.state.data === 'completed' ? false : POLL_INTERVAL_MS),
    retry: false,
  });

  const info = infoQuery.data;
  const paid = statusQuery.data === 'completed';

  const usedBytes = useMemo(() => (info ? info.up + info.down : 0), [info]);
  const totalLabel = info ? (info.totalBytes > 0 ? formatBytes(info.totalBytes) : t('unlimited')) : '';
  const usedPct = useMemo(() => {
    if (!info || info.totalBytes <= 0) return 0;
    return Math.min(100, Math.round((usedBytes / info.totalBytes) * 100));
  }, [info, usedBytes]);

  const total = useMemo(() => {
    if (!info) return 0;
    return Math.round((gb * info.pricePerGb + months * info.pricePerMonth) * 100) / 100;
  }, [info, gb, months]);

  const currency = info?.currency || '';

  const expiryLabel = useMemo(() => {
    if (!info) return '';
    if (info.expiryTime <= 0) return t('indefinite');
    return IntlUtil.formatDate(info.expiryTime, 'gregorian');
  }, [info, t]);

  async function placeOrder() {
    setSubmitting(true);
    try {
      const msg = await HttpUtil.post(
        '/api/checkout/order',
        { gb, months },
        { headers: { 'Content-Type': 'application/json' }, silent: true },
      );
      if (!msg?.success || !msg.obj) return;
      const created = msg.obj as CheckoutOrderView;
      setOrder(created);
      if (created.gatewayUrl) {
        window.open(created.gatewayUrl, '_blank', 'noopener');
      }
    } finally {
      setSubmitting(false);
    }
  }

  const shellStyle: React.CSSProperties = {
    minHeight: '100vh',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    padding: '24px 12px',
    background: isDark ? '#0b0f1a' : '#f3f5f9',
  };

  if (!ref) {
    return (
      <div style={shellStyle}>
        <Card style={{ maxWidth: 460, width: '100%' }}>
          <Result
            status="warning"
            title={t('pages.checkout.invalidLink')}
            subTitle={t('pages.checkout.invalidLinkDesc')}
          />
        </Card>
      </div>
    );
  }

  if (infoQuery.isLoading) {
    return (
      <div style={shellStyle}>
        <Spin size="large" />
      </div>
    );
  }

  if (infoQuery.isError || !info) {
    const serverMsg = (infoQuery.error as Error | null)?.message || '';
    return (
      <div style={shellStyle}>
        <Card style={{ maxWidth: 460, width: '100%' }}>
          <Result
            status="403"
            title={t('pages.checkout.invalidLink')}
            subTitle={serverMsg || t('pages.checkout.invalidLinkDesc')}
          />
        </Card>
      </div>
    );
  }

  return (
    <div style={shellStyle}>
      <Card
        style={{ maxWidth: 560, width: '100%' }}
        title={
          <Space>
            <ShoppingOutlined />
            <span>{t('pages.checkout.title')}</span>
          </Space>
        }
      >
        {paid ? (
          <Result
            status="success"
            icon={<CheckCircleOutlined />}
            title={t('pages.checkout.paidTitle')}
            subTitle={t('pages.checkout.paidDesc')}
            extra={
              <Button
                type="primary"
                onClick={() => {
                  setOrder(null);
                  queryClient.invalidateQueries({ queryKey: ['checkout', 'info', ref] });
                }}
              >
                {t('pages.checkout.orderAgain')}
              </Button>
            }
          />
        ) : order ? (
          <Space direction="vertical" size={16} style={{ width: '100%' }}>
            <Alert
              type="info"
              showIcon
              message={t('pages.checkout.waitingPayment')}
              description={
                <Space direction="vertical" size={4}>
                  <Typography.Text>
                    {t('pages.checkout.orderId')}: <Typography.Text copyable code>{order.orderId}</Typography.Text>
                  </Typography.Text>
                  <Typography.Text strong>
                    {t('pages.checkout.amount')}: {formatAmount(order.amount)} {order.currency}
                  </Typography.Text>
                </Space>
              }
            />
            {order.gatewayUrl ? (
              <Button
                type="primary"
                icon={<LinkOutlined />}
                href={order.gatewayUrl}
                target="_blank"
                rel="noopener noreferrer"
                block
              >
                {t('pages.checkout.openGateway')}
              </Button>
            ) : order.payInfo ? (
              <Card size="small" title={t('pages.checkout.payInfoTitle')}>
                <Typography.Paragraph style={{ whiteSpace: 'pre-wrap', marginBottom: 0 }}>
                  {order.payInfo}
                </Typography.Paragraph>
              </Card>
            ) : null}
            <Space style={{ width: '100%', justifyContent: 'center' }}>
              <Spin size="small" />
              <Typography.Text type="secondary">{t('pages.checkout.waitingHint')}</Typography.Text>
            </Space>
          </Space>
        ) : (
          <Space direction="vertical" size={20} style={{ width: '100%' }}>
            <Descriptions size="small" column={1} bordered>
              <Descriptions.Item label={t('pages.checkout.client')}>{info.email}</Descriptions.Item>
              <Descriptions.Item label={t('pages.checkout.status')}>
                {info.enabled ? (
                  <Tag color="green">{t('enabled')}</Tag>
                ) : (
                  <Tag color="red">{t('disabled')}</Tag>
                )}
              </Descriptions.Item>
              <Descriptions.Item label={t('pages.checkout.expiry')}>{expiryLabel}</Descriptions.Item>
              <Descriptions.Item label={t('pages.checkout.traffic')}>
                <Space direction="vertical" size={2} style={{ width: '100%' }}>
                  <span>
                    {formatBytes(usedBytes)} / {totalLabel}
                  </span>
                  {info.totalBytes > 0 && <Progress percent={usedPct} showInfo={false} size="small" />}
                </Space>
              </Descriptions.Item>
            </Descriptions>

            <Row gutter={12}>
              <Col span={12}>
                <Typography.Text strong>{t('pages.checkout.addTraffic')}</Typography.Text>
                <InputNumber
                  min={0}
                  max={10000}
                  value={gb}
                  onChange={(v) => setGb(Math.max(0, Math.floor(Number(v) || 0)))}
                  addonAfter="GB"
                  style={{ width: '100%', marginTop: 6 }}
                  placeholder="0"
                />
                {info.pricePerGb > 0 && (
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                    {formatAmount(info.pricePerGb)} {currency} / GB
                  </Typography.Text>
                )}
              </Col>
              <Col span={12}>
                <Typography.Text strong>{t('pages.checkout.addMonths')}</Typography.Text>
                <InputNumber
                  min={0}
                  max={36}
                  value={months}
                  onChange={(v) => setMonths(Math.max(0, Math.floor(Number(v) || 0)))}
                  addonAfter={t('pages.checkout.months')}
                  style={{ width: '100%', marginTop: 6 }}
                  placeholder="0"
                />
                {info.pricePerMonth > 0 && (
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                    {formatAmount(info.pricePerMonth)} {currency} / {t('pages.checkout.month')}
                  </Typography.Text>
                )}
              </Col>
            </Row>

            <Button
              type="primary"
              size="large"
              icon={<PlusSquareOutlined />}
              block
              loading={submitting}
              disabled={total <= 0}
              onClick={placeOrder}
            >
              {t('pages.checkout.pay')} — {formatAmount(total)} {currency}
            </Button>
          </Space>
        )}
      </Card>
    </div>
  );
}
