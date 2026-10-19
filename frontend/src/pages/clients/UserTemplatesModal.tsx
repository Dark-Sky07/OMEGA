import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  AutoComplete,
  Button,
  Col,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Row,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  message,
} from 'antd';
import { DeleteOutlined, EditOutlined, PlusOutlined } from '@ant-design/icons';
import { formatInboundLabel } from '@/lib/inbounds/label';
import { SizeFormatter } from '@/utils';
import type { InboundOption } from '@/hooks/useClients';
import { useUserTemplates, type UserTemplate } from '@/hooks/useUserTemplates';

interface UserTemplatesModalProps {
  open: boolean;
  inbounds: InboundOption[];
  groups?: string[];
  onOpenChange: (open: boolean) => void;
  onChanged?: () => void;
}

interface TemplateFormState {
  id: number;
  name: string;
  description: string;
  totalGB: number;
  expireDurationDays: number;
  delayedStart: boolean;
  activateBeforeDays: number;
  limitIp: number;
  limitDevice: number;
  reset: number;
  renewalMode: number;
  renewalDay: number;
  fixedPrice: number;
  group: string;
  inboundIds: number[];
}

function emptyTemplateForm(): TemplateFormState {
  return {
    id: 0,
    name: '',
    description: '',
    totalGB: 50,
    expireDurationDays: 30,
    delayedStart: true,
    activateBeforeDays: 0,
    limitIp: 0,
    limitDevice: 0,
    reset: 0,
    renewalMode: 0,
    renewalDay: 0,
    fixedPrice: 0,
    group: '',
    inboundIds: [],
  };
}

