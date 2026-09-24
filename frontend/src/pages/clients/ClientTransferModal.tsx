import { useEffect, useState } from 'react';
import { Alert, Button, Checkbox, Modal, Space, Statistic, Upload } from 'antd';
import type { UploadFile } from 'antd';
import { InboxOutlined, UploadOutlined } from '@ant-design/icons';

import { HttpUtil } from '@/utils';

const { Dragger } = Upload;

interface TransferReport {
  total?: number;
  created?: number;
  updated?: number;
  skipped?: number;
  failed?: number;
  errors?: Array<{ index?: number; email?: string; error?: string }>;
}

interface ClientTransferModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onImported: () => void;
}

export default function ClientTransferModal({ open, onOpenChange, onImported }: ClientTransferModalProps) {
  const [fileList, setFileList] = useState<UploadFile[]>([]);
  const [payload, setPayload] = useState<unknown>(null);
  const [parseError, setParseError] = useState('');
  const [report, setReport] = useState<TransferReport | null>(null);
  const [replaceAttachments, setReplaceAttachments] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (!open) {
      setFileList([]);
      setPayload(null);
      setParseError('');
      setReport(null);
      setReplaceAttachments(false);
      setSubmitting(false);
    }
  }, [open]);

  function readFile(file: File) {
    setParseError('');
    setReport(null);
    if (file.size > 25 * 1024 * 1024) {
      setPayload(null);
      setParseError('The transfer file is larger than 25 MB.');
      return;
    }
    const reader = new FileReader();
    reader.onload = () => {
      try {
        const parsed = JSON.parse(String(reader.result || ''));
        setPayload(parsed);
      } catch {
        setPayload(null);
        setParseError('The selected file is not valid JSON.');
      }
    };
    reader.onerror = () => {
      setPayload(null);
      setParseError('The selected file could not be read.');
    };
    reader.readAsText(file);
  }

  async function submit() {
    if (!payload) {
      setParseError('Choose a valid OMEGA client transfer file first.');
      return;
    }
    setSubmitting(true);
    try {
      const requestPayload = payload && typeof payload === 'object' && !Array.isArray(payload)
        ? { ...(payload as Record<string, unknown>), replaceAttachments }
        : payload;
      const msg = await HttpUtil.post<TransferReport>('/panel/api/clients/import', requestPayload);
      const nextReport = (msg.obj || {}) as TransferReport;
      setReport(nextReport);
      if (msg.success && (nextReport.failed || 0) === 0) {
        onImported();
        onOpenChange(false);
      }
    } finally {
      setSubmitting(false);
    }
  }

  const errors = report?.errors || [];
  return (
    <Modal
      open={open}
      title="Import clients"
      onCancel={() => onOpenChange(false)}
      footer={[
        <Button key="cancel" onClick={() => onOpenChange(false)}>Cancel</Button>,
        <Button key="import" type="primary" icon={<UploadOutlined />} loading={submitting} onClick={submit}>
          Import
        </Button>,
      ]}
      destroyOnClose
    >
      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        <Alert
          type="info"
          showIcon
          message="The file is validated before any client or inbound is changed. Existing traffic usage is not imported."
        />
        <Checkbox checked={replaceAttachments} onChange={(event) => setReplaceAttachments(event.target.checked)}>
          Replace existing client-to-inbound attachments not listed in the file
        </Checkbox>
        <Dragger
          accept=".json,application/json"
          maxCount={1}
          fileList={fileList}
          beforeUpload={(file) => {
            setFileList([file]);
            readFile(file);
            return false;
          }}
          onRemove={() => {
            setFileList([]);
            setPayload(null);
            setParseError('');
            setReport(null);
            setReplaceAttachments(false);
          }}
        >
          <p className="ant-upload-drag-icon"><InboxOutlined /></p>
          <p className="ant-upload-text">Click or drag an OMEGA JSON export here</p>
          <p className="ant-upload-hint">Only client configuration and authorized inbound attachments are transferred.</p>
        </Dragger>
        {parseError && <Alert type="error" showIcon message={parseError} />}
        {report && (
          <>
            <Space wrap>
              <Statistic title="Created" value={report.created || 0} />
              <Statistic title="Updated" value={report.updated || 0} />
              <Statistic title="Failed" value={report.failed || 0} />
            </Space>
            {errors.length > 0 && (
              <Alert
                type="warning"
                showIcon
                message="Some entries were not imported"
                description={(
                  <div style={{ maxHeight: 180, overflow: 'auto' }}>
                    {errors.map((item, index) => (
                      <div key={`${item.index ?? index}-${item.email ?? ''}`}>
                        {item.email ? `${item.email}: ` : ''}{item.error || 'Unknown error'}
                      </div>
                    ))}
                  </div>
                )}
              />
            )}
          </>
        )}
      </Space>
    </Modal>
  );
}
