import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Badge, Card, Col, Popover, Row, Space, Tag } from 'antd';
import {
  BarsOutlined,
  CloudSyncOutlined,
  PlayCircleOutlined,
  PoweroffOutlined,
  ReloadOutlined,
} from '@ant-design/icons';

import type { DaemonInfo } from '@/models/status';
import './DaemonStatusCard.css';

interface DaemonStatusCardProps {
  title: string;
  status: DaemonInfo;
  isMobile: boolean;
  onStop: () => void;
  onStart: () => void;
  onRestart: () => void;
  onUpdate: () => void;
  onOpenLogs: () => void;
}

const STATE_KEYS: Record<string, string> = {
  running: 'pages.index.xrayStatusRunning',
  stop: 'pages.index.xrayStatusStop',
  error: 'pages.index.xrayStatusError',
};

export default function DaemonStatusCard({
  title,
  status,
  isMobile,
  onStop,
  onStart,
  onRestart,
  onUpdate,
  onOpenLogs,
}: DaemonStatusCardProps) {
  const { t } = useTranslation();
  const stateText = t(STATE_KEYS[status.state] ?? 'pages.index.xrayStatusUnknown');
  const errorLines = useMemo(() => (status.errorMsg || '').split('\n'), [status.errorMsg]);

  const extra = status.state === 'error' ? (
    <Popover
      title={
        <Row align="middle" justify="space-between">
          <Col><span>{t('pages.index.xrayStatusError')}</span></Col>
          <Col><BarsOutlined className="cursor-pointer" onClick={onOpenLogs} /></Col>
        </Row>
      }
      content={errorLines.map((line, i) => (
        <span key={i} className="daemon-error-line">{line}</span>
      ))}
    >
      <Badge status="processing" text={stateText} color={status.color} />
    </Popover>
  ) : (
    <Badge status="processing" text={stateText} color={status.color} />
  );

  return (
    <Card
      hoverable
      title={
        <Space>
          <span>{title}</span>
          {isMobile && <Tag color={status.state === 'running' ? 'green' : 'orange'}>{stateText}</Tag>}
        </Space>
      }
      extra={extra}
      actions={[
        status.state === 'running' ? (
          <Space className="action" key="stop" onClick={onStop}>
            <PoweroffOutlined />
            {!isMobile && <span>{t('pages.index.stopDaemon', 'Stop')}</span>}
          </Space>
        ) : (
          <Space className="action" key="start" onClick={onStart}>
            <PlayCircleOutlined />
            {!isMobile && <span>{t('pages.index.startDaemon', 'Start')}</span>}
          </Space>
        ),
        <Space className="action" key="restart" onClick={onRestart}>
          <ReloadOutlined />
          {!isMobile && <span>{t('pages.index.restartDaemon', 'Restart')}</span>}
        </Space>,
        <Space className="action" key="update" onClick={onUpdate}>
          <CloudSyncOutlined />
          {!isMobile && <span>{t('pages.index.updateDaemon', 'Update')}</span>}
        </Space>,
        <Space className="daemon-stats" key="stats">
          <span>{t('pages.index.daemonInbounds', 'Inbounds')}: {status.inboundCount}</span>
          <span>{t('pages.index.daemonOnline', 'Online')}: {status.onlineClients}</span>
        </Space>,
      ]}
      className="daemon-status-card"
    />
  );
}
