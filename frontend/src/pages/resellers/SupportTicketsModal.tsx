import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Form,
  Input,
  Modal,
  Select,
  Space,
  Table,
  Tabs,
  Tag,
  message,
} from 'antd';
import {
  CheckCircleOutlined,
  CustomerServiceOutlined,
  PlusOutlined,
  SendOutlined,
} from '@ant-design/icons';
import { HttpUtil } from '@/utils';

export interface SupportTicketRow {
  id: number;
  resellerId: number;
  resellerUsername: string;
  subject: string;
  status: 'open' | 'answered' | 'closed';
  priority: 'low' | 'normal' | 'high';
  createdAt: number;
  updatedAt: number;
}

export interface SupportTicketMessageRow {
  id: number;
  ticketId: number;
  senderRole: 'reseller' | 'admin';
  senderName: string;
  message: string;
  createdAt: number;
}

export interface PaymentOrderRow {
  id: number;
  orderId: string;
  gateway: string;
  targetType: string;
  targetRef: string;
  amount: number;
  addGb: number;
  addDays: number;
  status: string;
  createdAt: number;
}

interface SupportTicketsModalProps {
  open: boolean;
  mode: 'admin' | 'reseller';
  onOpenChange: (open: boolean) => void;
}

const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } };

export default function SupportTicketsModal({
  open,
  mode,
  onOpenChange,
}: SupportTicketsModalProps) {
  const { t } = useTranslation();
  const [messageApi, messageContextHolder] = message.useMessage();
  const isAdmin = mode === 'admin';
  const baseUrl = isAdmin ? '/panel/api/resellers/tickets' : '/panel/api/reseller/tickets';

  const [tickets, setTickets] = useState<SupportTicketRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [activeTicket, setActiveTicket] = useState<SupportTicketRow | null>(null);
  const [messages, setMessages] = useState<SupportTicketMessageRow[]>([]);
  const [replyText, setReplyText] = useState('');
  const [sendingReply, setSendingReply] = useState(false);

  const [createOpen, setCreateOpen] = useState(false);
  const [newSubject, setNewSubject] = useState('');
  const [newPriority, setNewPriority] = useState<'low' | 'normal' | 'high'>('normal');
  const [newMessage, setNewMessage] = useState('');
  const [creating, setCreating] = useState(false);

  const [orders, setOrders] = useState<PaymentOrderRow[]>([]);
  const [ordersLoading, setOrdersLoading] = useState(false);

  const loadTickets = useCallback(async () => {
    setLoading(true);
    try {
      const res = await HttpUtil.get(baseUrl);
      if (res?.success && Array.isArray(res.obj)) {
        setTickets(res.obj as SupportTicketRow[]);
      }
    } finally {
      setLoading(false);
    }
  }, [baseUrl]);

  const loadOrders = useCallback(async () => {
    if (!isAdmin) return;
    setOrdersLoading(true);
    try {
      const res = await HttpUtil.get('/panel/api/resellers/orders');
      if (res?.success && Array.isArray(res.obj)) {
        setOrders(res.obj as PaymentOrderRow[]);
      }
    } finally {
      setOrdersLoading(false);
    }
  }, [isAdmin]);

  useEffect(() => {
    if (open) {
      setActiveTicket(null);
      void loadTickets();
      if (isAdmin) void loadOrders();
    }
  }, [open, isAdmin, loadTickets, loadOrders]);

  async function openThread(row: SupportTicketRow) {
    const res = await HttpUtil.get(`${baseUrl}/${row.id}`);
    if (res?.success && res.obj) {
      const data = res.obj as { ticket: SupportTicketRow; messages: SupportTicketMessageRow[] };
      setActiveTicket(data.ticket);
      setMessages(Array.isArray(data.messages) ? data.messages : []);
      setReplyText('');
    }
  }

  async function handleCreateTicket() {
    if (!newSubject.trim() || !newMessage.trim()) {
      messageApi.error(t('resellers.ticketSubjectAndMessageRequired') || 'Subject and message are required');
      return;
    }
    setCreating(true);
    try {
      const res = await HttpUtil.post(
        '/panel/api/reseller/tickets/create',
        {
          subject: newSubject.trim(),
          priority: newPriority,
          message: newMessage.trim(),
        },
        JSON_HEADERS,
      );
      if (res?.success) {
        setCreateOpen(false);
        setNewSubject('');
        setNewMessage('');
        await loadTickets();
      }
    } finally {
      setCreating(false);
    }
  }

  async function handleReply() {
    if (!activeTicket || !replyText.trim()) return;
    setSendingReply(true);
    try {
      const res = await HttpUtil.post(
        `${baseUrl}/${activeTicket.id}/reply`,
        { message: replyText.trim() },
        JSON_HEADERS,
      );
      if (res?.success) {
        setReplyText('');
        await openThread(activeTicket);
        await loadTickets();
      }
    } finally {
      setSendingReply(false);
    }
  }

  async function handleCloseTicket(ticketId: number) {
    const res = await HttpUtil.post(`${baseUrl}/${ticketId}/close`);
    if (res?.success) {
      if (activeTicket?.id === ticketId) {
        setActiveTicket({ ...activeTicket, status: 'closed' });
      }
      await loadTickets();
    }
  }

  const ticketsPane = (
    <>
      {!activeTicket ? (
        <>
          <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 12 }}>
            <span style={{ color: 'var(--ant-color-text-secondary)', fontSize: 13 }}>
              {t('resellers.ticketsDesc') || 'Direct support communication between Resellers and Panel Admins'}
            </span>
            {!isAdmin && (
              <Button type="primary" size="small" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
                {t('resellers.newTicket') || 'New Ticket'}
              </Button>
            )}
          </div>

          {createOpen && !isAdmin && (
            <div
              style={{
                padding: 12,
                marginBottom: 12,
                borderRadius: 8,
                border: '1px solid var(--ant-color-border)',
              }}
            >
              <Form layout="vertical">
                <Form.Item label={t('resellers.ticketSubject') || 'Subject'} required>
                  <Input value={newSubject} onChange={(e) => setNewSubject(e.target.value)} />
                </Form.Item>
                <Form.Item label={t('resellers.ticketPriority') || 'Priority'}>
                  <Select
                    value={newPriority}
                    onChange={(v) => setNewPriority(v)}
                    options={[
                      { value: 'low', label: 'Low' },
                      { value: 'normal', label: 'Normal' },
                      { value: 'high', label: 'High' },
                    ]}
                  />
                </Form.Item>
                <Form.Item label={t('resellers.ticketMessage') || 'Message'} required>
                  <Input.TextArea rows={3} value={newMessage} onChange={(e) => setNewMessage(e.target.value)} />
                </Form.Item>
                <Space>
                  <Button type="primary" loading={creating} onClick={handleCreateTicket}>
                    {t('create')}
                  </Button>
                  <Button onClick={() => setCreateOpen(false)}>{t('cancel')}</Button>
                </Space>
              </Form>
            </div>
          )}

          <Table<SupportTicketRow>
            rowKey="id"
            size="small"
            loading={loading}
            dataSource={tickets}
            pagination={{ pageSize: 10 }}
            columns={[
              {
                title: '#',
                dataIndex: 'id',
                width: 60,
              },
              ...(isAdmin
                ? [
                    {
                      title: t('username'),
                      dataIndex: 'resellerUsername',
                      key: 'resellerUsername',
                      render: (v: string) => <Tag color="blue">{v}</Tag>,
                    },
                  ]
                : []),
              {
                title: t('resellers.ticketSubject') || 'Subject',
                dataIndex: 'subject',
                key: 'subject',
                render: (v: string, row: SupportTicketRow) => (
                  <a onClick={() => void openThread(row)}>{v}</a>
                ),
              },
              {
                title: t('status') || 'Status',
                dataIndex: 'status',
                key: 'status',
                width: 110,
                render: (v: string) => {
                  const color = v === 'answered' ? 'green' : v === 'closed' ? 'default' : 'gold';
                  return <Tag color={color}>{v}</Tag>;
                },
              },
              {
                title: '',
                key: 'actions',
                width: 120,
                render: (_, row) => (
                  <Space size={4}>
                    <Button size="small" onClick={() => void openThread(row)}>
                      {t('open') || 'Open'}
                    </Button>
                    {row.status !== 'closed' && (
                      <Button
                        size="small"
                        icon={<CheckCircleOutlined />}
                        onClick={() => void handleCloseTicket(row.id)}
                      />
                    )}
                  </Space>
                ),
              },
            ]}
          />
        </>
      ) : (
        <div>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
            <Space>
              <Button size="small" onClick={() => setActiveTicket(null)}>
                ← {t('back') || 'Back'}
              </Button>
              <strong>#{activeTicket.id} — {activeTicket.subject}</strong>
              <Tag color={activeTicket.status === 'answered' ? 'green' : activeTicket.status === 'closed' ? 'default' : 'gold'}>
                {activeTicket.status}
              </Tag>
            </Space>
            {activeTicket.status !== 'closed' && (
              <Button size="small" onClick={() => void handleCloseTicket(activeTicket.id)}>
                {t('close')}
              </Button>
            )}
          </div>

          <div
            style={{
              maxHeight: 320,
              overflowY: 'auto',
              padding: 12,
              marginBottom: 12,
              borderRadius: 8,
              border: '1px solid var(--ant-color-border)',
            }}
          >
            {messages.map((m) => (
              <div
                key={m.id}
                style={{
                  marginBottom: 10,
                  padding: '8px 12px',
                  borderRadius: 8,
                  background:
                    m.senderRole === 'admin'
                      ? 'var(--ant-color-primary-bg)'
                      : 'var(--ant-color-fill-quaternary)',
                }}
              >
                <div style={{ fontSize: 12, color: 'var(--ant-color-text-secondary)', marginBottom: 4 }}>
                  <Tag color={m.senderRole === 'admin' ? 'gold' : 'blue'}>{m.senderName}</Tag>
                  {new Date(m.createdAt).toLocaleString()}
                </div>
                <div style={{ whiteSpace: 'pre-wrap' }}>{m.message}</div>
              </div>
            ))}
          </div>

          {activeTicket.status !== 'closed' && (
            <Space.Compact style={{ display: 'flex' }}>
              <Input
                value={replyText}
                placeholder={t('resellers.ticketReplyPlaceholder') || 'Write a reply...'}
                onChange={(e) => setReplyText(e.target.value)}
                onPressEnter={() => void handleReply()}
              />
              <Button
                type="primary"
                icon={<SendOutlined />}
                loading={sendingReply}
                onClick={() => void handleReply()}
              >
                {t('send') || 'Send'}
              </Button>
            </Space.Compact>
          )}
        </div>
      )}
    </>
  );

  return (
    <>
      {messageContextHolder}
      <Modal
        open={open}
        title={
          <Space>
            <CustomerServiceOutlined />
            <span>{t('resellers.supportTickets') || 'Support Tickets'}</span>
          </Space>
        }
        width={760}
        onCancel={() => onOpenChange(false)}
        footer={
          <Button onClick={() => onOpenChange(false)}>
            {t('close')}
          </Button>
        }
      >
        {isAdmin ? (
          <Tabs
            items={[
              {
                key: 'tickets',
                label: t('resellers.supportTickets') || 'Support Tickets',
                children: ticketsPane,
              },
              {
                key: 'orders',
                label: t('resellers.paymentOrders') || 'Payment Webhook Orders',
                children: (
                  <>
                    <div style={{ marginBottom: 8, fontSize: 12, color: 'var(--ant-color-text-secondary)' }}>
                      Webhook Endpoint: <code>POST /api/payment/webhook</code> (Header: <code>X-Omega-Signature: sha256=&lt;hmac&gt;</code>)
                    </div>
                    <Table<PaymentOrderRow>
                      rowKey="id"
                      size="small"
                      loading={ordersLoading}
                      dataSource={orders}
                      pagination={{ pageSize: 10 }}
                      columns={[
                        { title: 'Order ID', dataIndex: 'orderId', key: 'orderId' },
                        { title: 'Gateway', dataIndex: 'gateway', key: 'gateway' },
                        { title: 'Type', dataIndex: 'targetType', key: 'targetType', render: (v: string) => <Tag>{v}</Tag> },
                        { title: 'Target', dataIndex: 'targetRef', key: 'targetRef' },
                        { title: 'Amount', dataIndex: 'amount', key: 'amount' },
                        {
                          title: 'Date',
                          dataIndex: 'createdAt',
                          key: 'createdAt',
                          render: (v: number) => (v ? new Date(v).toLocaleString() : '-'),
                        },
                      ]}
                    />
                  </>
                ),
              },
            ]}
          />
        ) : (
          ticketsPane
        )}
      </Modal>
    </>
  );
}