export default function UserTemplatesModal({
  open,
  inbounds,
  groups = [],
  onOpenChange,
  onChanged,
}: UserTemplatesModalProps) {
  const { t } = useTranslation();
  const [messageApi, messageContextHolder] = message.useMessage();
  const { templates, loading, createTemplate, updateTemplate, deleteTemplate } = useUserTemplates(open);

  const [editing, setEditing] = useState<TemplateFormState | null>(null);
  const [saving, setSaving] = useState(false);

  const inboundOptions = useMemo(
    () =>
      (inbounds || []).map((ib) => ({
        label: formatInboundLabel(ib.tag, ib.remark),
        value: ib.id,
      })),
    [inbounds],
  );

  function startCreate() {
    setEditing(emptyTemplateForm());
  }

  function startEdit(row: UserTemplate) {
    setEditing({
      id: row.id,
      name: row.name,
      description: row.description || '',
      totalGB: row.totalGB > 0 ? Math.round((row.totalGB / (1024 * 1024 * 1024)) * 100) / 100 : 0,
      expireDurationDays: row.expireDurationDays || 0,
      delayedStart: !!row.delayedStart,
      activateBeforeDays: row.activateBeforeDays || 0,
      limitIp: row.limitIp || 0,
      limitDevice: row.limitDevice || 0,
      reset: row.reset || 0,
      renewalMode: row.renewalMode || 0,
      renewalDay: row.renewalDay || 0,
      fixedPrice: row.fixedPrice || 0,
      group: row.group || '',
      inboundIds: Array.isArray(row.inboundIds) ? row.inboundIds : [],
    });
  }

  async function handleSave() {
    if (!editing) return;
    if (!editing.name.trim()) {
      messageApi.error(t('pages.clients.templateNameRequired'));
      return;
    }
    setSaving(true);
    try {
      const payload = {
        name: editing.name.trim(),
        description: editing.description.trim(),
        totalGB: Math.round((Number(editing.totalGB) || 0) * 1024 * 1024 * 1024),
        expireDurationDays: Number(editing.expireDurationDays) || 0,
        delayedStart: !!editing.delayedStart,
        activateBeforeDays: editing.delayedStart ? Number(editing.activateBeforeDays) || 0 : 0,
        limitIp: Number(editing.limitIp) || 0,
        limitDevice: Number(editing.limitDevice) || 0,
        reset: Number(editing.reset) || 0,
        renewalMode: Number(editing.renewalMode) || 0,
        renewalDay: Number(editing.renewalMode) > 0 ? Number(editing.renewalDay) || 0 : 0,
        fixedPrice: Number(editing.fixedPrice) || 0,
        group: editing.group.trim(),
        inboundIds: editing.inboundIds || [],
      };
      const res = editing.id > 0
        ? await updateTemplate(editing.id, payload)
        : await createTemplate(payload);
      if (res?.success) {
        setEditing(null);
        onChanged?.();
      }
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete(id: number) {
    const res = await deleteTemplate(id);
    if (res?.success) {
      onChanged?.();
    }
  }

  return (
    <>
      {messageContextHolder}
      <Modal
        open={open}
        title={t('pages.clients.userTemplates')}
        width={780}
        zIndex={1050}
        onCancel={() => {
          setEditing(null);
          onOpenChange(false);
        }}
        footer={
          <Button
            onClick={() => {
              setEditing(null);
              onOpenChange(false);
            }}
          >
            {t('close')}
          </Button>
        }
      >
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
          <span style={{ color: 'var(--ant-color-text-secondary)', fontSize: 13 }}>
            {t('pages.clients.userTemplatesDesc')}
          </span>
          <Button type="primary" icon={<PlusOutlined />} onClick={startCreate}>
            {t('pages.clients.addTemplate')}
          </Button>
        </div>

        {editing && (
          <div
            style={{
              padding: 16,
              marginBottom: 16,
              borderRadius: 8,
              border: '1px solid var(--ant-color-border)',
              background: 'var(--ant-color-fill-quaternary)',
            }}
          >
            <Form layout="vertical">
              <Row gutter={12}>
                <Col xs={24} md={12}>
                  <Form.Item label={t('pages.clients.templateName')} required>
                    <Input
                      value={editing.name}
                      placeholder="Gold 50GB / 30d"
                      onChange={(e) => setEditing({ ...editing, name: e.target.value })}
                    />
                  </Form.Item>
                </Col>
                <Col xs={24} md={12}>
                  <Form.Item label={t('pages.clients.comment')}>
                    <Input
                      value={editing.description}
                      onChange={(e) => setEditing({ ...editing, description: e.target.value })}
                    />
                  </Form.Item>
                </Col>
              </Row>

              <Row gutter={12}>
                <Col xs={12} md={6}>
                  <Form.Item label={t('pages.clients.totalGB')}>
                    <InputNumber
                      min={0}
                      value={editing.totalGB}
                      style={{ width: '100%' }}
                      onChange={(v) => setEditing({ ...editing, totalGB: Number(v) || 0 })}
                    />
                  </Form.Item>
                </Col>
                <Col xs={12} md={6}>
                  <Form.Item label={t('pages.clients.expireDays')}>
                    <InputNumber
                      min={0}
                      value={editing.expireDurationDays}
                      style={{ width: '100%' }}
                      onChange={(v) => setEditing({ ...editing, expireDurationDays: Number(v) || 0 })}
                    />
                  </Form.Item>
                </Col>
                <Col xs={12} md={6}>
                  <Form.Item label={t('pages.clients.limitIp')}>
                    <InputNumber
                      min={0}
                      value={editing.limitIp}
                      style={{ width: '100%' }}
                      onChange={(v) => setEditing({ ...editing, limitIp: Number(v) || 0 })}
                    />
                  </Form.Item>
                </Col>
                <Col xs={12} md={6}>
                  <Form.Item label={t('pages.clients.limitDevice')}>
                    <InputNumber
                      min={0}
                      value={editing.limitDevice}
                      style={{ width: '100%' }}
                      onChange={(v) => setEditing({ ...editing, limitDevice: Number(v) || 0 })}
                    />
                  </Form.Item>
                </Col>
              </Row>

              <Row gutter={12}>
                <Col xs={12} md={6}>
                  <Form.Item label={t('pages.clients.delayedStart')}>
                    <Switch
                      checked={editing.delayedStart}
                      onChange={(v) => setEditing({ ...editing, delayedStart: v })}
                    />
                  </Form.Item>
                </Col>
                {editing.delayedStart && (
                  <Col xs={12} md={6}>
                    <Form.Item label={t('pages.clients.activateBefore')}>
                      <InputNumber
                        min={0}
                        addonAfter="d"
                        value={editing.activateBeforeDays}
                        style={{ width: '100%' }}
                        onChange={(v) => setEditing({ ...editing, activateBeforeDays: Number(v) || 0 })}
                      />
                    </Form.Item>
                  </Col>
                )}
                <Col xs={12} md={6}>
                  <Form.Item label={t('pages.clients.renewDays')}>
                    <InputNumber
                      min={0}
                      value={editing.reset}
                      style={{ width: '100%' }}
                      onChange={(v) => setEditing({ ...editing, reset: Number(v) || 0 })}
                    />
                  </Form.Item>
                </Col>
                <Col xs={12} md={6}>
                  <Form.Item label={t('pages.clients.templateFixedPrice', 'Fixed Price (Reseller)')}>
                    <InputNumber
                      min={0}
                      value={editing.fixedPrice}
                      style={{ width: '100%' }}
                      onChange={(v) => setEditing({ ...editing, fixedPrice: Number(v) || 0 })}
                    />
                  </Form.Item>
                </Col>
                <Col xs={12} md={6}>
                  <Form.Item label={t('pages.clients.group')}>
                    <AutoComplete
                      value={editing.group}
                      options={groups.map((g) => ({ value: g }))}
                      onChange={(v) => setEditing({ ...editing, group: v ?? '' })}
                      allowClear
                    />
                  </Form.Item>
                </Col>
              </Row>

              <Form.Item label={t('pages.clients.attachedInbounds')}>
                <Select
                  mode="multiple"
                  value={editing.inboundIds}
                  options={inboundOptions}
                  onChange={(v) => setEditing({ ...editing, inboundIds: v })}
                  placeholder={t('pages.clients.selectInbound')}
                  maxTagCount="responsive"
                />
              </Form.Item>

              <Space>
                <Button type="primary" loading={saving} onClick={handleSave}>
                  {t('save')}
                </Button>
                <Button onClick={() => setEditing(null)}>{t('cancel')}</Button>
              </Space>
            </Form>
          </div>
        )}

        <Table<UserTemplate>
          rowKey="id"
          size="small"
          loading={loading}
          dataSource={templates}
          pagination={false}
          columns={[
            {
              title: t('pages.clients.templateName'),
              dataIndex: 'name',
              key: 'name',
              render: (v: string, row) => (
                <div>
                  <strong>{v}</strong>
                  {row.description && (
                    <div style={{ fontSize: 12, color: 'var(--ant-color-text-secondary)' }}>
                      {row.description}
                    </div>
                  )}
                </div>
              ),
            },
            {
              title: t('pages.clients.totalGB'),
              dataIndex: 'totalGB',
              key: 'totalGB',
              render: (v: number) => (v > 0 ? SizeFormatter.sizeFormat(v) : t('unlimited')),
            },
            {
              title: t('pages.clients.expireDays'),
              dataIndex: 'expireDurationDays',
              key: 'expireDurationDays',
              render: (v: number, row) => (
                <Space size={4}>
                  <span>{v > 0 ? `${v}d` : t('unlimited')}</span>
                  {row.delayedStart && <Tag color="blue">{t('pages.clients.delayedStart')}</Tag>}
                </Space>
              ),
            },
            {
              title: t('pages.clients.templateFixedPrice', 'Fixed Price'),
              dataIndex: 'fixedPrice',
              key: 'fixedPrice',
              render: (v?: number) => (v && v > 0 ? <Tag color="gold">{v}</Tag> : '-'),
            },
            {
              title: t('pages.clients.group'),
              dataIndex: 'group',
              key: 'group',
              render: (v: string) => (v ? <Tag>{v}</Tag> : '-'),
            },
            {
              title: '',
              key: 'actions',
              width: 90,
              render: (_, row) => (
                <Space size={4}>
                  <Button size="small" icon={<EditOutlined />} onClick={() => startEdit(row)} />
                  <Popconfirm
                    title={t('delete')}
                    onConfirm={() => handleDelete(row.id)}
                  >
                    <Button size="small" danger icon={<DeleteOutlined />} />
                  </Popconfirm>
                </Space>
              ),
            },
          ]}
        />
      </Modal>
    </>
  );
}
