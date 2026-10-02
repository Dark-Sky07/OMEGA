import type { ReactNode } from 'react';

import './PageHeader.css';

export interface PageHeaderProps {
  /** Small uppercase label above the title (section / group name). */
  eyebrow?: ReactNode;
  title: ReactNode;
  /** One-liner under the title — ideally live numbers, not marketing copy. */
  subtitle?: ReactNode;
  /** Primary actions, rendered at the end of the row (start in RTL). */
  actions?: ReactNode;
  /** Optional status pill / badge rendered next to the title. */
  badge?: ReactNode;
  className?: string;
  children?: ReactNode;
}

/**
 * Aurora page header: eyebrow + big title + subtitle on one side, actions on
 * the other. Purely presentational — pages keep their own handlers.
 */
export default function PageHeader({ eyebrow, title, subtitle, actions, badge, className, children }: PageHeaderProps) {
  return (
    <header className={`omega-page-header omega-rise${className ? ` ${className}` : ''}`}>
      <div className="omega-page-heading">
        {eyebrow && <span className="omega-eyebrow">{eyebrow}</span>}
        <div className="omega-page-title-row">
          <h1 className="omega-page-title">{title}</h1>
          {badge}
        </div>
        {subtitle && <p className="omega-page-subtitle">{subtitle}</p>}
        {children}
      </div>
      {actions && <div className="omega-page-actions">{actions}</div>}
    </header>
  );
}
