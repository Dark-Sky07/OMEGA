import { useMemo, type ReactElement } from 'react';
import { useTranslation } from 'react-i18next';
import { Popover, Switch, Tag, Tooltip, type TableColumnType } from 'antd';
import { TeamOutlined } from '@ant-design/icons';

import { IntlUtil, ColorUtils } from '@/utils';
import { InfinityIcon } from '@/components/ui';
import ClientTrafficCell from '@/components/clients/ClientTrafficCell';
import { useDatepicker } from '@/hooks/useDatepicker';
import type { NodeRecord } from '@/api/queries/useNodesQuery';

import { RowActionsCell } from './RowActions';
import {
  readStreamHints,
  networkLabel,
  networkL4,
  shadowsocksNetworkLabel,
  tunnelNetworkLabel,
  mixedNetworkLabel,
} from './helpers';
import type { ClientCountEntry, DBInboundRecord, RowAction } from './types';

interface UseInboundColumnsParams {
  hasAnyRemark: boolean;
  hasAnySubSortIndex: boolean;
  hasActiveNode: boolean;
  nodesById: Map<number, NodeRecord>;
  clientCount: Record<number, ClientCountEntry>;
  subEnable: boolean;
  expireDiff: number;
  trafficDiff: number;
  onRowAction: (action: { key: RowAction; dbInbound: DBInboundRecord }) => void;
  onSwitchEnable: (dbInbound: DBInboundRecord, next: boolean) => void;
  isReseller?: boolean;
}

