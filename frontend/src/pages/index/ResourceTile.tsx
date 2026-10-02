import type { ReactNode } from 'react';
import { Progress, Tooltip } from 'antd';

import './ResourceTile.css';

interface ResourceTileProps {
  label: string;
  icon: ReactNode;
  percent: number;
  /** Primary reading, e.g. "3.2 GB / 8 GB". */
  value: ReactNode;
  /** Secondary line under the value. */
  caption?: ReactNode;
  /** Optional hover details (CPU model, frequency, ...). */
  details?: ReactNode;
  size?: 'default' | 'compact';
  className?: string;
}

const HEALTHY_GRADIENT = { '0%': '#7c5cff', '100%': '#22d3ee' };
const WARN_GRADIENT = { '0%': '#f59e0b', '100%': '#fbbf24' };
const DANGER_GRADIENT = { '0%': '#f43f5e', '100%': '#fb7185' };

function toneFor(percent: number): 'ok' | 'warn' | 'danger' {
  if (percent >= 90) return 'danger';
  if (percent >= 80) return 'warn';
  return 'ok';
}

/** Ring-gauge tile used for CPU / RAM / swap / disk on the overview. */
export default function ResourceTile({
  label,
  icon,
  percent,
  value,
  caption,
  details,
  size = 'default',
  className = '',
}: ResourceTileProps) {
  const safe = Number.isFinite(percent) ? Math.max(0, Math.min(100, percent)) : 0;
  const tone = toneFor(safe);
  const gradient = tone === 'danger' ? DANGER_GRADIENT : tone === 'warn' ? WARN_GRADIENT : HEALTHY_GRADIENT;
  const ring = size === 'compact' ? 64 : 84;

  const body = (
    <div className={`resource-tile is-${tone} is-${size} ${className}`.trim()}>
      <div className="resource-tile-ring">
        <Progress
          type="circle"
          percent={safe}
          size={ring}
          strokeWidth={9}
          strokeColor={gradient}
          strokeLinecap="round"
          format={() => (
            <span className="resource-tile-ring-value">
              {Math.round(safe)}
              <small>%</small>
            </span>
          )}
        />
      </div>
      <div className="resource-tile-body">
        <div className="resource-tile-label">
          <span className="resource-tile-icon">{icon}</span>
          <span>{label}</span>
        </div>
        <div className="resource-tile-value">{value}</div>
        {caption && <div className="resource-tile-caption">{caption}</div>}
      </div>
    </div>
  );

  if (!details) return body;
  return (
    <Tooltip title={details} placement="bottom">
      {body}
    </Tooltip>
  );
}
