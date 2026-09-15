import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Alert,
  Button,
  Card,
  Col,
  ConfigProvider,
  Descriptions,
  Form,
  Input,
  Layout,
  Progress,
  Row,
  Space,
  Statistic,
  Tag,
  Typography,
  message,
} from 'antd';
import { ShopOutlined, UserOutlined } from '@ant-design/icons';

import AppSidebar from '@/layouts/AppSidebar';
import { HttpUtil } from '@/utils';
import { SizeFormatter } from '@/utils';
import { keys } from '@/api/queryKeys';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { useTheme } from '@/hooks/useTheme';
import type { ResellerStat } from '@/api/queries/useSession';
import './ResellersPage.css';

export default function ResellerProfilePage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const { isMobile } = useMediaQuery();
  const queryClient = useQueryClient();
  const [messageApi, messageContextHolder] = message.useMessage();
  const [saving, setSaving] = useState(false);
  const [passwordForm] = Form.useForm<{ oldPassword: string; newPassword: string; confirmPassword: string }>();

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
  const trafficPercent = stat?.reseller.trafficLimit
    ? Math.min(100, Math.round((stat.allocatedTraffic / stat.reseller.trafficLimit) * 100))
    : 0;

  const changePassword = useCallback(async () => {
    const values = await passwordForm.validateFields().catch(() => null);
    if (!values) return;
    setSaving(true);
    try {
      const msg = await HttpUtil.post('/panel/api/reseller/password', {
        oldPassword: values.oldPassword,
        newPassword: values.newPassword,
      });
      if (msg.success) {
        messageApi.success(t('resellers.toasts.passwordChanged'));
        passwordForm.resetFields();
        queryClient.invalidateQueries({ queryKey: keys.session.me() });
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
              <Row gutter={[isMobile ? 8 : 16, isMobile ? 8 : 12]}>
                <Col xs={24} lg={16}>
                  <Card
                    size="small"
                    hoverable
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
                          <Statistic
                            title={t('resellers.table.inbounds')}
                            value={
                              stat?.reseller.inboundLimit
                                ? `${stat.inboundCount} / ${stat.reseller.inboundLimit}`
                                : `${stat?.inboundCount ?? 0} / ${t('resellers.unlimited')}`
                            }
                          />
                        </Col>
                      </Row>
                      <Row gutter={8}>
                        <Col span={12}>
                          <Statistic title={t('resellers.table.cost')} value={(stat?.cost || 0).toFixed(2)} />
                        </Col>
                        <Col span={12}>
                          <Statistic
                            title={t('resellers.table.balance')}
                            value={(stat?.balance || 0).toFixed(2)}
                            valueStyle={{ color: (stat?.balance || 0) < 0 ? '#cf1322' : undefined }}
                          />
                        </Col>
                      </Row>
                      <Row gutter={8}>
                        <Col span={12}>
                          <Statistic title={t('resellers.pricePerGb')} value={stat?.reseller.pricePerGb || 0} />
                        </Col>
                        <Col span={12}>
                          <Statistic title={t('resellers.online')} value={stat?.onlineCount || 0} />
                        </Col>
                      </Row>
                    </Space>
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
              </Row>
            )}
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
