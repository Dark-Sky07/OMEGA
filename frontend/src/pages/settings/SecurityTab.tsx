import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Empty,
  Form,
  Input,
  InputNumber,
  Modal,
  Select,
  Space,
  Spin,
  Switch,
  Tabs,
  Tag,
  message,
} from 'antd';
import { ApiOutlined, SafetyOutlined, TeamOutlined, UserOutlined } from '@ant-design/icons';
import { ClipboardManager, HttpUtil, RandomUtil } from '@/utils';
import type { AllSetting } from '@/models/setting';
import { SettingListItem } from '@/components/ui';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { catTabLabel } from './catTabLabel';
import TwoFactorModal from './TwoFactorModal';
import './SecurityTab.css';

interface ApiMsg<T = unknown> {
  success?: boolean;
  msg?: string;
  obj?: T;
}

interface ApiTokenRow {
  id: number;
  name: string;
  prefix?: string;
  scope?: string;
  enabled: boolean;
  expiresAt?: number;
  lastUsedAt?: number;
  createdAt: number;
}

interface AdminRow {
  id: number;
  username: string;
  role: 'superadmin' | 'admin' | 'support' | 'viewer';
  permissions: string;
  disabled: boolean;
  note: string;
  createdAt?: number;
  lastLoginAt?: number;
}

const ADMIN_MODULES = [
  'inbounds',
  'clients',
  'resellers',
  'nodes',
  'xray',
  'settings',
  'server',
] as const;

interface SecurityTabProps {
  allSetting: AllSetting;
  updateSetting: (patch: Partial<AllSetting>) => void;
}

type TfaType = 'set' | 'confirm';

interface TfaState {
  open: boolean;
  title: string;
  description: string;
  token: string;
  type: TfaType;
  onConfirm: (success: boolean, code?: string) => void;
}

const TFA_INITIAL: TfaState = {
  open: false,
  title: '',
  description: '',
  token: '',
  type: 'set',
  onConfirm: () => {},
};

