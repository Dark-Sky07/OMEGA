import type { ReactNode } from 'react';
import { Popover } from 'antd';

import './StatStrip.css';

export type StatTone = 'default' | 'primary' | 'accent' | 'success' | 'warning' | 'danger' | 'muted';

export interface StatItem {
  key: string;
  label: ReactNode;
  value: ReactNode;
  icon?: ReactNode;
  /** Secondary line under the value (e.g. "of 120"). */
  hint?: ReactNode;
  tone?: StatTone;
  /** When set the chip becomes a trigger for this popover (hover/click). */
  popover?: { title?: ReactNode; content: ReactNode };
  onClick?: () => void;
}

export interface StatStripProps {
  items: StatItem[];
  className?: string;
  /** Visual density. `compact` is used inside cards and on mobile. */
  size?: 'default' | 'compact';
}

/**
 * Row of glass "stat chips" that replaces the old Statistic grids on list
 * pages. Each chip is icon + label + value; optional popover / click.
 */
export default function StatStrip({ items, className, size = 'default' }: StatStripProps) {
  return (
    <div className={`omega-stat-strip is-${size}${className ? ` ${className}` : ''}`}>
      {items.map((item, index) => {
        const interactive = !!item.onClick || !!item.popover;
        const chip = (
          <div
            key={item.key}
            className={`omega-stat is-${item.tone ?? 'default'}${interactive ? ' is-interactive' : ''} omega-rise omega-rise-${Math.min(index + 1, 5)}`}
            role={item.onClick ? 'button' : undefined}
            tabIndex={item.onClick ? 0 : undefined}
            onClick={item.onClick}
            onKeyDown={item.onClick ? (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); item.onClick?.(); } } : undefined}
          >
            {item.icon && <span className="omega-stat-icon">{item.icon}</span>}
            <div className="omega-stat-copy">
              <span className="omega-stat-label">{item.label}</span>
              <span className="omega-stat-value">{item.value}</span>
              {item.hint && <span className="omega-stat-hint">{item.hint}</span>}
            </div>
          </div>
        );
        if (item.popover) {
          return (
            <Popover key={item.key} title={item.popover.title} content={item.popover.content} trigger={['hover', 'click']}>
              {chip}
            </Popover>
          );
        }
        return chip;
      })}
    </div>
  );
}