export function useInboundColumns({
  hasAnyRemark,
  hasAnySubSortIndex,
  hasActiveNode,
  nodesById,
  clientCount,
  subEnable,
  expireDiff,
  trafficDiff,
  onRowAction,
  onSwitchEnable,
  isReseller,
}: UseInboundColumnsParams): TableColumnType<DBInboundRecord>[] {
  const { t } = useTranslation();
  const { datepicker } = useDatepicker();

  return useMemo(() => {
    const cols: TableColumnType<DBInboundRecord>[] = [
      {
        title: 'ID',
        dataIndex: 'id',
        key: 'id',
        align: 'right',
        width: 30,
        render: (id: number) => <span className="omega-id">#{id}</span>,
      },
      {
        title: t('pages.inbounds.operate'),
        key: 'action',
        align: 'center',
        width: 60,
        render: (_, record) => (
          <RowActionsCell
            record={record}
            subEnable={subEnable}
            hasClients={(clientCount[record.id]?.clients || 0) > 0}
            onClick={(key) => onRowAction({ key, dbInbound: record })}
            isReseller={isReseller}
          />
        ),
      },
      {
        title: t('pages.inbounds.enable'),
        key: 'enable',
        align: 'center',
        width: 35,
        render: (_, record) => (
          <Switch
            checked={record.enable}
            disabled={isReseller}
            onChange={(next) => onSwitchEnable(record, next)}
          />
        ),
      },
    ];

    if (hasAnyRemark) {
      cols.push({
        title: t('pages.inbounds.remark'),
        dataIndex: 'remark',
        key: 'remark',
        align: 'left',
        width: 80,
        render: (remark: string) => <span className="inbound-remark">{remark}</span>,
      });
    }

    if (hasActiveNode) {
      cols.push({
        title: t('pages.inbounds.node'),
        key: 'node',
        align: 'center',
        width: 60,
        render: (_, record) => {
          if (record.nodeId == null) {
            return <Tag color="default">{t('pages.inbounds.localPanel')}</Tag>;
          }
          const node = nodesById.get(record.nodeId);
          if (!node) {
            return <Tag color="orange">node #{record.nodeId}</Tag>;
          }
          return (
            <Tag color={node.status === 'online' ? 'blue' : 'red'}>{node.name}</Tag>
          );
        },
      });
    }

    if (hasAnySubSortIndex) {
      cols.push({
        title: (
          <Tooltip title={t('pages.inbounds.form.subSortIndex')}>
            {t('pages.inbounds.subSortIndex')}
          </Tooltip>
        ),
        dataIndex: 'subSortIndex',
        key: 'subSortIndex',
        align: 'right',
        width: 70,
      });
    }

    cols.push(
      {
        title: t('pages.inbounds.port'),
        dataIndex: 'port',
        key: 'port',
        align: 'center',
        width: 40,
        render: (port: number) => <span className="omega-mono-sm">{port}</span>,
      },
      {
        title: t('pages.inbounds.protocol'),
        key: 'protocol',
        align: 'left',
        width: 130,
        render: (_, record) => {
          const chip = (key: string, label: ReactElement | string, tone = '') => (
            <span key={key} className={`omega-chip${tone ? ` ${tone}` : ''}`}>{label}</span>
          );
          const tags: ReactElement[] = [chip('p', record.protocol, 'is-protocol')];
          if (record.isWireguard || record.isHysteria) {
            tags.push(chip('n', 'UDP', 'is-success'));
          } else if (record.isSS) {
            const stream = readStreamHints(record.streamSettings);
            tags.push(chip('n', shadowsocksNetworkLabel(record.settings), 'is-success'));
            if (stream.isTls) tags.push(chip('tls', 'TLS', 'is-info'));
          } else if (record.isTunnel) {
            tags.push(chip('n', tunnelNetworkLabel(record.settings), 'is-success'));
          } else if (record.isMixed) {
            tags.push(chip('n', mixedNetworkLabel(record.settings), 'is-success'));
          } else if (record.isVMess || record.isVLess || record.isTrojan) {
            const stream = readStreamHints(record.streamSettings);
            tags.push(chip('n', networkLabel(stream.network), 'is-success'));
            const l4 = networkL4(stream.network);
            if (l4) tags.push(chip('l4', l4, 'is-success'));
            if (stream.isTls) tags.push(chip('tls', 'TLS', 'is-info'));
            if (stream.isReality) tags.push(chip('reality', 'Reality', 'is-accent'));
          }
          return <div className="protocol-tags">{tags}</div>;
        },
      },
      {
        title: t('clients'),
        key: 'clients',
        align: 'left',
        width: 110,
        render: (_, record) => {
          const cc = clientCount[record.id];
          if (!cc) return null;
          return (
            <>
              <Tag className="client-count-tag" style={{ margin: 0, marginRight: 4, padding: '0 2px' }}>
                <TeamOutlined /> {cc.clients}
              </Tag>
              {cc.active.length > 0 && (
                <Popover
                  title={t('subscription.active')}
                  content={(
                    <div className="client-email-list">
                      {cc.active.map((e) => <div key={e}>{e}</div>)}
                    </div>
                  )}
                >
                  <Tag color="green" className="client-count-tag" style={{ margin: 0, marginRight: 4, padding: '0 2px' }}>{cc.active.length}</Tag>
                </Popover>
              )}
              {cc.deactive.length > 0 && (
                <Popover
                  title={t('disabled')}
                  content={(
                    <div className="client-email-list">
                      {cc.deactive.map((e) => <div key={e}>{e}</div>)}
                    </div>
                  )}
                >
                  <Tag className="client-count-tag" style={{ margin: 0, marginRight: 4, padding: '0 2px' }}>{cc.deactive.length}</Tag>
                </Popover>
              )}
              {cc.depleted.length > 0 && (
                <Popover
                  title={t('depleted')}
                  content={(
                    <div className="client-email-list">
                      {cc.depleted.map((e) => <div key={e}>{e}</div>)}
                    </div>
                  )}
                >
                  <Tag color="red" className="client-count-tag" style={{ margin: 0, marginRight: 4, padding: '0 2px' }}>{cc.depleted.length}</Tag>
                </Popover>
              )}
              {cc.online.length > 0 && (
                <Popover
                  title={t('online')}
                  content={(
                    <div className="client-email-list">
                      {cc.online.map((e) => <div key={e}>{e}</div>)}
                    </div>
                  )}
                >
                  <Tag color="blue" className="client-count-tag" style={{ margin: 0, padding: '0 2px' }}>{cc.online.length}</Tag>
                </Popover>
              )}
            </>
          );
        },
      },
      {
        title: t('pages.inbounds.traffic'),
        key: 'traffic',
        align: 'center',
        width: 180,
        render: (_, record) => (
          <ClientTrafficCell
            up={record.up}
            down={record.down}
            total={record.total}
            enabled={record.enable}
            trafficDiff={trafficDiff}
            compact
          />
        ),
      },
      {
        title: t('pages.inbounds.expireDate'),
        key: 'expiryTime',
        align: 'center',
        width: 40,
        render: (_, record) => {
          if (record.expiryTime > 0) {
            return (
              <Popover content={IntlUtil.formatDate(record.expiryTime, datepicker)}>
                <Tag color={ColorUtils.usageColor(Date.now(), expireDiff, record._expiryTime)} style={{ minWidth: 50 }}>
                  {IntlUtil.formatRelativeTime(record.expiryTime)}
                </Tag>
              </Popover>
            );
          }
          return <Tag color="purple"><InfinityIcon /></Tag>;
        },
      },
    );

    return cols;
  }, [t, hasAnyRemark, hasAnySubSortIndex, hasActiveNode, nodesById, clientCount, subEnable, expireDiff, trafficDiff, datepicker, onRowAction, onSwitchEnable, isReseller]);
}
