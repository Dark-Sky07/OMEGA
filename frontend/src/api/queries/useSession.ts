import { useQuery } from '@tanstack/react-query';

import { HttpUtil } from '@/utils';
import { keys } from '@/api/queryKeys';

/** Live usage snapshot of a reseller (نمایندگی) account. */
export interface ResellerStat {
  reseller: {
    id: number;
    username: string;
    name: string;
    comment?: string;
    enable: boolean;
    trafficLimit: number;
    clientLimit: number;
    expiryTime: number;
    // Kept for backward compat, billing is now removed from UI
    pricePerGb?: number;
    deposit?: number;
    createdAt?: number;
    updatedAt?: number;
  };
  inboundCount: number;
  clientCount: number;
  onlineCount: number;
  usedTraffic: number;
  allocatedTraffic: number;
  remainingTraffic: number;
  cost?: number;
  balance?: number;
  expired: boolean;
  overQuota: boolean;
  disabled: boolean;
}

export interface ResellerClientRow {
  email: string;
  comment?: string;
  group?: string;
  enable: boolean;
  totalBytes: number;
  expiryTime: number;
  up: number;
  down: number;
  used: number;
  cost?: number;
  inboundIds: number[];
  subId?: string;
}

export interface ResellerTransaction {
  id: number;
  resellerId: number;
  type: string;
  amount: number;
  balance: number;
  comment?: string;
  createdAt: number;
}

export interface ResellerReport {
  stat: ResellerStat;
  clients: ResellerClientRow[];
  transactions: ResellerTransaction[];
}

export interface SessionInfo {
  role: 'admin' | 'reseller';
  username: string;
  name: string;
  stat?: ResellerStat;
}

async function fetchSession(): Promise<SessionInfo> {
  const msg = await HttpUtil.get<SessionInfo>('/panel/api/auth/me', undefined, { silent: true });
  if (!msg?.success || !msg.obj) {
    throw new Error(msg?.msg || 'not authenticated');
  }
  return msg.obj;
}

/**
 * Who is using the panel right now. The backend answers `admin` or `reseller`;
 * the UI uses it to pick the navigation, the landing page and which actions to
 * offer. Cached for the lifetime of the tab — the role never changes without a
 * reload (login/logout redirects).
 */
export function useSession() {
  const query = useQuery({
    queryKey: keys.session.me(),
    queryFn: fetchSession,
    staleTime: Infinity,
    retry: false,
  });

  const role = query.data?.role;
  return {
    session: query.data,
    role,
    isAdmin: role === 'admin',
    isReseller: role === 'reseller',
    loading: query.isLoading,
    error: query.error,
  };
}
