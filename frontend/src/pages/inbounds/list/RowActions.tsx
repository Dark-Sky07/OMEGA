import { useTranslation } from 'react-i18next';
import { Button, Dropdown, type MenuProps } from 'antd';
import {
  MoreOutlined,
  EditOutlined,
  QrcodeOutlined,
  CopyOutlined,
  ExportOutlined,
  RetweetOutlined,
  BlockOutlined,
  DeleteOutlined,
  InfoCircleOutlined,
  TagsOutlined,
  UsergroupAddOutlined,
  UsergroupDeleteOutlined,
} from '@ant-design/icons';

import { isInboundMultiUser, showQrCodeMenu } from './helpers';
import type { DBInboundRecord, RowAction } from './types';

interface RowActionsMenuProps {
  record: DBInboundRecord;
  subEnable: boolean;
  hasClients: boolean;
  onClick: (key: RowAction) => void;
  isMobile?: boolean;
  isReseller?: boolean;
}

function isDividerItem(item: NonNullable<MenuProps['items']>[number]): boolean {
  return typeof item === 'object' && item !== null && 'type' in item && item.type === 'divider';
}

export function buildRowActionsMenu({ record, subEnable, t, isMobile, hasClients, isReseller }: { record: DBInboundRecord; subEnable: boolean; t: (k: string) => string; isMobile?: boolean; hasClients?: boolean; isReseller?: boolean }): MenuProps['items'] {
  const items: MenuProps['items'] = [];
  // Inbounds are read-only for resellers: they can inspect an inbound and
  // manage its clients, but every structural write (edit, reset, delete,
  // clone) is admin-only — the backend answers those with 403.
  if (isMobile && !isReseller) {
    items.push({ key: 'edit', icon: <EditOutlined />, label: t('edit') });
  }
  if (showQrCodeMenu(record)) {
    items.push({ key: 'qrcode', icon: <QrcodeOutlined />, label: t('qrCode') });
  }
  if (isInboundMultiUser(record)) {
    items.push({ key: 'export', icon: <ExportOutlined />, label: t('pages.inbounds.export') });
    if (subEnable) {
      items.push({
        key: 'subs',
        icon: <ExportOutlined />,
        label: `${t('pages.inbounds.export')} — ${t('pages.settings.subSettings')}`,
      });
    }
  } else {
    items.push({ key: 'showInfo', icon: <InfoCircleOutlined />, label: t('pages.inbounds.inboundInfo') });
  }
  items.push({ key: 'clipboard', icon: <CopyOutlined />, label: t('pages.inbounds.exportInbound') });
  if (!isReseller) {
    items.push({ key: 'resetTraffic', icon: <RetweetOutlined />, label: t('pages.inbounds.resetTraffic') });
  }
  if (!isReseller) {
    // Cloning can duplicate a reseller's inbound, which is an admin action.
    items.push({ key: 'clone', icon: <BlockOutlined />, label: t('pages.inbounds.clone') });
  }
  if (isInboundMultiUser(record)) {
    items.push({ key: 'attachExisting', icon: <UsergroupAddOutlined />, label: t('pages.inbounds.attachExistingClients') });
  }
  if (isInboundMultiUser(record) && hasClients) {
    items.push({ key: 'attachClients', icon: <UsergroupAddOutlined />, label: t('pages.inbounds.attachClients') });
    items.push({ key: 'detachClients', icon: <UsergroupDeleteOutlined />, label: t('pages.inbounds.detachClients') });
    if (!isReseller) {
      // Groups are a panel-wide admin feature; the groups API is admin-only.
      items.push({ key: 'addToGroup', icon: <TagsOutlined />, label: t('pages.inbounds.addClientsToGroup') });
    }
    items.push({ type: 'divider' });
    if (!isReseller) {
      items.push({ key: 'delAllClients', icon: <UsergroupDeleteOutlined />, danger: true, label: t('pages.inbounds.delAllClients') });
    }
  } else {
    items.push({ type: 'divider' });
  }
  if (!isReseller) {
    items.push({ key: 'delete', icon: <DeleteOutlined />, danger: true, label: t('delete') });
  }
  // Dropping the trailing writes can leave a dangling divider; trim it so a
  // reseller menu never ends with (or collapses to) an orphan separator.
  while (items.length > 0 && isDividerItem(items[0] as NonNullable<MenuProps['items']>[number])) {
    items.shift();
  }
  while (items.length > 0 && isDividerItem(items[items.length - 1] as NonNullable<MenuProps['items']>[number])) {
    items.pop();
  }
  return items;
}

export function RowActionsCell({ record, subEnable, hasClients, onClick, isReseller }: RowActionsMenuProps) {
  const { t } = useTranslation();
  return (
    <div className="action-buttons">
      {!isReseller && (
        <Button type="text" size="small" style={{ fontSize: 18 }} icon={<EditOutlined />} onClick={() => onClick('edit')} />
      )}
      <Dropdown
        trigger={['click']}
        menu={{
          items: buildRowActionsMenu({ record, subEnable, t, hasClients, isReseller }),
          onClick: ({ key }) => onClick(key as RowAction),
        }}
      >
        <Button type="text" size="small" style={{ fontSize: 18 }} icon={<MoreOutlined />} />
      </Dropdown>
    </div>
  );
}
