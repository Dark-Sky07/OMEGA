import { useTranslation } from 'react-i18next';
import { Card, Tooltip } from 'antd';
import {
  ArrowDownOutlined,
  ArrowUpOutlined,
  AreaChartOutlined,
  CloudDownloadOutlined,
  CloudUploadOutlined,
  SwapOutlined,
} from '@ant-design/icons';

import { SizeFormatter } from '@/utils';
import { Sparkline } from '@/components/viz';
import type { Status } from '@/models/status';
import './NetworkPanel.css';

interface NetworkPanelProps {
  status: Status;
  upHistory: number[];
  downHistory: number[];
  onOpenHistory: () => void;
}

const UP_COLOR = '#22d3ee';
const DOWN_COLOR = '#8b6cff';

function Speed({ icon, label, value, color }: { icon: React.ReactNode; label: string; value: number; color: string }) {
  return (
    <div className="net-speed" style={{ ['--net-color' as string]: color }}>
      <span className="net-speed-icon">{icon}</span>
      <div className="net-speed-copy">
        <span className="net-speed-label">{label}</span>
        <span className="net-speed-value">
          {SizeFormatter.sizeFormat(value)}
          <small>/s</small>
        </span>
      </div>
    </div>
  );
}

function Stat({ icon, label, value }: { icon: React.ReactNode; label: string; value: React.ReactNode }) {
  return (
    <div className="net-stat">
      <span className="net-stat-icon">{icon}</span>
      <div className="net-stat-copy">
        <span className="net-stat-label">{label}</span>
        <span className="net-stat-value">{value}</span>
      </div>
    </div>
  );
}

/** Live throughput with a rolling sparkline, plus lifetime totals and sockets. */
export default function NetworkPanel({ status, upHistory, downHistory, onOpenHistory }: NetworkPanelProps) {
  const { t } = useTranslation();
  const fmt = (v: number) => `${SizeFormatter.sizeFormat(v)}/s`;

  return (
    <Card
      className="network-panel"
      title={<span className="omega-eyebrow">{t('pages.index.network', 'Network')}</span>}
      extra={(
        <Tooltip title={t('pages.index.systemHistoryTitle')}>
          <button type="button" className="omega-icon-btn network-history-btn" onClick={onOpenHistory} aria-label={t('pages.index.systemHistoryTitle')}>
            <AreaChartOutlined />
          </button>
        </Tooltip>
      )}
    >
      <div className="net-speeds">
        <Speed icon={<ArrowUpOutlined />} label={t('pages.index.upload')} value={status.netIO.up} color={UP_COLOR} />
        <Speed icon={<ArrowDownOutlined />} label={t('pages.index.download')} value={status.netIO.down} color={DOWN_COLOR} />
      </div>

      <div className="net-chart" aria-hidden="true">
        <Sparkline
          data={upHistory}
          data2={downHistory}
          stroke={UP_COLOR}
          stroke2={DOWN_COLOR}
          height={96}
          strokeWidth={2}
          showGrid={false}
          showMarker={false}
          fillOpacity={0.18}
          valueMin={0}
          valueMax={null}
          maxPoints={60}
          yFormatter={fmt}
          showTooltip
          tooltipFormatter={fmt}
        />
      </div>

      <div className="net-stats">
        <Stat icon={<CloudUploadOutlined />} label={t('pages.index.sent')} value={SizeFormatter.sizeFormat(status.netTraffic.sent)} />
        <Stat icon={<CloudDownloadOutlined />} label={t('pages.index.received')} value={SizeFormatter.sizeFormat(status.netTraffic.recv)} />
        <Stat icon={<SwapOutlined />} label="TCP" value={status.tcpCount} />
        <Stat icon={<SwapOutlined />} label="UDP" value={status.udpCount} />
      </div>
    </Card>
  );
}
