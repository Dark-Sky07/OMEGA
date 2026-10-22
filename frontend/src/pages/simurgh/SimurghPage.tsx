import { Layout } from 'antd';

import AppSidebar from '@/layouts/AppSidebar';
import { useTheme } from '@/hooks/useTheme';
import './SimurghPage.css';

function simurghPanelURL() {
  const raw = window.X_UI_BASE_PATH || '/';
  const basePath = raw.replace(/\/+$/, '');
  return `${basePath}/panel/simurgh/`;
}

export default function SimurghPage() {
  const { isDark, isUltra } = useTheme();
  const pageClass = `simurgh-page ${isDark ? 'is-dark' : ''} ${isUltra ? 'is-ultra' : ''}`.trim();

  return (
    <Layout className={pageClass}>
      <AppSidebar />
      <Layout className="content-shell">
        <Layout.Content className="content-area simurgh-content">
          <iframe
            className="simurgh-frame"
            src={simurghPanelURL()}
            title="Simurgh Tunnel"
            referrerPolicy="no-referrer"
            allow="clipboard-read; clipboard-write"
          />
        </Layout.Content>
      </Layout>
    </Layout>
  );
}
