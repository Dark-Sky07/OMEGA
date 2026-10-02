import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { Button, ConfigProvider, Layout, Modal, Result, Spin, message } from 'antd';
import {
  CheckCircleOutlined,
  CloseCircleOutlined,
  CloudServerOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';

import { useTheme } from '@/hooks/useTheme';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { useNodesQuery } from '@/api/queries/useNodesQuery';
import type { NodeRecord } from '@/api/queries/useNodesQuery';
import { useNodeMutations } from '@/api/queries/useNodeMutations';
import AppSidebar from '@/layouts/AppSidebar';
import { PageHeader, StatStrip, type StatItem } from '@/components/ui';
import NodeList from './NodeList';
import NodeFormModal from './NodeFormModal';
import { setMessageInstance } from '@/utils/messageBus';
import { HttpUtil } from '@/utils';
import type { PanelUpdateInfo } from '../index/PanelUpdateModal';

export default function NodesPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const { isMobile } = useMediaQuery();
  const [modal, modalContextHolder] = Modal.useModal();
  const [messageApi, messageContextHolder] = message.useMessage();
  useEffect(() => { setMessageInstance(messageApi); }, [messageApi]);

  const { nodes, loading, fetched, fetchError, refetch, totals } = useNodesQuery();
  const { create, update, remove, setEnable, testConnection, fetchFingerprint, fetchInbounds, probe, updatePanels } = useNodeMutations();

  const { data: latestVersion = '' } = useQuery({
    queryKey: ['server', 'panelUpdateInfo'],
    queryFn: async () => {
      const msg = await HttpUtil.get<PanelUpdateInfo>('/panel/api/server/getPanelUpdateInfo');
      return msg?.obj?.latestVersion || '';
    },
    staleTime: 5 * 60 * 1000,
  });

  const [formOpen, setFormOpen] = useState(false);
  const [formMode, setFormMode] = useState<'add' | 'edit'>('add');
  const [formNode, setFormNode] = useState<NodeRecord | null>(null);
  const [selectedIds, setSelectedIds] = useState<number[]>([]);

  const onAdd = useCallback(() => {
    setFormMode('add');
    setFormNode(null);
    setFormOpen(true);
  }, []);

  const onEdit = useCallback((node: NodeRecord) => {
    setFormMode('edit');
    setFormNode({ ...node });
    setFormOpen(true);
  }, []);

  const onSave = useCallback(async (payload: Partial<NodeRecord>) => {
    if (formMode === 'edit' && formNode?.id) {
      return update(formNode.id, payload);
    }
    return create(payload);
  }, [formMode, formNode, update, create]);

  const onDelete = useCallback((node: NodeRecord) => {
    modal.confirm({
      title: t('pages.nodes.deleteConfirmTitle', { name: node.name }),
      content: t('pages.nodes.deleteConfirmContent'),
      okText: t('delete'),
      okType: 'danger',
      cancelText: t('cancel'),
      onOk: async () => {
        const msg = await remove(node.id);
        if (msg?.success) messageApi.success(t('pages.nodes.toasts.deleted'));
      },
    });
  }, [modal, t, remove, messageApi]);

  const onProbe = useCallback(async (node: NodeRecord) => {
    const msg = await probe(node.id);
    if (msg?.success && msg.obj) {
      if (msg.obj.status === 'online') {
        // Even if xray is in error/stop on the node we still reached its panel API.
        messageApi.success(t('pages.nodes.connectionOk', { ms: msg.obj.latencyMs }));
      } else {
        messageApi.error(msg.obj.error || t('pages.nodes.toasts.probeFailed'));
      }
    }
    // Refresh the list so the new xrayState / xrayError (if any) appears immediately in the row.
    refetch();
  }, [probe, t, messageApi, refetch]);

  const onToggleEnable = useCallback(async (node: NodeRecord, next: boolean) => {
    await setEnable(node.id, next);
  }, [setEnable]);

  const runUpdate = useCallback(async (ids: number[]) => {
    const msg = await updatePanels(ids);
    if (!msg?.success) {
      messageApi.error(msg?.msg || t('somethingWentWrong'));
      return;
    }
    const results = msg.obj ?? [];
    const ok = results.filter((r) => r.ok).length;
    const failed = results.length - ok;
    if (failed === 0) {
      messageApi.success(t('pages.nodes.toasts.updateStarted'));
    } else {
      const firstError = results.find((r) => !r.ok)?.error ?? '';
      const base = t('pages.nodes.toasts.updateResult', { ok, failed });
      messageApi.warning(firstError ? `${base} — ${firstError}` : base);
    }
    setSelectedIds([]);
  }, [updatePanels, messageApi, t]);

  const onUpdateNode = useCallback((node: NodeRecord) => {
    modal.confirm({
      title: t('pages.nodes.updateConfirmTitle', { count: 1 }),
      content: t('pages.nodes.updateConfirmContent'),
      okText: t('update'),
      cancelText: t('cancel'),
      onOk: () => runUpdate([node.id]),
    });
  }, [modal, t, runUpdate]);

  const onUpdateSelected = useCallback(() => {
    const eligible = nodes
      .filter((n) => selectedIds.includes(n.id) && n.enable && n.status === 'online')
      .map((n) => n.id);
    if (eligible.length === 0) {
      messageApi.warning(t('pages.nodes.toasts.updateNoneEligible'));
      return;
    }
    modal.confirm({
      title: t('pages.nodes.updateConfirmTitle', { count: eligible.length }),
      content: t('pages.nodes.updateConfirmContent'),
      okText: t('update'),
      cancelText: t('cancel'),
      onOk: () => runUpdate(eligible),
    });
  }, [modal, t, nodes, selectedIds, runUpdate, messageApi]);

  const pageClass = useMemo(() => {
    const classes = ['nodes-page'];
    if (isDark) classes.push('is-dark');
    if (isUltra) classes.push('is-ultra');
    return classes.join(' ');
  }, [isDark, isUltra]);

  const nodeStats = useMemo<StatItem[]>(() => [
    { key: 'total', icon: <CloudServerOutlined />, tone: 'primary', label: t('pages.nodes.totalNodes'), value: String(totals.total) },
    { key: 'online', icon: <CheckCircleOutlined />, tone: 'success', label: t('pages.nodes.onlineNodes'), value: String(totals.online) },
    { key: 'offline', icon: <CloseCircleOutlined />, tone: totals.offline > 0 ? 'danger' : 'muted', label: t('pages.nodes.offlineNodes'), value: String(totals.offline) },
    { key: 'latency', icon: <ThunderboltOutlined />, tone: 'accent', label: t('pages.nodes.avgLatency'), value: totals.avgLatency > 0 ? `${totals.avgLatency} ms` : '-' },
  ], [t, totals.total, totals.online, totals.offline, totals.avgLatency]);

  return (
    <ConfigProvider theme={antdThemeConfig}>
      {messageContextHolder}
      {modalContextHolder}
      <Layout className={pageClass}>
        <AppSidebar />

        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <Spin spinning={!fetched} delay={200} description={t('loading')} size="large">
              {!fetched ? (
                <div className="loading-spacer" />
              ) : fetchError ? (
                <Result
                  status="error"
                  title={t('somethingWentWrong')}
                  subTitle={fetchError}
                  extra={<Button type="primary" loading={loading} onClick={() => refetch()}>{t('refresh')}</Button>}
                />
              ) : (
                <>
                  <PageHeader
                    eyebrow={t('menu.groupManage', 'Manage')}
                    title={t('menu.nodes', 'Nodes')}
                    subtitle={(
                      <>
                        <span><strong>{totals.total}</strong> {t('pages.nodes.totalNodes')}</span>
                        <span className="omega-sep">·</span>
                        <span><strong>{totals.online}</strong> {t('pages.nodes.onlineNodes')}</span>
                      </>
                    )}
                  />

                  <StatStrip size={isMobile ? 'compact' : 'default'} items={nodeStats} />

                  <div className="omega-rise omega-rise-3">
                    <NodeList
                      nodes={nodes}
                      loading={loading}
                      isMobile={isMobile}
                      latestVersion={latestVersion}
                      selectedIds={selectedIds}
                      onSelectionChange={setSelectedIds}
                      onAdd={onAdd}
                      onEdit={onEdit}
                      onDelete={onDelete}
                      onProbe={onProbe}
                      onToggleEnable={onToggleEnable}
                      onUpdateNode={onUpdateNode}
                      onUpdateSelected={onUpdateSelected}
                    />
                  </div>
                </>
              )}
            </Spin>
          </Layout.Content>
        </Layout>

        <NodeFormModal
          open={formOpen}
          mode={formMode}
          node={formNode}
          testConnection={testConnection}
          fetchFingerprint={fetchFingerprint}
          fetchInbounds={fetchInbounds}
          save={onSave}
          onOpenChange={setFormOpen}
        />
      </Layout>
    </ConfigProvider>
  );
}
