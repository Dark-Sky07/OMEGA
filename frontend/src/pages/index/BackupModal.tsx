import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Modal, message } from 'antd';
import { CloudUploadOutlined, DownloadOutlined, UploadOutlined } from '@ant-design/icons';

import { HttpUtil, PromiseUtil } from '@/utils';
import './BackupModal.css';

interface BusyEvent {
  busy: boolean;
  tip?: string;
}

interface BackupModalProps {
  open: boolean;
  basePath: string;
  onClose: () => void;
  onBusy: (e: BusyEvent) => void;
}

export default function BackupModal({ open, basePath: _basePath, onClose, onBusy }: BackupModalProps) {
  const { t } = useTranslation();
  const isPostgres = window.X_UI_DB_TYPE === 'postgres';
  const [cloudLoading, setCloudLoading] = useState(false);

  async function backupToCloud() {
    setCloudLoading(true);
    try {
      const res = await HttpUtil.post('/panel/api/setting/cloudBackup/run', {}) as {
        success?: boolean;
        obj?: { key?: string };
      };
      if (res?.success) {
        message.success(`${t('pages.settings.cloudBackupSuccess', 'Cloud backup uploaded')}: ${res.obj?.key || ''}`);
      }
    } finally {
      setCloudLoading(false);
    }
  }

  function exportDb() {
    window.location.href = (window.X_UI_BASE_PATH || '') + 'panel/api/server/getDb';
  }

  function exportMigration() {
    window.location.href = (window.X_UI_BASE_PATH || '') + 'panel/api/server/getMigration';
  }

  function importDb() {
    const fileInput = document.createElement('input');
    fileInput.type = 'file';
    fileInput.accept = isPostgres ? '.dump' : '.db';
    fileInput.addEventListener('change', async (e) => {
      const dbFile = (e.target as HTMLInputElement).files?.[0];
      if (!dbFile) return;

      const formData = new FormData();
      formData.append('db', dbFile);

      onClose();
      onBusy({ busy: true, tip: `${t('pages.index.importDatabase')}…` });

      const upload = await HttpUtil.post('/panel/api/server/importDB', formData, {
        headers: { 'Content-Type': 'multipart/form-data' },
      });
      if (!upload?.success) {
        onBusy({ busy: false });
        return;
      }

      onBusy({ busy: true, tip: `${t('pages.settings.restartPanel')}…` });
      const restart = await HttpUtil.post('/panel/api/setting/restartPanel');
      if (restart?.success) {
        await PromiseUtil.sleep(5000);
        window.location.reload();
      } else {
        onBusy({ busy: false });
      }
    });
    fileInput.click();
  }

  return (
    <Modal
      open={open}
      title={t('pages.index.backupTitle')}
      footer={null}
      onCancel={onClose}
    >
      {isPostgres && (
        <div className="backup-description" style={{ marginBottom: 16 }}>
          {t('pages.index.backupPostgresNote')}
        </div>
      )}
      <div className="backup-list">
        <div className="backup-item">
          <div className="backup-meta">
            <div className="backup-title">{t('pages.index.exportDatabase')}</div>
            <div className="backup-description">
              {isPostgres ? t('pages.index.exportDatabasePgDesc') : t('pages.index.exportDatabaseDesc')}
            </div>
          </div>
          <Button type="primary" onClick={exportDb} icon={<DownloadOutlined />} />
        </div>

        <div className="backup-item">
          <div className="backup-meta">
            <div className="backup-title">{t('pages.index.migrationDownload')}</div>
            <div className="backup-description">
              {isPostgres ? t('pages.index.migrationDownloadPgDesc') : t('pages.index.migrationDownloadDesc')}
            </div>
          </div>
          <Button type="primary" onClick={exportMigration} icon={<DownloadOutlined />} />
        </div>

        <div className="backup-item">
          <div className="backup-meta">
            <div className="backup-title">{t('pages.index.importDatabase')}</div>
            <div className="backup-description">
              {isPostgres ? t('pages.index.importDatabasePgDesc') : t('pages.index.importDatabaseDesc')}
            </div>
          </div>
          <Button type="primary" onClick={importDb} icon={<UploadOutlined />} />
        </div>

        <div className="backup-item">
          <div className="backup-meta">
            <div className="backup-title">{t('pages.settings.cloudBackupTitle', 'Cloud Backup (S3 / R2 / WebDAV)')}</div>
            <div className="backup-description">
              {t('pages.settings.cloudBackupEnableDesc', 'Upload database snapshot to configured cloud storage and prune older backups.')}
            </div>
          </div>
          <Button type="primary" loading={cloudLoading} onClick={backupToCloud} icon={<CloudUploadOutlined />} />
        </div>
      </div>
    </Modal>
  );
}
