import { useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import {
  Alert,
  Card,
  Col,
  ConfigProvider,
  Descriptions,
  Empty,
  Layout,
  Progress,
  Row,
  Space,
  Statistic,
  Table,
  Tag,
  Typography,
  message,
} from 'antd';
import { ShopOutlined, ThunderboltOutlined } from '@ant-design/icons';

import AppSidebar from '@/layouts/AppSidebar';
import { HttpUtil } from '@/utils';
import { SizeFormatter } from '@/utils';
import { setMessageInstance } from '@/utils/messageBus';
import { keys } from '@/api/queryKeys';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { useTheme } from '@/hooks/useTheme';
import type { ResellerReport } from '@/api/queries/useSession';
import './ResellersPage.css';

export default function ResellerReportPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const { isMobile } = useMediaQuery();
  const [messageApi, messageContextHolder] = message.useMessage();

  useEffect(() => {
    setMessageInstance(messageApi);
  }, [messageApi]);

  const pageClass = useMemo(() => {
    const classes = ['resellers-page'];
    if (isDark) classes.push('is-dark');
    if (isUltra) classes.push('is-ultra');
    return classes.join(' ');
  }, [isDark, isUltra]);

  const reportQuery = useQuery({
    queryKey: keys.resellers.selfReport(),
    queryFn: async (): Promise<ResellerReport> => {
      const msg = await HttpUtil.get<ResellerReport>('/panel/api/reseller/report');
      if (!msg.success) throw new Error(msg.msg || 'failed');
      return msg.obj as ResellerReport;
    },
  });

  const stat = reportQuery.data?.stat;
  const trafficPercent = stat?.reseller.trafficLimit
    ? Math.min(100, Math.round((stat.usedTraffic / stat.reseller.trafficLimit) * 100))
    : 0;

  return (
    <ConfigProvider theme={antdThemeConfig}>
      {messageContextHolder}
      <Layout className={pageClass}>
        <AppSidebar />
        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            {reportQuery.isError ? (
              <Alert type="error" showIcon message={t('somethingWentWrong')} />
            ) : (
              <Row gutter={[isMobile ? 8 : 16, isMobile ? 8 : 12]}>
                <Col span={24}>
                  <Card size="small" hoverable className="summary-card">
                    <Row gutter={[16, 12]}>
                      <Col xs={12} md={6}>
                        <Statistic
                          title={t('resellers.usedTraffic')}
                          value={SizeFormatter.sizeFormat(stat?.usedTraffic || 0)}
                          prefix={<ThunderboltOutlined />}
                        />
                      </Col>
                      <Col xs={12} md={6}>
                        <Statistic
                          title={t('resellers.allocatedTraffic')}
                          value={SizeFormatter.sizeFormat(stat?.allocatedTraffic || 0)}
                        />
                      </Col>
                      <Col xs={12} md={6}>
                        <Statistic title={t('resellers.table.inbounds')} value={stat?.inboundCount || 0} />
                      </Col>
                      <Col xs={12} md={6}>
                        <Statistic title={t('resellers.online')} value={stat?.onlineCount || 0} />
                      </Col>
                    </Row>
                    {stat && (
                      <div style={{ marginTop: 12 }}>
                        <Typography.Text type="secondary">
                          {t('resellers.usedTraffic')}: {SizeFormatter.sizeFormat(stat.usedTraffic)}
                          {stat.reseller.trafficLimit
                            ? ` / ${SizeFormatter.sizeFormat(stat.reseller.trafficLimit)}`
                            : ` / ${t('resellers.unlimited')}`}
                        </Typography.Text>
                        <Progress
                          percent={stat.reseller.trafficLimit ? trafficPercent : 0}
                          showInfo={false}
                          status={stat.overQuota ? 'exception' : 'normal'}
                        />
                      </div>
                    )}
                  </Card>
                </Col>

                {stat && (
                  <Col span={24}>
                    <Card size="small" hoverable title={<Space><ShopOutlined />{t('resellers.info')}</Space>}>
                      <Descriptions size="small" bordered column={isMobile ? 1 : 3}>
                        <Descriptions.Item label={t('resellers.table.reseller')}>
                          {stat.reseller.name || stat.reseller.username}
                        </Descriptions.Item>
                        <Descriptions.Item label={t('username')}>{stat.reseller.username}</Descriptions.Item>
                        <Descriptions.Item label={t('resellers.table.enable')}>
                          {stat.reseller.enable ? <Tag color="green">{t('enable')}</Tag> : <Tag color="red">{t('disabled')}</Tag>}
                        </Descriptions.Item>
                        <Descriptions.Item label={t('resellers.clientLimit')}>
                          {stat.clientCount} / {stat.reseller.clientLimit || t('resellers.unlimited')}
                        </Descriptions.Item>
                        <Descriptions.Item label={t('resellers.table.inbounds')}>
                          {stat.inboundCount}
                        </Descriptions.Item>
                        <Descriptions.Item label={t('resellers.online')}>{stat.onlineCount}</Descriptions.Item>
                        <Descriptions.Item label={t('resellers.expiry')}>
                          {stat.reseller.expiryTime
                            ? new Date(stat.reseller.expiryTime).toLocaleString()
                            : t('resellers.never')}
                        </Descriptions.Item>
                        <Descriptions.Item label={t('resellers.table.used')}>
                          {SizeFormatter.sizeFormat(stat.usedTraffic)}
                        </Descriptions.Item>
                      </Descriptions>
                    </Card>
                  </Col>
                )}

                <Col span={24}>
                  <Card size="small" hoverable title={t('resellers.clientsTitle')}>
                    <Table
                      rowKey="email"
                      size="small"
                      loading={reportQuery.isLoading}
                      dataSource={reportQuery.data?.clients || []}
                      pagination={{ pageSize: 20 }}
                      scroll={{ x: 'max-content' }}
                      locale={{ emptyText: <Empty description={t('resellers.empty')} /> }}
                      columns={[
                        { title: t('pages.clients.email'), dataIndex: 'email', key: 'email' },
                        {
                          title: t('resellers.table.quota'),
                          key: 'quota',
                          render: (_v, row) => (row.totalBytes ? SizeFormatter.sizeFormat(row.totalBytes) : t('resellers.unlimited')),
                        },
                        {
                          title: t('resellers.table.used'),
                          key: 'used',
                          render: (_v, row) => SizeFormatter.sizeFormat(row.used),
                        },
                        {
                          title: t('resellers.inbounds'),
                          key: 'inbounds',
                          render: (_v, row) => row.inboundIds.map((id) => <Tag key={id}>#{id}</Tag>),
                        },
                      ]}
                    />
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
