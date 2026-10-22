import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Card, ConfigProvider, InputNumber, Result, Space, Spin, Typography, message } from 'antd';
import { CheckCircleOutlined, WalletOutlined } from '@ant-design/icons';

import { HttpUtil, currencyFractionDigits, formatMoney, roundCurrencyAmount } from '@/utils';
import { useTheme } from '@/hooks/useTheme';

interface DepositInfo {
  resellerName: string;
  brandName?: string;
  brandLogo?: string;
  brandColor?: string;
  currency: string;
}
interface DepositOrder {
  orderId: string;
  amount: number;
  currency: string;
  gatewayUrl?: string;
  payInfo?: string;
}

const POLL_INTERVAL_MS = 4000;

/** Public, signed reseller wallet deposit page. */
export default function ResellerDepositPage() {
  const { t } = useTranslation();
  const { isDark } = useTheme();
  const [params] = useSearchParams();
  const ref = (params.get('ref') || '').trim();
  const [amount, setAmount] = useState<number | null>(null);
  const [order, setOrder] = useState<DepositOrder | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');
  const [messageApi, holder] = message.useMessage();

  const infoQuery = useQuery({
    queryKey: ['reseller-deposit', 'info', ref],
    queryFn: async (): Promise<DepositInfo> => {
      const response = await HttpUtil.get('/api/reseller-deposit/info', { ref }, { silent: true });
      if (!response.success || !response.obj) throw new Error(response.msg || 'Deposit checkout is unavailable.');
      return response.obj as DepositInfo;
    },
    enabled: Boolean(ref),
    retry: false,
  });
  const statusQuery = useQuery({
    queryKey: ['reseller-deposit', 'status', ref, order?.orderId],
    queryFn: async (): Promise<string> => {
      const response = await HttpUtil.get('/api/reseller-deposit/status', { ref, orderId: order?.orderId }, { silent: true });
      if (!response.success) throw new Error(response.msg || 'Unable to check payment status.');
      return String(response.obj || 'unknown');
    },
    enabled: Boolean(ref && order?.orderId),
    refetchInterval: (query) => query.state.data === 'completed' ? false : POLL_INTERVAL_MS,
    retry: false,
  });

  const currency = infoQuery.data?.currency || 'USD';
  const currencyDigits = currencyFractionDigits(currency);
  const minimumAmount = 10 ** -currencyDigits;

  async function submit() {
    if (!amount || amount <= 0) return;
    setSubmitting(true);
    setError('');
    try {
      const response = await HttpUtil.post(
        '/api/reseller-deposit/order',
        { amount },
        { params: { ref }, headers: { 'Content-Type': 'application/json' }, silent: true },
      );
      if (!response.success || !response.obj) {
        setError(response.msg || 'Could not create the payment order.');
        return;
      }
      const created = response.obj as DepositOrder;
      setOrder(created);
      if (created.gatewayUrl) window.open(created.gatewayUrl, '_blank', 'noopener');
      messageApi.success(t('pages.checkout.resellerDeposit.orderCreated', 'Payment order created.'));
    } finally {
      setSubmitting(false);
    }
  }

  const info = infoQuery.data;
  const completed = statusQuery.data === 'completed';
  return (
    <ConfigProvider theme={info?.brandColor ? { token: { colorPrimary: info.brandColor } } : undefined}>
      <main style={{ minHeight: '100vh', display: 'grid', placeItems: 'center', padding: 16, background: isDark ? '#10131a' : '#f5f7fa' }}>
      {holder}
      <Card style={{ width: 'min(100%, 460px)' }}>
        {infoQuery.isLoading ? <Spin /> : null}
        {(!ref || infoQuery.isError) ? (
          <Result status="error" title={t('pages.checkout.resellerDeposit.unavailable', 'Deposit page unavailable')} subTitle={t('pages.checkout.resellerDeposit.invalidDesc', 'This link is invalid, expired, or public checkout is disabled.')} />
        ) : null}
        {info && completed ? (
          <Result status="success" icon={<CheckCircleOutlined />} title={t('pages.checkout.resellerDeposit.completed', 'Deposit received')} subTitle={t('pages.checkout.resellerDeposit.completedDesc', 'The reseller wallet has been credited. You can close this page.')} />
        ) : null}
        {info && !completed ? (
          <Space direction="vertical" size="large" style={{ width: '100%' }}>
            <div style={{ textAlign: 'center' }}>
              {info.brandLogo ? <img src={info.brandLogo} alt={info.brandName || info.resellerName} style={{ maxWidth: 160, maxHeight: 64, objectFit: 'contain' }} /> : <WalletOutlined style={{ fontSize: 30, color: info.brandColor || '#1677ff' }} />}
              <Typography.Title level={3} style={{ margin: '10px 0 4px', color: info.brandColor || undefined }}>{info.brandName || info.resellerName}</Typography.Title>
              <Typography.Text type="secondary">{t('pages.checkout.resellerDeposit.title', 'Add credit to reseller wallet')}</Typography.Text>
            </div>
            {order ? (
              <Card size="small" type="inner" title={t('pages.checkout.resellerDeposit.order', 'Payment order')}>
                <Space direction="vertical" style={{ width: '100%' }}>
                  <Typography.Text>{t('pages.checkout.resellerDeposit.orderId', 'Order')}: <Typography.Text code copyable>{order.orderId}</Typography.Text></Typography.Text>
                  <Typography.Text strong>{formatMoney(order.amount, order.currency)}</Typography.Text>
                  <Typography.Text>{t('pages.checkout.resellerDeposit.status', 'Status')}: {statusQuery.data || t('pages.checkout.resellerDeposit.pending', 'pending')}</Typography.Text>
                  {order.payInfo ? <Alert type="info" showIcon title={t('pages.checkout.resellerDeposit.instructions', 'Payment instructions')} description={order.payInfo} /> : null}
                  {order.gatewayUrl ? <Button type="primary" href={order.gatewayUrl} target="_blank" rel="noreferrer">{t('pages.checkout.resellerDeposit.openGateway', 'Open payment page')}</Button> : null}
                </Space>
              </Card>
            ) : (
              <>
                <Typography.Text>{t('pages.checkout.resellerDeposit.amount', 'Deposit amount')} ({currency})</Typography.Text>
                <InputNumber
                  min={minimumAmount}
                  max={1000000}
                  step={minimumAmount}
                  precision={currencyDigits}
                  value={amount}
                  onChange={(value) => setAmount(value == null ? null : roundCurrencyAmount(value, currency))}
                  style={{ width: '100%' }}
                />
                {error ? <Alert type="error" showIcon title={error} /> : null}
                <Button block type="primary" loading={submitting} disabled={!amount || amount <= 0} onClick={() => void submit()}>
                  {t('pages.checkout.resellerDeposit.createOrder', 'Continue to payment')}
                </Button>
              </>
            )}
          </Space>
        ) : null}
      </Card>
      </main>
    </ConfigProvider>
  );
}
