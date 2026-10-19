import { useCallback, useEffect, useState } from 'react';
import { HttpUtil } from '@/utils';

export interface UserTemplate {
  id: number;
  name: string;
  description: string;
  totalGB: number;
  expireDurationDays: number;
  delayedStart: boolean;
  activateBeforeDays: number;
  limitIp: number;
  limitDevice: number;
  reset: number;
  renewalMode: number;
  renewalDay: number;
  fixedPrice?: number;
  group: string;
  inboundIds: number[];
  createdAt?: number;
  updatedAt?: number;
}

const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } };

export function useUserTemplates(enabled = true) {
  const [templates, setTemplates] = useState<UserTemplate[]>([]);
  const [loading, setLoading] = useState(false);

  const reload = useCallback(async () => {
    setLoading(true);
    try {
      const msg = await HttpUtil.get('/panel/api/clients/templates');
      if (msg?.success && Array.isArray(msg.obj)) {
        setTemplates(msg.obj as UserTemplate[]);
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (enabled) {
      void reload();
    }
  }, [enabled, reload]);

  const createTemplate = useCallback(
    async (payload: Partial<UserTemplate>) => {
      const msg = await HttpUtil.post('/panel/api/clients/templates/add', payload, JSON_HEADERS);
      if (msg?.success) {
        await reload();
      }
      return msg;
    },
    [reload],
  );

  const updateTemplate = useCallback(
    async (id: number, payload: Partial<UserTemplate>) => {
      const msg = await HttpUtil.post(
        `/panel/api/clients/templates/update/${id}`,
        payload,
        JSON_HEADERS,
      );
      if (msg?.success) {
        await reload();
      }
      return msg;
    },
    [reload],
  );

  const deleteTemplate = useCallback(
    async (id: number) => {
      const msg = await HttpUtil.post(`/panel/api/clients/templates/del/${id}`);
      if (msg?.success) {
        await reload();
      }
      return msg;
    },
    [reload],
  );

  return {
    templates,
    loading,
    reload,
    createTemplate,
    updateTemplate,
    deleteTemplate,
  };
}
