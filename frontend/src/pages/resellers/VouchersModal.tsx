import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Col,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Row,
  Space,
  Table,
  Tag,
  message,
} from 'antd';
import { CopyOutlined, DeleteOutlined, GiftOutlined, PlusOutlined } from '@ant-design/icons';
import { ClipboardManager, HttpUtil } from '@/utils';

interface VoucherItem {
  id: number;
  code: string;
  amount: number;
  trafficGB: number;
  maxUses: number;
  usedCount: number;
  expiresAt: number;
  note: string;
  createdAt: number;
}

interface VouchersModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } };

export default function VouchersModal({ open, onOpenChange }: VouchersModalProps) {
  const { t } = useTranslation();
  const [vouchers, setVouchers] = useState<VoucherItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [creating, setCreating] = useState(false);
  const [code, setCode] = useState('');
  const [amount, setAmount] = useState<number>(100);
  const [trafficGB, setTrafficGB] = useState<number>(0);
  const [maxUses, setMaxUses] = useState<number>(1);
  const [note, setNote] = useState('');

  const loadVouchers = useCallback(async () => {
    setLoading(true);
    try {
      const msg = await HttpUtil.get('/panel/api/resellers/vouchers', undefined, { silent: true });
      if (msg?.success && Array.isArray(msg.obj)) {
        setVouchers(msg.obj as VoucherItem[]);
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (open) {
      void loadVouchers();
    }
  }, [open, loadVouchers]);

  async function handleCreate() {
    setCreating(true);
    try {
      const msg = await HttpUtil.post(
        '/panel/api/resellers/vouchers/create',
        {
          code: code.trim(),
          amount: Number(amount) || 0,
          trafficGB: Number(trafficGB) || 0,
          maxUses: Number(maxUses) || 1,
          note: note.trim(),
        },
        JSON_HEADERS as never,
      );
      if (msg?.success) {
        setCode('');
        setNote('');
        await loadVouchers();
      }
    } finally {
      setCreating(false);
    }
  }

  async function handleDelete(id: number) {
    const msg = await HttpUtil.post(`/panel/api/resellers/vouchers/delete/${id}`);
    if (msg?.success) {
      await loadVouchers();
    }
  }

  return (
    <Modal
      open={open}
      title={
        <Space>
          <GiftOutlined />
          <span>{t('resellers.vouchersTitle', 'Reseller Gift / Voucher Codes')}</span>
        </Space>
      }
      width={760}
      onCancel={() => onOpenChange(false)}
      footer={<Button onClick={() => onOpenChange(false)}>{t('close')}</Button>}
    >
      <Form layout="vertical" style={{ marginBottom: 16 }}>
        <Row gutter={10}>
          <Col xs={24} md={6}>
            <Form.Item label={t('resellers.voucherCode', 'Code (optional)')}>
              <Input
                value={code}
                placeholder="Auto: PHOENIX-XXXX"
                onChange={(e) => setCode(e.target.value.toUpperCase())}
              />
            </Form.Item>
          </Col>
          <Col xs={12} md={4}>
            <Form.Item label={t('resellers.voucherAmount', 'Wallet Credit')}>
              <InputNumber
                min={0}
                value={amount}
                style={{ width: '100%' }}
                onChange={(v) => setAmount(Number(v) || 0)}
              />
            </Form.Item>
          </Col>
          <Col xs={12} md={4}>
            <Form.Item label={t('resellers.voucherTrafficGB', 'Bonus GB')}>
              <InputNumber
                min={0}
                value={trafficGB}
                style={{ width: '100%' }}
                onChange={(v) => setTrafficGB(Number(v) || 0)}
              />
            </Form.Item>
          </Col>
          <Col xs={12} md={4}>
            <Form.Item label={t('resellers.voucherMaxUses', 'Max Uses')}>
              <InputNumber
                min={1}
                value={maxUses}
                style={{ width: '100%' }}
                onChange={(v) => setMaxUses(Number(v) || 1)}
              />
            </Form.Item>
          </Col>
          <Col xs={12} md={6}>
            <Form.Item label={t('resellers.comment', 'Note')}>
              <Input value={note} onChange={(e) => setNote(e.target.value)} />
            </Form.Item>
          </Col>
        </Row>
        <Button type="primary" icon={<PlusOutlined />} loading={creating} onClick={handleCreate}>
          {t('resellers.createVoucherBtn', 'Create Voucher Code')}
        </Button>
      </Form>

      <Table<VoucherItem>
        rowKey="id"
        size="small"
        loading={loading}
        dataSource={vouchers}
        pagination={{ pageSize: 8 }}
        columns={[
          {
            title: t('resellers.voucherCode', 'Code'),
            dataIndex: 'code',
            render: (v: string) => (
              <Space size={4}>
                <code>{v}</code>
                <Button
                  size="small"
                  type="text"
                  icon={<CopyOutlined />}
                  onClick={async () => {
                    if (await ClipboardManager.copyText(v)) {
                      message.success(t('copied'));
                    }
                  }}
                />
              </Space>
            ),
          },
          {
            title: t('resellers.voucherAmount', 'Credit'),
            dataIndex: 'amount',
            render: (v: number) => (v > 0 ? <Tag color="green">+{v}</Tag> : '-'),
          },
          {
            title: t('resellers.voucherTrafficGB', 'Traffic GB'),
            dataIndex: 'trafficGB',
            render: (v: number) => (v > 0 ? <Tag color="blue">+{v} GB</Tag> : '-'),
          },
          {
            title: t('resellers.voucherUses', 'Uses'),
            key: 'uses',
            render: (_, r) => `${r.usedCount} / ${r.maxUses}`,
          },
          {
            title: t('resellers.comment', 'Note'),
            dataIndex: 'note',
            render: (v: string) => v || '-',
          },
          {
            title: '',
            key: 'del',
            width: 60,
            render: (_, r) => (
              <Popconfirm title={t('delete')} onConfirm={() => handleDelete(r.id)}>
                <Button size="small" danger icon={<DeleteOutlined />} />
              </Popconfirm>
            ),
          },
        ]}
      />
    </Modal>
  );
}
