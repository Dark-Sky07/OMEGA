import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Alert,
  Button,
  Card,
  Checkbox,
  Col,
  ConfigProvider,
  Descriptions,
  Drawer,
  Empty,
  Form,
  Input,
  InputNumber,
  Layout,
  Modal,
  Popconfirm,
  Progress,
  Row,
  Select,
  Space,
  Spin,
  Statistic,
  Switch,
  Table,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
  AccountBookOutlined,
  DeleteOutlined,
  EditOutlined,
  ExportOutlined,
  KeyOutlined,
  PlusOutlined,
  ReloadOutlined,
  ShopOutlined,
} from '@ant-design/icons';

import AppSidebar from '@/layouts/AppSidebar';
import { HttpUtil } from '@/utils';
import { SizeFormatter } from '@/utils';
import { setMessageInstance } from '@/utils/messageBus';
import { keys } from '@/api/queryKeys';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { useTheme } from '@/hooks/useTheme';
import type { ResellerReport, ResellerStat } from '@/api/queries/useSession';
import './ResellersPage.css';

const GB = 1024 * 1024 * 1024;

interface ResellerFormValues {
  username: string;
  password?: string;
  name?: string;
  comment?: string;
  enable: boolean;
  trafficLimitGb?: number | null;
  clientLimit?: number | null;
  inboundIds?: number[];
}

interface InboundRow {
  id: number;
  remark: string;
  port: number;
  protocol: string;
  enable: boolean;
}

function quotaText(used: number, limit: number, t: (key: string) => string): string {
  if (!limit) return `${used} / ${t('resellers.unlimited')}`;
  return `${used} / ${limit}`;
}