export default function SecurityTab({ allSetting, updateSetting }: SecurityTabProps) {
  const { t } = useTranslation();
  const { isMobile } = useMediaQuery();
  const [modal, modalContextHolder] = Modal.useModal();
  const [messageApi, messageContextHolder] = message.useMessage();

  const [tfa, setTfa] = useState<TfaState>(TFA_INITIAL);
  const [user, setUser] = useState({
    oldUsername: '',
    oldPassword: '',
    newUsername: '',
    newPassword: '',
  });
  const [updating, setUpdating] = useState(false);

  const [apiTokens, setApiTokens] = useState<ApiTokenRow[]>([]);
  const [apiTokensLoading, setApiTokensLoading] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [createName, setCreateName] = useState('');
  const [createScope, setCreateScope] = useState<'full' | 'read'>('full');
  const [createExpireDays, setCreateExpireDays] = useState<number>(0);
  const [creating, setCreating] = useState(false);
  const [createdToken, setCreatedToken] = useState<{ name: string; token: string } | null>(null);

  const [admins, setAdmins] = useState<AdminRow[]>([]);
  const [adminsLoading, setAdminsLoading] = useState(false);
  const [adminModalOpen, setAdminModalOpen] = useState(false);
  const [adminSaving, setAdminSaving] = useState(false);
  const [editingAdmin, setEditingAdmin] = useState<{
    id: number;
    username: string;
    password: string;
    role: 'superadmin' | 'admin' | 'support' | 'viewer';
    permissions: string[];
    note: string;
    disabled: boolean;
  }>({
    id: 0,
    username: '',
    password: '',
    role: 'support',
    permissions: ['inbounds', 'clients'],
    note: '',
    disabled: false,
  });

  const openTfa = useCallback((opts: Omit<TfaState, 'open'>) => {
    setTfa({ ...opts, open: true });
  }, []);

  const onTfaConfirm = useCallback((success: boolean, code?: string) => {
    tfa.onConfirm(success, code);
  }, [tfa]);

  function updateUserField<K extends keyof typeof user>(key: K, value: string) {
    setUser((prev) => ({ ...prev, [key]: value }));
  }

  const sendUpdateUser = useCallback(async () => {
    setUpdating(true);
    try {
      const msg = await HttpUtil.post('/panel/api/setting/updateUser', user) as ApiMsg;
      if (msg?.success) {
        await HttpUtil.post('/logout');
        const basePath = window.X_UI_BASE_PATH || '/';
        window.location.replace(basePath);
      }
    } finally {
      setUpdating(false);
    }
  }, [user]);

  function onUpdateUserClick() {
    if (allSetting.twoFactorEnable) {
      openTfa({
        title: t('pages.settings.security.twoFactorModalChangeCredentialsTitle'),
        description: t('pages.settings.security.twoFactorModalChangeCredentialsStep'),
        token: allSetting.twoFactorToken,
        type: 'confirm',
        onConfirm: (ok: boolean) => { if (ok) sendUpdateUser(); },
      });
    } else {
      sendUpdateUser();
    }
  }

  const loadApiTokens = useCallback(async () => {
    setApiTokensLoading(true);
    try {
      const msg = await HttpUtil.get('/panel/api/setting/apiTokens') as ApiMsg<ApiTokenRow[]>;
      if (msg?.success) setApiTokens(Array.isArray(msg.obj) ? msg.obj : []);
    } finally {
      setApiTokensLoading(false);
    }
  }, []);

  useEffect(() => {
     
    loadApiTokens();
  }, [loadApiTokens]);

  const loadAdmins = useCallback(async () => {
    setAdminsLoading(true);
    try {
      const msg = await HttpUtil.get('/panel/api/setting/admins') as ApiMsg<AdminRow[]>;
      if (msg?.success && Array.isArray(msg.obj)) {
        setAdmins(msg.obj);
      }
    } finally {
      setAdminsLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadAdmins();
  }, [loadAdmins]);

  function parseAdminPermissions(raw: string): string[] {
    if (!raw) return [];
    try {
      const parsed = JSON.parse(raw);
      return Array.isArray(parsed) ? parsed.map(String) : [];
    } catch {
      return raw.split(',').map((s) => s.trim()).filter(Boolean);
    }
  }

  function openNewAdminModal() {
    setEditingAdmin({
      id: 0,
      username: '',
      password: '',
      role: 'support',
      permissions: ['inbounds', 'clients'],
      note: '',
      disabled: false,
    });
    setAdminModalOpen(true);
  }

  function openEditAdminModal(row: AdminRow) {
    setEditingAdmin({
      id: row.id,
      username: row.username,
      password: '',
      role: (row.role || 'superadmin') as 'superadmin' | 'admin' | 'support' | 'viewer',
      permissions: parseAdminPermissions(row.permissions),
      note: row.note || '',
      disabled: !!row.disabled,
    });
    setAdminModalOpen(true);
  }

  async function saveAdminModal() {
    if (!editingAdmin.username.trim()) {
      messageApi.error(t('pages.settings.security.adminUsernameRequired') || 'Username is required');
      return;
    }
    if (editingAdmin.id === 0 && editingAdmin.password.length < 4) {
      messageApi.error(t('pages.settings.security.adminPasswordMin') || 'Password must be at least 4 characters');
      return;
    }
    setAdminSaving(true);
    try {
      const url = editingAdmin.id > 0
        ? `/panel/api/setting/admins/update/${editingAdmin.id}`
        : '/panel/api/setting/admins/create';
      const msg = await HttpUtil.post(
        url,
        {
          username: editingAdmin.username.trim(),
          password: editingAdmin.password,
          role: editingAdmin.role,
          permissions: editingAdmin.permissions,
          note: editingAdmin.note.trim(),
          disabled: editingAdmin.disabled,
        },
        { headers: { 'Content-Type': 'application/json' } },
      ) as ApiMsg;
      if (msg?.success) {
        setAdminModalOpen(false);
        await loadAdmins();
      }
    } finally {
      setAdminSaving(false);
    }
  }

  function confirmDeleteAdmin(row: AdminRow) {
    modal.confirm({
      title: `${t('delete')} "${row.username}"?`,
      okText: t('delete'),
      cancelText: t('cancel'),
      okType: 'danger',
      onOk: async () => {
        const msg = await HttpUtil.post(`/panel/api/setting/admins/delete/${row.id}`) as ApiMsg;
        if (msg?.success) await loadAdmins();
      },
    });
  }

  async function copyToken(token: string) {
    if (!token) return;
    const ok = await ClipboardManager.copyText(token);
    if (ok) messageApi.success(t('copySuccess'));
    else messageApi.error(t('copyFail') ?? 'Copy failed');
  }

  function openCreateModal() {
    setCreateName('');
    setCreateScope('full');
    setCreateExpireDays(0);
    setCreateOpen(true);
  }

  async function confirmCreateToken() {
    const name = createName.trim();
    if (!name) {
      messageApi.error(t('pages.settings.security.apiTokenNameRequired') || 'Name is required');
      return;
    }
    setCreating(true);
    try {
      const msg = await HttpUtil.post('/panel/api/setting/apiTokens/create', {
        name,
        scope: createScope,
        expireDays: createExpireDays || 0,
      }) as ApiMsg<{ token?: string }>;
      if (msg?.success) {
        setCreateOpen(false);
        await loadApiTokens();
        if (msg.obj?.token) {
          setCreatedToken({ name, token: msg.obj.token });
        }
      }
    } finally {
      setCreating(false);
    }
  }

  function confirmDeleteToken(row: ApiTokenRow) {
    modal.confirm({
      title: `${t('delete')} "${row.name}"?`,
      content: t('pages.settings.security.apiTokenDeleteWarning')
        || 'Any caller using this token will stop authenticating immediately.',
      okText: t('delete'),
      cancelText: t('cancel'),
      okType: 'danger',
      onOk: async () => {
        const msg = await HttpUtil.post(`/panel/api/setting/apiTokens/delete/${row.id}`) as ApiMsg;
        if (msg?.success) await loadApiTokens();
      },
    });
  }

  async function toggleTokenEnabled(row: ApiTokenRow) {
    const target = !row.enabled;
    const msg = await HttpUtil.post(`/panel/api/setting/apiTokens/setEnabled/${row.id}`, { enabled: target }) as ApiMsg;
    if (msg?.success) {
      setApiTokens((prev) => prev.map((r) => (r.id === row.id ? { ...r, enabled: target } : r)));
    }
  }

  function formatTokenDate(ts?: number): string {
    if (!ts) return '';
    // Model timestamps are unix milliseconds; handle legacy second-precision values gracefully.
    const ms = ts < 1e12 ? ts * 1000 : ts;
    return new Date(ms).toLocaleString();
  }

  function isTokenExpired(row: ApiTokenRow): boolean {
    return !!row.expiresAt && row.expiresAt > 0 && Date.now() > row.expiresAt;
  }

  function toggleTwoFactor() {
    if (!allSetting.twoFactorEnable) {
      const newToken = RandomUtil.randomBase32String();
      openTfa({
        title: t('pages.settings.security.twoFactorModalSetTitle'),
        description: '',
        token: newToken,
        type: 'set',
        onConfirm: (ok: boolean) => {
          if (ok) {
            messageApi.success(t('pages.settings.security.twoFactorModalSetSuccess'));
            updateSetting({ twoFactorToken: newToken, twoFactorEnable: true });
          } else {
            updateSetting({ twoFactorEnable: false });
          }
        },
      });
    } else {
      openTfa({
        title: t('pages.settings.security.twoFactorModalDeleteTitle'),
        description: t('pages.settings.security.twoFactorModalRemoveStep'),
        token: allSetting.twoFactorToken,
        type: 'confirm',
        onConfirm: (ok: boolean) => {
          if (!ok) return;
          messageApi.success(t('pages.settings.security.twoFactorModalDeleteSuccess'));
          updateSetting({ twoFactorEnable: false, twoFactorToken: '' });
        },
      });
    }
  }

  return (
    <>
      {messageContextHolder}
      {modalContextHolder}
      <Tabs defaultActiveKey="1" items={[
        {
          key: '1',
          label: catTabLabel(<UserOutlined />, t('pages.settings.security.admin'), isMobile),
          children: (
            <>
              <SettingListItem paddings="small" title={t('pages.settings.oldUsername')}>
                <Input value={user.oldUsername} autoComplete="username"
                  onChange={(e) => updateUserField('oldUsername', e.target.value)} />
              </SettingListItem>
              <SettingListItem paddings="small" title={t('pages.settings.currentPassword')}>
                <Input.Password value={user.oldPassword} autoComplete="current-password"
                  onChange={(e) => updateUserField('oldPassword', e.target.value)} />
              </SettingListItem>
              <SettingListItem paddings="small" title={t('pages.settings.newUsername')}>
                <Input value={user.newUsername}
                  onChange={(e) => updateUserField('newUsername', e.target.value)} />
              </SettingListItem>
              <SettingListItem paddings="small" title={t('pages.settings.newPassword')}>
                <Input.Password value={user.newPassword} autoComplete="new-password"
                  onChange={(e) => updateUserField('newPassword', e.target.value)} />
              </SettingListItem>
              <div className="security-actions">
                <Space style={{ padding: '0 20px' }}>
                  <Button type="primary" loading={updating} onClick={onUpdateUserClick}>
                    {t('confirm')}
                  </Button>
                </Space>
              </div>
            </>
          ),
        },
        {
          key: '2',
          label: catTabLabel(<SafetyOutlined />, t('pages.settings.security.twoFactor'), isMobile),
          children: (
            <SettingListItem
              paddings="small"
              title={t('pages.settings.security.twoFactorEnable')}
              description={t('pages.settings.security.twoFactorEnableDesc')}
            >
              <Switch checked={allSetting.twoFactorEnable} onClick={toggleTwoFactor} />
            </SettingListItem>
          ),
        },
        {
          key: '3',
          label: catTabLabel(<ApiOutlined />, t('pages.nodes.apiToken'), isMobile),
          children: (
            <div className="api-token-section">
              <div className="api-token-header">
                <p className="api-token-hint">{t('pages.nodes.apiTokenHint')}</p>
                <Button type="primary" size="small" onClick={openCreateModal}>
                  + {t('pages.settings.security.apiTokenNew') || 'New token'}
                </Button>
              </div>
              <Spin spinning={apiTokensLoading}>
                {!apiTokens.length && !apiTokensLoading && (
                  <Empty description={t('pages.settings.security.apiTokenEmpty') || 'No tokens yet'} />
                )}
                {apiTokens.map((row) => {
                  const expired = isTokenExpired(row);
                  const readOnly = row.scope === 'read';
                  return (
                    <div key={row.id} className={`api-token-row${row.enabled && !expired ? '' : ' disabled'}`}>
                      <div className="api-token-row-head">
                        <div className="api-token-name-wrap">
                          <Space size={6} wrap>
                            <span className="api-token-name">{row.name}</span>
                            {row.prefix ? <code>{row.prefix}…</code> : null}
                            <Tag color={readOnly ? 'blue' : 'gold'}>
                              {readOnly
                                ? (t('pages.settings.security.apiTokenScopeRead') || 'Read-only')
                                : (t('pages.settings.security.apiTokenScopeFull') || 'Full access')}
                            </Tag>
                            {expired ? (
                              <Tag color="red">{t('pages.settings.security.apiTokenExpired') || 'Expired'}</Tag>
                            ) : null}
                          </Space>
                          <span className="api-token-created">
                            {formatTokenDate(row.createdAt)}
                            {row.expiresAt ? ` · ${t('pages.settings.security.apiTokenExpires') || 'Expires'}: ${formatTokenDate(row.expiresAt)}` : ''}
                            {row.lastUsedAt ? ` · ${t('pages.settings.security.apiTokenLastUsed') || 'Last used'}: ${formatTokenDate(row.lastUsedAt)}` : ''}
                          </span>
                        </div>
                        <div className="api-token-actions">
                          <Switch size="small" checked={row.enabled} onChange={() => toggleTokenEnabled(row)} />
                          <Button size="small" danger type="text" onClick={() => confirmDeleteToken(row)}>
                            {t('delete')}
                          </Button>
                        </div>
                      </div>
                    </div>
                  );
                })}
              </Spin>
            </div>
          ),
        },
        {
          key: '4',
          label: catTabLabel(<TeamOutlined />, t('pages.settings.security.multiAdminTitle') || 'Admins & RBAC', isMobile),
          children: (
            <div className="api-token-section">
              <div className="api-token-header">
                <p className="api-token-hint">
                  {t('pages.settings.security.multiAdminDesc')
                    || 'Create secondary admin accounts with granular RBAC roles (Superadmin, Custom Admin, Support, or Read-only Viewer).'}
                </p>
                <Button type="primary" size="small" onClick={openNewAdminModal}>
                  + {t('pages.settings.security.addAdmin') || 'Add Admin'}
                </Button>
              </div>
              <Spin spinning={adminsLoading}>
                {admins.map((row) => {
                  const roleColor =
                    row.role === 'superadmin'
                      ? 'gold'
                      : row.role === 'admin'
                        ? 'purple'
                        : row.role === 'support'
                          ? 'cyan'
                          : 'blue';
                  const perms = parseAdminPermissions(row.permissions);
                  return (
                    <div key={row.id} className={`api-token-row${row.disabled ? ' disabled' : ''}`}>
                      <div className="api-token-row-head">
                        <div className="api-token-name-wrap">
                          <Space size={6} wrap>
                            <span className="api-token-name">{row.username}</span>
                            {row.id === 1 && <Tag color="gold">Primary</Tag>}
                            <Tag color={roleColor}>
                              {t(`pages.settings.security.role_${row.role || 'superadmin'}`) || row.role}
                            </Tag>
                            {row.role === 'admin' && perms.map((p) => (
                              <Tag key={p}>{p}</Tag>
                            ))}
                            {row.disabled && <Tag color="red">{t('disabled') || 'Disabled'}</Tag>}
                          </Space>
                          <span className="api-token-created">
                            {row.note ? `${row.note} · ` : ''}
                            {row.lastLoginAt
                              ? `${t('pages.settings.security.lastLogin') || 'Last login'}: ${formatTokenDate(row.lastLoginAt)}`
                              : ''}
                          </span>
                        </div>
                        <div className="api-token-actions">
                          <Button size="small" type="text" onClick={() => openEditAdminModal(row)}>
                            {t('edit') || 'Edit'}
                          </Button>
                          {row.id !== 1 && (
                            <Button size="small" danger type="text" onClick={() => confirmDeleteAdmin(row)}>
                              {t('delete')}
                            </Button>
                          )}
                        </div>
                      </div>
                    </div>
                  );
                })}
              </Spin>
            </div>
          ),
        },
      ]} />

      <Modal
        open={adminModalOpen}
        title={
          editingAdmin.id > 0
            ? (t('pages.settings.security.editAdmin') || 'Edit Admin Account')
            : (t('pages.settings.security.addAdmin') || 'Add Admin Account')
        }
        confirmLoading={adminSaving}
        okText={t('save')}
        cancelText={t('cancel')}
        onOk={saveAdminModal}
        onCancel={() => setAdminModalOpen(false)}
      >
        <Form layout="vertical">
          <Form.Item label={t('pages.settings.newUsername') || 'Username'} required>
            <Input
              value={editingAdmin.username}
              onChange={(e) => setEditingAdmin((prev) => ({ ...prev, username: e.target.value }))}
            />
          </Form.Item>
          <Form.Item
            label={t('pages.settings.newPassword') || 'Password'}
            required={editingAdmin.id === 0}
            extra={
              editingAdmin.id > 0
                ? (t('pages.settings.security.adminPasswordLeaveBlank') || 'Leave blank to keep existing password')
                : undefined
            }
          >
            <Input.Password
              value={editingAdmin.password}
              autoComplete="new-password"
              onChange={(e) => setEditingAdmin((prev) => ({ ...prev, password: e.target.value }))}
            />
          </Form.Item>
          <Form.Item label={t('pages.settings.security.adminRole') || 'Role'}>
            <Select
              disabled={editingAdmin.id === 1}
              value={editingAdmin.role}
              onChange={(v) => setEditingAdmin((prev) => ({ ...prev, role: v }))}
              options={[
                { value: 'superadmin', label: t('pages.settings.security.role_superadmin') || 'Superadmin (Full Access)' },
                { value: 'admin', label: t('pages.settings.security.role_admin') || 'Custom Admin (Selected Modules)' },
                { value: 'support', label: t('pages.settings.security.role_support') || 'Support (Clients Read/Write + Inbounds Read)' },
                { value: 'viewer', label: t('pages.settings.security.role_viewer') || 'Viewer (Read-only Auditor)' },
              ]}
            />
          </Form.Item>
          {editingAdmin.role === 'admin' && (
            <Form.Item label={t('pages.settings.security.adminModules') || 'Allowed Modules'}>
              <Select
                mode="multiple"
                value={editingAdmin.permissions}
                onChange={(v) => setEditingAdmin((prev) => ({ ...prev, permissions: v }))}
                options={ADMIN_MODULES.map((m) => ({ value: m, label: m }))}
              />
            </Form.Item>
          )}
          <Form.Item label={t('pages.clients.comment') || 'Note'}>
            <Input
              value={editingAdmin.note}
              onChange={(e) => setEditingAdmin((prev) => ({ ...prev, note: e.target.value }))}
            />
          </Form.Item>
          {editingAdmin.id > 1 && (
            <Form.Item label={t('enable') || 'Enabled'}>
              <Switch
                checked={!editingAdmin.disabled}
                onChange={(v) => setEditingAdmin((prev) => ({ ...prev, disabled: !v }))}
              />
            </Form.Item>
          )}
        </Form>
      </Modal>

      <Modal
        open={createOpen}
        title={t('pages.settings.security.apiTokenNew') || 'New API token'}
        confirmLoading={creating}
        okText={t('confirm')}
        cancelText={t('cancel')}
        onOk={confirmCreateToken}
        onCancel={() => setCreateOpen(false)}
      >
        <Form layout="vertical">
          <Form.Item label={t('pages.settings.security.apiTokenName') || 'Name'} required>
            <Input
              value={createName}
              maxLength={64}
              placeholder={t('pages.settings.security.apiTokenNamePlaceholder') || 'e.g. central-panel-a'}
              onChange={(e) => setCreateName(e.target.value)}
              onPressEnter={confirmCreateToken}
            />
          </Form.Item>
          <Form.Item label={t('pages.settings.security.apiTokenScope') || 'Permissions'}>
            <Select
              value={createScope}
              onChange={(v) => setCreateScope(v)}
              options={[
                {
                  value: 'full',
                  label: t('pages.settings.security.apiTokenScopeFull') || 'Full access (read & write)',
                },
                {
                  value: 'read',
                  label: t('pages.settings.security.apiTokenScopeRead') || 'Read-only (GET only)',
                },
              ]}
            />
          </Form.Item>
          <Form.Item
            label={t('pages.settings.security.apiTokenExpireDays') || 'Expires in (days)'}
            extra={t('pages.settings.security.apiTokenExpireDaysDesc') || '0 means never expires.'}
          >
            <InputNumber
              min={0}
              max={3650}
              style={{ width: '100%' }}
              value={createExpireDays}
              onChange={(v) => setCreateExpireDays(typeof v === 'number' ? v : 0)}
            />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        open={!!createdToken}
        title={t('pages.settings.security.apiTokenCreatedTitle') || 'Token created'}
        okText={t('done')}
        onOk={() => setCreatedToken(null)}
        onCancel={() => setCreatedToken(null)}
        cancelButtonProps={{ style: { display: 'none' } }}
      >
        <p className="api-token-created-notice">
          {t('pages.settings.security.apiTokenCreatedNotice')
            || 'Copy this token now. For security it is not stored in readable form and will not be shown again.'}
        </p>
        <div className="api-token-value-wrap">
          <code className="api-token-value">{createdToken?.token}</code>
          <Button size="small" type="primary" onClick={() => createdToken && copyToken(createdToken.token)}>
            {t('copy')}
          </Button>
        </div>
      </Modal>

      <TwoFactorModal
        open={tfa.open}
        title={tfa.title}
        description={tfa.description}
        token={tfa.token}
        type={tfa.type}
        onConfirm={onTfaConfirm}
        onOpenChange={(open) => setTfa((prev) => ({ ...prev, open }))}
      />
    </>
  );
}
