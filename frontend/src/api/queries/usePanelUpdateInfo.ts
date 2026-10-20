import { useQuery, useQueryClient } from '@tanstack/react-query';

import { HttpUtil } from '@/utils';
import { keys } from '@/api/queryKeys';

export interface PanelUpdateInfo {
  currentVersion: string;
  latestVersion: string;
  updateAvailable: boolean;
}

export const EMPTY_PANEL_UPDATE_INFO: PanelUpdateInfo = {
  currentVersion: '',
  latestVersion: '',
  updateAvailable: false,
};

async function fetchPanelUpdateInfo(force: boolean): Promise<PanelUpdateInfo> {
  const msg = await HttpUtil.get<PanelUpdateInfo>(
    `/panel/api/server/getPanelUpdateInfo${force ? '?force=true' : ''}`,
    undefined,
    { silent: true },
  );
  if (!msg?.success || !msg.obj) {
    throw new Error(msg?.msg || 'panel update check failed');
  }
  return msg.obj;
}

/**
 * Shared panel-update status used by the dashboard card, the sidebar
 * version badge and the update modal. The backend caches the GitHub lookup,
 * so every surface can read this without extra API pressure.
 *
 * `checkFailed` distinguishes "up to date" from "could not reach GitHub".
 */
export function usePanelUpdateInfo() {
  const queryClient = useQueryClient();
  const query = useQuery({
    queryKey: keys.server.panelUpdateInfo(),
    queryFn: () => fetchPanelUpdateInfo(false),
    staleTime: 2 * 60 * 1000,
    retry: false,
  });

  const refresh = async () => {
    await queryClient.fetchQuery({
      queryKey: keys.server.panelUpdateInfo(),
      queryFn: () => fetchPanelUpdateInfo(true),
      staleTime: 0,
    });
  };

  return {
    info: query.data ?? EMPTY_PANEL_UPDATE_INFO,
    checkFailed: query.isError || (query.isSuccess && !query.data),
    loading: query.isLoading,
    refreshing: query.isFetching,
    refresh,
  };
}
