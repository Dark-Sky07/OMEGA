import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { ConfigProvider, Layout } from 'antd';
import SwaggerUI from 'swagger-ui-react';
import 'swagger-ui-react/swagger-ui.css';

import { useTheme } from '@/hooks/useTheme';
import AppSidebar from '@/layouts/AppSidebar';
import { PageHeader } from '@/components/ui';
import './ApiDocsPage.css';

const basePath = window.X_UI_BASE_PATH || '';
const openApiUrl = `${basePath}panel/api/openapi.json`;

export default function ApiDocsPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();

  const pageClass = useMemo(() => {
    const classes = ['api-docs-page'];
    if (isDark) classes.push('is-dark');
    if (isUltra) classes.push('is-ultra');
    return classes.join(' ');
  }, [isDark, isUltra]);

  return (
    <ConfigProvider theme={antdThemeConfig}>
      <Layout className={pageClass}>
        <AppSidebar />

        <Layout className="content-shell">
          <Layout.Content className="content-area">
            <PageHeader
              eyebrow={t('menu.groupSystem', 'System')}
              title={t('menu.apiDocs')}
              subtitle={<span className="omega-mono-sm">{openApiUrl}</span>}
            />
            <div className="docs-wrapper omega-glass omega-rise omega-rise-2">
              <SwaggerUI
                url={openApiUrl}
                docExpansion="list"
                deepLinking={false}
                tryItOutEnabled
                persistAuthorization
              />
            </div>
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
