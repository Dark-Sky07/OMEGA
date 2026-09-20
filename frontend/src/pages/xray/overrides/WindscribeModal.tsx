import { useMemo, useState } from 'react';
import { Alert, Button, Input, Modal, Space, Statistic, Upload } from 'antd';
import type { UploadFile } from 'antd';
import { InboxOutlined, SaveOutlined } from '@ant-design/icons';

import {
  parseWindscribeConfig,
  type ParsedWindscribeConfig,
} from './windscribe';

const { Dragger } = Upload;

interface WindscribeModalProps {
  open: boolean;
  templateSettings: { outbounds?: { tag?: string }[] } | null;
  onClose: () => void;
  onAddOutbound: (outbound: Record<string, unknown>) => void;
  onResetOutbound: (payload: { index: number; outbound: Record<string, unknown>; oldTag?: string; newTag: string }) => void;
}

export default function WindscribeModal({
  open,
  templateSettings,
  onClose,
  onAddOutbound,
  onResetOutbound,
}: WindscribeModalProps) {
  const [fileList, setFileList] = useState<UploadFile[]>([]);
  const [parsed, setParsed] = useState<ParsedWindscribeConfig | null>(null);
  const [tag, setTag] = useState('');
  const [error, setError] = useState('');

  const existingIndex = useMemo(() => {
    const list = templateSettings?.outbounds || [];
    return list.findIndex((item) => item?.tag?.startsWith?.('windscribe-'));
  }, [templateSettings?.outbounds]);

  function resetFile() {
    setFileList([]);
    setParsed(null);
    setTag('');
    setError('');
  }

  function readFile(file: File) {
    setError('');
    const reader = new FileReader();
    reader.onload = () => {
      try {
        const next = parseWindscribeConfig(String(reader.result || ''), file.name);
        setParsed(next);
        setTag(next.tag);
      } catch (cause) {
        setParsed(null);
        setTag('');
        setError(cause instanceof Error ? cause.message : 'Invalid WireGuard profile');
      }
    };
    reader.onerror = () => setError('The selected file could not be read');
    reader.readAsText(file);
  }

  function save() {
    if (!parsed || !tag.trim()) {
      setError('Choose a valid Windscribe profile and enter an outbound tag');
      return;
    }
    const outbound = {
      tag: tag.trim(),
      protocol: parsed.protocol,
      settings: parsed.settings,
    } as Record<string, unknown>;
    if (existingIndex >= 0) {
      const oldTag = templateSettings?.outbounds?.[existingIndex]?.tag;
      onResetOutbound({ index: existingIndex, outbound, oldTag, newTag: tag.trim() });
    } else {
      onAddOutbound(outbound);
    }
    resetFile();
    onClose();
  }

  return (
    <Modal
      open={open}
      title="Import Windscribe WireGuard"
      onCancel={() => { resetFile(); onClose(); }}
      footer={[
        <Button key="cancel" onClick={() => { resetFile(); onClose(); }}>Cancel</Button>,
        <Button key="save" type="primary" icon={<SaveOutlined />} disabled={!parsed} onClick={save}>
          {existingIndex >= 0 ? 'Update outbound' : 'Add outbound'}
        </Button>,
      ]}
      destroyOnClose
    >
      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        <Alert
          type="info"
          showIcon
          message="The private key is parsed locally and is never included in logs or import reports."
        />
        <Dragger
          accept=".conf,.txt"
          maxCount={1}
          fileList={fileList}
          beforeUpload={(file) => {
            setFileList([file]);
            readFile(file);
            return false;
          }}
          onRemove={resetFile}
        >
          <p className="ant-upload-drag-icon"><InboxOutlined /></p>
          <p className="ant-upload-text">Click or drag a Windscribe WireGuard profile here</p>
          <p className="ant-upload-hint">Supports IPv4/IPv6 addresses, DNS, multiple peers, endpoints and keepalive.</p>
        </Dragger>
        {error && <Alert type="error" showIcon message={error} />}
        {parsed && (
          <>
            <Input
              addonBefore="Tag"
              value={tag}
              maxLength={64}
              onChange={(event) => setTag(event.target.value)}
            />
            <Space wrap>
              <Statistic title="Addresses" value={parsed.settings.address.length} />
              <Statistic title="DNS" value={parsed.settings.remoteDNS.length} />
              <Statistic title="Peers" value={parsed.settings.peers.length} />
            </Space>
          </>
        )}
      </Space>
    </Modal>
  );
}