export default function ResellersPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const { isMobile } = useMediaQuery();
  const queryClient = useQueryClient();
  const [messageApi, messageContextHolder] = message.useMessage();

  useEffect(() => {
    setMessageInstance(messageApi);
  }, [messageApi]);

  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<ResellerStat | null>(null);
  const [passwordFor, setPasswordFor] = useState<ResellerStat | null>(null);
  const [reportFor, setReportFor] = useState<ResellerStat | null>(null);
  const [clientFor, setClientFor] = useState<ResellerStat | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [form] = Form.useForm<ResellerFormValues>();
  const [passwordForm] = Form.useForm<{ password: string }>();
  const [clientForm] = Form.useForm<{ email: string; resellerId: number }>();

  const pageClass = useMemo(() => {
    const classes = ['resellers-page'];
    if (isDark) classes.push('is-dark');
    if (isUltra) classes.push('is-ultra');
    return classes.join(' ');
  }, [isDark, isUltra]);

  const listQuery = useQuery({
    queryKey: keys.resellers.list(),
    queryFn: async (): Promise<ResellerStat[]> => {
      const msg = await HttpUtil.get<ResellerStat[]>('/panel/api/resellers/list');
      if (!msg.success) throw new Error(msg.msg || 'failed');
      return msg.obj || [];
    },
  });

  const inboundsQuery = useQuery({
    queryKey: [...keys.inbounds.slim(), 'all'],
    queryFn: async (): Promise<InboundRow[]> => {
      const msg = await HttpUtil.get<InboundRow[]>('/panel/api/inbounds/list/slim');
      if (!msg.success) throw new Error(msg.msg || 'failed');
      return msg.obj || [];
    },
    enabled: formOpen,
  });

  const assignmentsQuery = useQuery({
    queryKey: keys.resellers.assignments(),
    queryFn: async (): Promise<{ resellerId: number; inboundIds: number[]; emails: string[] }[]> => {
      const msg = await HttpUtil.get<{ resellerId: number; inboundIds: number[]; emails: string[] }[]>(
        '/panel/api/resellers/assignments',
      );
      if (!msg.success) throw new Error(msg.msg || 'failed');
      return msg.obj || [];
    },
    enabled: formOpen || clientFor !== null,
  });

  const clientsQuery = useQuery({
    queryKey: [...keys.clients.all(), 'assignable'],
    queryFn: async (): Promise<{ email: string; totalGB: number }[]> => {
      const msg = await HttpUtil.get<{ email: string; totalGB: number }[]>('/panel/api/clients/list');
      if (!msg.success) throw new Error(msg.msg || 'failed');
      return msg.obj || [];
    },
    enabled: clientFor !== null,
  });

  const reportQuery = useQuery({
    queryKey: keys.resellers.report(reportFor?.reseller.id || 0),
    queryFn: async (): Promise<ResellerReport> => {
      const msg = await HttpUtil.get<ResellerReport>(`/panel/api/resellers/report/${reportFor?.reseller.id}`);
      if (!msg.success) throw new Error(msg.msg || 'failed');
      return msg.obj as ResellerReport;
    },
    enabled: reportFor !== null,
  });

  const refreshAll = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: keys.resellers.root() });
    queryClient.invalidateQueries({ queryKey: keys.inbounds.root() });
  }, [queryClient]);

  const openCreate = useCallback(() => {
    setEditing(null);
    form.resetFields();
    form.setFieldsValue({ enable: true, inboundIds: [] });
    setFormOpen(true);
  }, [form]);

  const openEdit = useCallback(
    (stat: ResellerStat) => {
      setEditing(stat);
      form.setFieldsValue({
        username: stat.reseller.username,
        password: '',
        name: stat.reseller.name,
        comment: stat.reseller.comment,
        enable: stat.reseller.enable,
        trafficLimitGb: stat.reseller.trafficLimit ? stat.reseller.trafficLimit / GB : 0,
        clientLimit: stat.reseller.clientLimit,
      });
      setFormOpen(true);
    },
    [form],
  );

  const submitForm = useCallback(async () => {
    let values: ResellerFormValues;
    try {
      values = await form.validateFields();
    } catch {
      return;
    }
    setSubmitting(true);
    try {
      const body = {
        username: values.username,
        password: values.password || '',
        name: values.name || values.username,
        comment: values.comment || '',
        enable: values.enable ?? true,
        trafficLimit: Math.round((values.trafficLimitGb || 0) * GB),
        clientLimit: values.clientLimit || 0,
      };
      const msg = editing
        ? await HttpUtil.post(`/panel/api/resellers/update/${editing.reseller.id}`, body)
        : await HttpUtil.post('/panel/api/resellers/add', body);

      if (!msg.success) {
        messageApi.error(msg.msg || t('somethingWentWrong'));
        return;
      }

      // Attach inbound: admin decides which inbounds the reseller can access.
      const selected: number[] = values.inboundIds || [];
      const targetId = editing ? editing.reseller.id : (msg.obj as { id: number } | null)?.id;
      if (targetId) {
        const prevOwned = editing
          ? (assignmentsQuery.data || []).find((entry) => entry.resellerId === editing.reseller.id)?.inboundIds || []
          : [];
        const prev = new Set(prevOwned);
        const next = new Set(selected);
        for (const id of selected) {
          if (!prev.has(id)) {
            const assignMsg = await HttpUtil.post('/panel/api/resellers/assignInbound', {
              resellerId: targetId,
              inboundId: id,
            });
            if (!assignMsg.success) {
              messageApi.error(assignMsg.msg || t('somethingWentWrong'));
            }
          }
        }
        for (const id of prevOwned) {
          if (!next.has(id)) {
            const unassignMsg = await HttpUtil.post('/panel/api/resellers/unassignInbound', {
              resellerId: targetId,
              inboundId: id,
            });
            if (!unassignMsg.success) {
              messageApi.error(unassignMsg.msg || t('somethingWentWrong'));
            }
          }
        }
      }
      messageApi.success(t(editing ? 'resellers.toasts.updated' : 'resellers.toasts.created'));
      setFormOpen(false);
      refreshAll();
      queryClient.invalidateQueries({ queryKey: keys.resellers.assignments() });
    } catch (err) {
      const m = err instanceof Error ? err.message : String(err);
      messageApi.error(m || t('somethingWentWrong'));
    } finally {
      setSubmitting(false);
    }
  }, [editing, form, messageApi, refreshAll, t, assignmentsQuery.data, queryClient]);

  const toggleEnable = useCallback(
    async (stat: ResellerStat, enable: boolean) => {
      const msg = await HttpUtil.post(`/panel/api/resellers/setEnable/${stat.reseller.id}`, { enable });
      if (msg.success) {
        messageApi.success(t('resellers.toasts.updated'));
        refreshAll();
      } else {
        messageApi.error(msg.msg || t('somethingWentWrong'));
      }
    },
    [messageApi, refreshAll, t],
  );

  const removeReseller = useCallback(
    async (stat: ResellerStat) => {
      const msg = await HttpUtil.post(`/panel/api/resellers/del/${stat.reseller.id}`);
      if (msg.success) {
        messageApi.success(t('resellers.toasts.deleted'));
        refreshAll();
      } else {
        messageApi.error(msg.msg || t('somethingWentWrong'));
      }
    },
    [messageApi, refreshAll, t],
  );

  const submitPassword = useCallback(async () => {
    const values = await passwordForm.validateFields().catch(() => null);
    if (!values) return;
    const msg = await HttpUtil.post(`/panel/api/resellers/resetPassword/${passwordFor?.reseller.id}`, {
      password: values.password,
    });
    if (msg.success) {
      messageApi.success(t('resellers.toasts.passwordReset'));
      setPasswordFor(null);
      passwordForm.resetFields();
    } else {
      messageApi.error(msg.msg || t('somethingWentWrong'));
    }
  }, [messageApi, passwordFor, passwordForm, t]);

  const submitAssignClient = useCallback(async () => {
    const values = await clientForm.validateFields().catch(() => null);
    if (!values) return;
    const msg = await HttpUtil.post('/panel/api/resellers/assignClient', {
      resellerId: clientFor?.reseller.id,
      email: values.email,
    });
    if (msg.success) {
      messageApi.success(t('resellers.toasts.clientAssigned'));
      clientForm.resetFields();
      queryClient.invalidateQueries({ queryKey: keys.resellers.assignments() });
      refreshAll();
    } else {
      messageApi.error(msg.msg || t('somethingWentWrong'));
    }
  }, [clientFor, clientForm, messageApi, queryClient, refreshAll, t]);

  const unassignClient = useCallback(
    async (email: string) => {
      const msg = await HttpUtil.post('/panel/api/resellers/unassignClient', {
        resellerId: clientFor?.reseller.id,
        email,
      });
      if (msg.success) {
        messageApi.success(t('resellers.toasts.clientUnassigned'));
        queryClient.invalidateQueries({ queryKey: keys.resellers.assignments() });
        refreshAll();
      } else {
        messageApi.error(msg.msg || t('somethingWentWrong'));
      }
    },
    [clientFor, messageApi, queryClient, refreshAll, t],
  );

  const rows = listQuery.data || [];
  const assignments = useMemo(() => assignmentsQuery.data || [], [assignmentsQuery.data]);
  const ownerByInbound = useMemo(() => {
    const map = new Map<number, number>();
    assignments.forEach((entry) => entry.inboundIds.forEach((id) => map.set(id, entry.resellerId)));
    return map;
  }, [assignments]);
  const explicitEmails = useMemo(
    () => assignments.find((entry) => entry.resellerId === clientFor?.reseller.id)?.emails || [],
    [assignments, clientFor],
  );

  // The assignment map loads lazily when the form opens, so on the first edit
  // it is usually still empty when openEdit runs. Re-apply the owned set once
  // it arrives so the checkboxes always reflect the stored ownership.
  useEffect(() => {
    if (!formOpen || !editing) return;
    const owned = assignments.find((entry) => entry.resellerId === editing.reseller.id)?.inboundIds || [];
    form.setFieldsValue({ inboundIds: owned });
  }, [formOpen, editing, assignments, form]);

  const columns: ColumnsType<ResellerStat> = [
    {
      title: t('resellers.table.reseller'),
      key: 'reseller',
      render: (_value, stat) => (
        <Space orientation="vertical" size={0}>
          <Space>
            <ShopOutlined />
            <b>{stat.reseller.name || stat.reseller.username}</b>
            {!stat.reseller.enable && <Tag color="red">{t('disabled')}</Tag>}
            {stat.expired && <Tag color="orange">{t('resellers.expired')}</Tag>}
            {stat.overQuota && !stat.expired && <Tag color="orange">{t('resellers.overQuota')}</Tag>}
          </Space>
          <Typography.Text type="secondary">{stat.reseller.username}</Typography.Text>
        </Space>
      ),
    },
    {
      title: t('resellers.table.enable'),
      key: 'enable',
      width: 90,
      render: (_value, stat) => (
        <Switch
          checked={stat.reseller.enable}
          onChange={(checked) => toggleEnable(stat, checked)}
          size="small"
        />
      ),
    },
    {
      title: t('resellers.table.inbounds'),
      key: 'inbounds',
      width: 120,
      render: (_value, stat) => <span>{stat.inboundCount}</span>,
    },
    {
      title: t('resellers.table.clients'),
      key: 'clients',
      width: 120,
      render: (_value, stat) => <span>{quotaText(stat.clientCount, stat.reseller.clientLimit, t)}</span>,
    },
    {
      title: t('resellers.table.traffic'),
      key: 'traffic',
      width: 260,
      render: (_value, stat) => {
        const percent = stat.reseller.trafficLimit
          ? Math.min(100, Math.round((stat.allocatedTraffic / stat.reseller.trafficLimit) * 100))
          : 0;
        return (
          <Space orientation="vertical" size={2} style={{ width: '100%' }}>
            <Typography.Text>
              {SizeFormatter.sizeFormat(stat.allocatedTraffic)}
              {stat.reseller.trafficLimit
                ? ` / ${SizeFormatter.sizeFormat(stat.reseller.trafficLimit)}`
                : ` / ${t('resellers.unlimited')}`}
            </Typography.Text>
            <Progress
              percent={percent}
              size="small"
              showInfo={false}
              status={stat.overQuota ? 'exception' : 'normal'}
            />
            <Typography.Text type="secondary">
              {t('resellers.table.used')}: {SizeFormatter.sizeFormat(stat.usedTraffic)}
            </Typography.Text>
          </Space>
        );
      },
    },
    {
      title: t('resellers.table.actions'),
      key: 'actions',
      width: isMobile ? 90 : 220,
      render: (_value, stat) => (
        <Space size={1} wrap>
          <Tooltip title={t('resellers.report')}>
            <Button size="small" icon={<AccountBookOutlined />} onClick={() => setReportFor(stat)} />
          </Tooltip>
          <Tooltip title={t('resellers.assignClient')}>
            <Button
              size="small"
              icon={<ExportOutlined />}
              onClick={() => {
                setClientFor(stat);
                clientForm.resetFields();
                clientForm.setFieldsValue({ resellerId: stat.reseller.id });
                assignmentsQuery.refetch();
              }}
            />
          </Tooltip>
          <Tooltip title={t('resellers.resetPassword')}>
            <Button
              size="small"
              icon={<KeyOutlined />}
              onClick={() => {
                setPasswordFor(stat);
                passwordForm.resetFields();
              }}
            />
          </Tooltip>
          <Tooltip title={t('edit')}>
            <Button size="small" icon={<EditOutlined />} onClick={() => openEdit(stat)} />
          </Tooltip>
          <Popconfirm
            title={t('resellers.deleteConfirm')}
            onConfirm={() => removeReseller(stat)}
            okText={t('confirm')}
            cancelText={t('cancel')}
          >
            <Button size="small" danger icon={<DeleteOutlined />} />
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <ConfigProvider theme={antdThemeConfig}>
      {messageContextHolder}
      <Layout className={pageClass}>
        <AppSidebar />
        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <Row gutter={[isMobile ? 8 : 16, isMobile ? 8 : 12]}>
              <Col span={24}>
                <Card size="small" hoverable className="summary-card">
                  <Row gutter={[16, 12]} align="middle">
                    <Col xs={24} md={8}>
                      <Statistic
                        title={t('resellers.total')}
                        value={String(rows.length)}
                        prefix={<ShopOutlined />}
                      />
                    </Col>
                    <Col xs={24} md={16}>
                      <Space wrap style={{ width: '100%', justifyContent: 'flex-end' }}>
                        <Button icon={<ReloadOutlined />} onClick={() => refreshAll()} loading={listQuery.isFetching}>
                          {t('refresh')}
                        </Button>
                        <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
                          {t('resellers.add')}
                        </Button>
                      </Space>
                    </Col>
                  </Row>
                </Card>
              </Col>

              <Col span={24}>
                <Card size="small" hoverable>
                  {listQuery.isError && (
                    <Alert type="error" showIcon message={t('somethingWentWrong')} style={{ marginBottom: 12 }} />
                  )}
                  <Table<ResellerStat>
                    rowKey={(stat) => String(stat.reseller.id)}
                    size="small"
                    loading={listQuery.isLoading}
                    columns={columns}
                    dataSource={rows}
                    pagination={rows.length > 20 ? { pageSize: 20 } : false}
                    scroll={{ x: 'max-content' }}
                    locale={{ emptyText: <Empty description={t('resellers.empty')} /> }}
                  />
                </Card>
              </Col>
            </Row>

            <Modal
              open={formOpen}
              title={editing ? t('resellers.edit') : t('resellers.add')}
              onCancel={() => setFormOpen(false)}
              onOk={submitForm}
              confirmLoading={submitting}
              okText={t('save')}
              cancelText={t('close')}
              destroyOnHidden
            >
              <Form form={form} layout="vertical">
                <Form.Item
                  name="username"
                  label={t('username')}
                  rules={[{ required: true, message: t('resellers.validation.username') }]}
                >
                  <Input autoComplete="off" />
                </Form.Item>
                <Form.Item
                  name="password"
                  label={t('password')}
                  rules={editing ? [] : [{ required: true, message: t('resellers.validation.password') }]}
                  extra={editing ? t('resellers.passwordKeepHint') : undefined}
                >
                  <Input.Password autoComplete="new-password" />
                </Form.Item>
                <Form.Item name="name" label={t('resellers.name')}>
                  <Input />
                </Form.Item>
                <Form.Item name="comment" label={t('resellers.comment')}>
                  <Input.TextArea rows={2} />
                </Form.Item>
                <Form.Item name="enable" label={t('resellers.table.enable')} valuePropName="checked">
                  <Switch />
                </Form.Item>
                <Row gutter={8}>
                  <Col span={12}>
                    <Form.Item name="trafficLimitGb" label={t('resellers.trafficLimit')} extra={t('resellers.zeroUnlimited')}>
                      <InputNumber min={0} step={10} style={{ width: '100%' }} addonAfter="GB" />
                    </Form.Item>
                  </Col>
                  <Col span={12}>
                    <Form.Item name="clientLimit" label={t('resellers.clientLimit')} extra={t('resellers.zeroUnlimited')}>
                      <InputNumber min={0} style={{ width: '100%' }} />
                    </Form.Item>
                  </Col>
                </Row>
                <Form.Item name="inboundIds" label={t('resellers.inbounds')} extra={t('resellers.inboundsHint')}>
                  {inboundsQuery.isLoading ? (
                    <Spin size="small" />
                  ) : (
                    <Checkbox.Group style={{ width: '100%' }}>
                      <Space orientation="vertical" style={{ width: '100%' }}>
                        {(inboundsQuery.data || []).map((row) => {
                          const owner = ownerByInbound.get(row.id);
                          const takenByOther = owner !== undefined && (!editing || owner !== editing.reseller.id);
                          const ownerName =
                            owner === undefined
                              ? null
                              : rows.find((r) => r.reseller.id === owner)?.reseller.name || `#${owner}`;
                          return (
                            <Checkbox key={row.id} value={row.id} disabled={takenByOther}>
                              <Space size={4}>
                                <span>{row.remark || `#${row.id}`}</span>
                                <Tag>{row.protocol}</Tag>
                                <Tag>:{row.port}</Tag>
                                {owner !== undefined && (
                                  <Tag color={editing && owner === editing.reseller.id ? 'green' : 'blue'}>
                                    {ownerName}
                                  </Tag>
                                )}
                              </Space>
                            </Checkbox>
                          );
                        })}
                        {(!inboundsQuery.data || inboundsQuery.data.length === 0) && (
                          <Typography.Text type="secondary">{t('resellers.emptyInbounds') || 'No inbounds'}</Typography.Text>
                        )}
                      </Space>
                    </Checkbox.Group>
                  )}
                </Form.Item>
              </Form>
            </Modal>

            <Modal
              open={clientFor !== null}
              title={`${t('resellers.assignClient')} — ${clientFor?.reseller.name || ''}`}
              onCancel={() => setClientFor(null)}
              onOk={submitAssignClient}
              okText={t('resellers.assign')}
              cancelText={t('close')}
            >
              <Form form={clientForm} layout="vertical">
                <Form.Item
                  name="email"
                  label={t('pages.clients.email')}
                  rules={[{ required: true, message: t('resellers.validation.email') }]}
                >
                  <Select
                    showSearch
                    optionFilterProp="label"
                    loading={clientsQuery.isLoading}
                    options={(clientsQuery.data || []).map((client) => ({
                      value: client.email,
                      label: client.email,
                    }))}
                  />
                </Form.Item>
              </Form>
              <Typography.Paragraph type="secondary">{t('resellers.assignClientHint')}</Typography.Paragraph>
              <Table<{ email: string }>
                rowKey="email"
                size="small"
                dataSource={explicitEmails.map((email) => ({ email }))}
                pagination={false}
                locale={{ emptyText: <Empty description={t('resellers.noAssignedClients')} /> }}
                columns={[
                  { title: t('pages.clients.email'), dataIndex: 'email', key: 'email' },
                  {
                    title: '',
                    key: 'actions',
                    width: 90,
                    render: (_value, row) => (
                      <Button size="small" danger onClick={() => unassignClient(row.email)}>
                        {t('resellers.unassign')}
                      </Button>
                    ),
                  },
                ]}
              />
            </Modal>

            <Modal
              open={passwordFor !== null}
              title={`${t('resellers.resetPassword')} — ${passwordFor?.reseller.name || ''}`}
              onCancel={() => setPasswordFor(null)}
              onOk={submitPassword}
              okText={t('save')}
              cancelText={t('close')}
            >
              <Form form={passwordForm} layout="vertical">
                <Form.Item
                  name="password"
                  label={t('password')}
                  rules={[{ required: true, message: t('resellers.validation.password') }]}
                >
                  <Input.Password autoComplete="new-password" />
                </Form.Item>
              </Form>
            </Modal>

            <Drawer
              open={reportFor !== null}
              onClose={() => setReportFor(null)}
              width={isMobile ? '100%' : 860}
              title={`${t('resellers.report')} — ${reportFor?.reseller.name || ''}`}
            >
              {reportQuery.isLoading ? null : reportQuery.data ? (
                <Space orientation="vertical" size={16} style={{ width: '100%' }}>
                  <Row gutter={[8, 8]}>
                    <Col xs={12} md={6}>
                      <Statistic
                        title={t('resellers.usedTraffic')}
                        value={SizeFormatter.sizeFormat(reportQuery.data.stat.usedTraffic)}
                      />
                    </Col>
                    <Col xs={12} md={6}>
                      <Statistic
                        title={t('resellers.allocatedTraffic')}
                        value={SizeFormatter.sizeFormat(reportQuery.data.stat.allocatedTraffic)}
                      />
                    </Col>
                    <Col xs={12} md={6}>
                      <Statistic title={t('resellers.table.inbounds')} value={reportQuery.data.stat.inboundCount} />
                    </Col>
                    <Col xs={12} md={6}>
                      <Statistic title={t('resellers.online')} value={reportQuery.data.stat.onlineCount} />
                    </Col>
                  </Row>
                  <Descriptions size="small" bordered column={isMobile ? 1 : 2}>
                    <Descriptions.Item label={t('resellers.clientLimit')}>
                      {reportQuery.data.stat.clientCount} / {reportQuery.data.stat.reseller.clientLimit || t('resellers.unlimited')}
                    </Descriptions.Item>
                    <Descriptions.Item label={t('resellers.table.inbounds')}>
                      {reportQuery.data.stat.inboundCount}
                    </Descriptions.Item>
                    <Descriptions.Item label={t('resellers.online')}>{reportQuery.data.stat.onlineCount}</Descriptions.Item>
                    <Descriptions.Item label={t('resellers.expiry')}>
                      {reportQuery.data.stat.reseller.expiryTime
                        ? new Date(reportQuery.data.stat.reseller.expiryTime).toLocaleString()
                        : t('resellers.never')}
                    </Descriptions.Item>
                  </Descriptions>
                  <Card size="small" title={t('resellers.clientsTitle')}>
                    <Table
                      rowKey="email"
                      size="small"
                      dataSource={reportQuery.data.clients}
                      pagination={{ pageSize: 10 }}
                      scroll={{ x: 'max-content' }}
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
                          title: t('resellers.owner'),
                          key: 'inbounds',
                          render: (_v, row) => row.inboundIds.map((id) => <Tag key={id}>#{id}</Tag>),
                        },
                      ]}
                    />
                  </Card>
                </Space>
              ) : null}
            </Drawer>
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
