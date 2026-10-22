import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Card, ConfigProvider, Form, Input, Result, Space, Spin, Typography, message } from 'antd';
import { CheckCircleOutlined, ShopOutlined, UserAddOutlined } from '@ant-design/icons';

import { HttpUtil } from '@/utils';
import { useTheme } from '@/hooks/useTheme';

interface SignupInfo {
  resellerName: string;
  brandName?: string;
  brandLogo?: string;
  brandColor?: string;
  brandTheme?: string;
  planName: string;
  description?: string;
  totalBytes: number;
  days: number;
}

/** Anonymous customer registration for a reseller's fixed plan invitation. */
export default function ResellerSignupPage() {
  const { t } = useTranslation();
  const { isDark } = useTheme();
  const [params] = useSearchParams();
  const ref = (params.get('ref') || '').trim();
  const [email, setEmail] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [complete, setComplete] = useState(false);
  const [error, setError] = useState('');
  const [messageApi, holder] = message.useMessage();

  const infoQuery = useQuery({
    queryKey: ['reseller-signup', 'info', ref],
    queryFn: async (): Promise<SignupInfo> => {
      const response = await HttpUtil.get('/api/reseller-signup/info', { ref }, { silent: true });
      if (!response?.success || !response.obj) throw new Error(response?.msg || 'Registration link is unavailable.');
      return response.obj as SignupInfo;
    },
    enabled: Boolean(ref),
    retry: false,
  });

  async function submit() {
    const normalized = email.trim();
    if (!normalized) return;
    setSubmitting(true);
    setError('');
    try {
      const response = await HttpUtil.post(
        '/api/reseller-signup/register',
        { email: normalized },
        { params: { ref }, headers: { 'Content-Type': 'application/json' }, silent: true } as never,
      );
      if (!response?.success) {
        setError(response?.msg || 'Registration could not be completed.');
        return;
      }
      setComplete(true);
      messageApi.success('Registration completed.');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Registration could not be completed.');
    } finally {
      setSubmitting(false);
    }
  }

  const info = infoQuery.data;
  const themeColors: Record<string, string> = { ocean: '#1677ff', forest: '#1c9a68', sunset: '#d9485f' };
  const brandAccent = info?.brandColor || themeColors[info?.brandTheme || ''] || undefined;
  return (
    <ConfigProvider theme={brandAccent ? { token: { colorPrimary: brandAccent } } : undefined}>
      <main style={{ minHeight: '100vh', display: 'grid', placeItems: 'center', padding: 16, background: isDark ? '#10131a' : '#f5f7fa' }}>
      {holder}
      <Card style={{ width: 'min(100%, 460px)' }}>
        {infoQuery.isLoading ? <Spin /> : null}
        {(!ref || infoQuery.isError) ? (
          <Result status="error" title={t('pages.checkout.resellerSignup.invalidTitle', 'Registration link unavailable')} subTitle={t('pages.checkout.resellerSignup.invalidDesc', 'This invitation is invalid, expired, or no longer available.')} />
        ) : null}
        {info && complete ? (
          <Result
            status="success"
            icon={<CheckCircleOutlined />}
            title={t('pages.checkout.resellerSignup.successTitle', 'Registration complete')}
            subTitle={t('pages.checkout.resellerSignup.successDesc', 'Your account has been created. Contact your reseller to receive your connection details.')}
          />
        ) : null}
        {info && !complete ? (
          <Space direction="vertical" size="large" style={{ width: '100%' }}>
            <div style={{ textAlign: 'center' }}>
              {info.brandLogo ? <img src={info.brandLogo} alt={info.brandName || info.resellerName} style={{ maxWidth: 160, maxHeight: 64, objectFit: 'contain' }} /> : <ShopOutlined style={{ fontSize: 30, color: info.brandColor || '#1677ff' }} />}
              <Typography.Title level={3} style={{ margin: '10px 0 4px', color: info.brandColor || undefined }}>{info.brandName || info.resellerName}</Typography.Title>
              <Typography.Text type="secondary">{t('pages.checkout.resellerSignup.invited', 'invites you to register')}</Typography.Text>
            </div>
            <Card size="small" type="inner" title={info.planName}>
              {info.description ? <Typography.Paragraph>{info.description}</Typography.Paragraph> : null}
              <Typography.Text type="secondary">
                {info.totalBytes > 0 ? `${(info.totalBytes / (1024 ** 3)).toFixed(2)} GB` : t('unlimited', 'Unlimited')}
                {info.days > 0 ? ` · ${info.days} ${t('pages.checkout.resellerSignup.days', 'days')}` : ''}
              </Typography.Text>
            </Card>
            <Form layout="vertical" onFinish={submit}>
              <Form.Item
                label={t('pages.checkout.resellerSignup.email', 'Email or account name')}
                help={t('pages.checkout.resellerSignup.emailHint', 'Use a unique identifier you can share with your reseller.')}
                required
              >
                <Input
                  autoComplete="email"
                  maxLength={191}
                  value={email}
                  onChange={(event) => setEmail(event.target.value)}
                  onPressEnter={() => void submit()}
                />
              </Form.Item>
              {error ? <Alert type="error" showIcon message={error} style={{ marginBottom: 16 }} /> : null}
              <Button block type="primary" htmlType="submit" icon={<UserAddOutlined />} loading={submitting} disabled={!email.trim()}>
                {t('pages.checkout.resellerSignup.submit', 'Create account')}
              </Button>
            </Form>
          </Space>
        ) : null}
      </Card>
      </main>
    </ConfigProvider>
  );
}
