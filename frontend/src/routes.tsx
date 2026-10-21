import { lazy, Suspense } from 'react';
import { createBrowserRouter, Navigate, useLocation, type RouteObject } from 'react-router-dom';

import PanelLayout from '@/layouts/PanelLayout';
import { useSession } from '@/api/queries/useSession';

const IndexPage = lazy(() => import('@/pages/index/IndexPage'));
const InboundsPage = lazy(() => import('@/pages/inbounds/InboundsPage'));
const ClientsPage = lazy(() => import('@/pages/clients/ClientsPage'));
const GroupsPage = lazy(() => import('@/pages/groups/GroupsPage'));
const NodesPage = lazy(() => import('@/pages/nodes/NodesPage'));
const SettingsPage = lazy(() => import('@/pages/settings/SettingsPage'));
const XrayPage = lazy(() => import('@/pages/xray/XrayPage'));
const ApiDocsPage = lazy(() => import('@/pages/api-docs/ApiDocsPage'));
const ResellersPage = lazy(() => import('@/pages/resellers/ResellersPage'));
const ResellerReportPage = lazy(() => import('@/pages/resellers/ResellerReportPage'));
const ResellerProfilePage = lazy(() => import('@/pages/resellers/ResellerProfilePage'));
const ResellerRenewalsPage = lazy(() => import('@/pages/resellers/ResellerRenewalsPage'));
const CheckoutPage = lazy(() => import('@/pages/checkout/CheckoutPage'));
const ResellerSignupPage = lazy(() => import('@/pages/checkout/ResellerSignupPage'));
const ResellerDepositPage = lazy(() => import('@/pages/checkout/ResellerDepositPage'));

function withSuspense(node: React.ReactNode) {
  return <Suspense fallback={null}>{node}</Suspense>;
}

/**
 * Admin-only views. A reseller (نمایندگی) gets bounced to their own report page
 * instead of seeing panel-wide pages the API would refuse to answer anyway.
 */
function AdminOnly({ children }: { children: React.ReactNode }) {
  const { isReseller, loading } = useSession();
  if (loading) return null;
  if (isReseller) return <Navigate to="/reseller/report" replace />;
  return <>{children}</>;
}

/** Reseller-only views. The admin manages resellers from /resellers instead. */
function ResellerOnly({ children }: { children: React.ReactNode }) {
  const { isAdmin, loading } = useSession();
  if (loading) return null;
  if (isAdmin) return <Navigate to="/resellers" replace />;
  return <>{children}</>;
}

/** Root route: send resellers to their report instead of the admin overview. */
function LandingPage() {
  const { isReseller, loading } = useSession();
  const location = useLocation();
  if (loading) return null;
  if (isReseller) return <Navigate to="/reseller/report" replace state={{ from: location }} />;
  return withSuspense(<IndexPage />);
}

export const appRoutes: RouteObject[] = [
  {
    path: '/',
    element: <PanelLayout />,
    children: [
      { index: true, element: <LandingPage /> },
      // Inbounds and clients are shared: the API scopes them to what a
      // reseller owns, so both roles may open these pages.
      { path: 'inbounds', element: withSuspense(<InboundsPage />) },
      { path: 'clients', element: withSuspense(<ClientsPage />) },
      {
        path: 'groups',
        element: <AdminOnly>{withSuspense(<GroupsPage />)}</AdminOnly>,
      },
      {
        path: 'nodes',
        element: <AdminOnly>{withSuspense(<NodesPage />)}</AdminOnly>,
      },
      {
        path: 'settings',
        element: <AdminOnly>{withSuspense(<SettingsPage />)}</AdminOnly>,
      },
      {
        path: 'xray',
        element: <AdminOnly>{withSuspense(<XrayPage />)}</AdminOnly>,
      },
      {
        path: 'api-docs',
        element: <AdminOnly>{withSuspense(<ApiDocsPage />)}</AdminOnly>,
      },
      {
        path: 'resellers',
        element: <AdminOnly>{withSuspense(<ResellersPage />)}</AdminOnly>,
      },
      {
        path: 'reseller/report',
        element: <ResellerOnly>{withSuspense(<ResellerReportPage />)}</ResellerOnly>,
      },
      {
        path: 'reseller/profile',
        element: <ResellerOnly>{withSuspense(<ResellerProfilePage />)}</ResellerOnly>,
      },
      {
        path: 'reseller/renewals',
        element: <ResellerOnly>{withSuspense(<ResellerRenewalsPage />)}</ResellerOnly>,
      },
    ],
  },
  {
    // Public client checkout: opened from a signed per-client link
    // (?ref=<token>), never behind a panel session.
    path: 'checkout',
    element: withSuspense(<CheckoutPage />),
  },
  {
    // Public reseller self-registration using a signed plan invitation.
    path: 'reseller-signup',
    element: withSuspense(<ResellerSignupPage />),
  },
  {
    // Public reseller wallet recharge page.
    path: 'reseller-deposit',
    element: withSuspense(<ResellerDepositPage />),
  },
];

function computeBasename() {
  const raw = (typeof window !== 'undefined' && window.X_UI_BASE_PATH) || '/';
  const trimmed = raw.replace(/\/+$/, '');
  return `${trimmed}/panel`;
}

export const router = createBrowserRouter(appRoutes, {
  basename: computeBasename(),
});
