import { useTranslation } from 'react-i18next';
import { Alert, Button, Modal, Tag, Tooltip } from 'antd';
import { CloudDownloadOutlined, LinkOutlined, ReloadOutlined } from '@ant-design/icons';
import axios from 'axios';

import { HttpUtil, PromiseUtil } from '@/utils';
import './PanelUpdateModal.css';

export interface PanelUpdateInfo {
  currentVersion: string;
  latestVersion: string;
  updateAvailable: boolean;
}

interface BusyEvent {
  busy: boolean;
  tip?: string;
}

interface PanelUpdateModalProps {
  open: boolean;
  info: PanelUpdateInfo;
  checkFailed?: boolean;
  refreshing?: boolean;
  onRefresh?: () => void;
  onClose: () => void;
  onBusy: (e: BusyEvent) => void;
}

export default function PanelUpdateModal({
  open,
  info,
  checkFailed = false,
  refreshing = false,
  onRefresh,
  onClose,
  onBusy,
}: PanelUpdateModalProps) {
  const { t } = useTranslation();
  const [modal, contextHolder] = Modal.useModal();

  async function pollUntilBack(): Promise<boolean> {
    await PromiseUtil.sleep(5000);
    const deadline = Date.now() + 90_000;
    while (Date.now() < deadline) {
      try {
        const r = await axios.get('/panel/api/server/status', { timeout: 2000 });
        if (r?.data?.success) return true;
      } catch {
        /* still restarting */
      }
      await PromiseUtil.sleep(2000);
    }
    return false;
  }

  function updatePanel() {
    modal.confirm({
      title: t('pages.index.panelUpdateDialog'),
      content: t('pages.index.panelUpdateDialogDesc').replace('#version#', info.latestVersion || ''),
      okText: t('confirm'),
      cancelText: t('cancel'),
      onOk: async () => {
        const baseTip = t('pages.index.dontRefresh');
        const tip = info.latestVersion ? `${baseTip} (${info.latestVersion})` : baseTip;
        onClose();
        onBusy({ busy: true, tip });
        const result = await HttpUtil.post('/panel/api/server/updatePanel');
        if (!result?.success) {
          onBusy({ busy: false });
          return;
        }
        const back = await pollUntilBack();
        if (back) await PromiseUtil.sleep(800);
        window.location.reload();
      },
    });
  }

  const summary = checkFailed
    ? { type: 'error' as const, text: t('pages.index.panelUpdateCheckFailed') }
    : info.updateAvailable
      ? { type: 'warning' as const, text: t('pages.index.panelUpdateDesc') }
      : { type: 'success' as const, text: t('pages.index.panelUpToDate') };

  return (
    <>
      {contextHolder}
      <Modal
        open={open}
        title={t('pages.index.updatePanel')}
        footer={null}
        onCancel={onClose}
      >
        <Alert type={summary.type} title={summary.text} showIcon className="mb-12" />

        <div className="version-list">
          <div className="version-list-item">
            <span>{t('pages.index.currentPanelVersion')}</span>
            <Tag color="green">v{info.currentVersion || '?'}</Tag>
          </div>
          {info.updateAvailable && (
            <div className="version-list-item">
              <span>{t('pages.index.latestPanelVersion')}</span>
              <Tag color="purple">{info.latestVersion || '-'}</Tag>
            </div>
          )}
        </div>

        <div className="actions-row">
          {info.updateAvailable && !checkFailed && (
            <Button
              type="primary"
              onClick={updatePanel}
              icon={<CloudDownloadOutlined />}
            >
              {t('update')} {info.latestVersion}
            </Button>
          )}
          {onRefresh && (
            <Tooltip title={t('pages.index.panelUpdateCheckAgain')}>
              <Button onClick={onRefresh} icon={<ReloadOutlined />} loading={refreshing}>
                {t('pages.index.panelUpdateCheckAgain')}
              </Button>
            </Tooltip>
          )}
          <Tooltip title={t('pages.index.viewReleases')}>
            <a
              className="releases-link"
              href="https://github.com/Dark-Sky07/PHOENIX/releases"
              target="_blank"
              rel="noopener noreferrer"
            >
              <LinkOutlined /> {t('pages.index.viewReleases')}
            </a>
          </Tooltip>
        </div>
      </Modal>
    </>
  );
}
