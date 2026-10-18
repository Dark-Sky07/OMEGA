import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Form, Input, InputNumber, Select, Typography } from 'antd';

import { JsonEditor } from '@/components/form';
import { XDriveServiceSchema } from '@/schemas/protocols/stream/xdrive';

const BASE = ['streamSettings', 'xdriveSettings'] as const;

const SERVICE_LABEL_KEYS: Partial<Record<string, string>> = {
  local: 'pages.inbounds.form.xdrive.serviceLocal',
  template: 'pages.inbounds.form.xdrive.serviceTemplate',
};

const TUNING_FIELDS = [
  ['segmentBytes', 'segmentBytes', 524288],
  ['flushIntervalMs', 'flushInterval', 20],
  ['pollIntervalMs', 'pollInterval', 50],
  ['maxPollIntervalMs', 'maxPollInterval', 500],
  ['eagerWindowMs', 'eagerWindow', 2000],
  ['holeTimeoutMs', 'holeTimeout', 30000],
  ['sessionTtlSeconds', 'sessionTtl', 300],
  ['concurrency', 'concurrency', 8],
] as const;

const GOOGLE_DRIVE_SECRETS = ['clientId', 'clientSecret', 'refreshToken'] as const;

function TemplateEditor({
  value,
  onChange,
}: {
  value?: Record<string, unknown>;
  onChange?: (next: Record<string, unknown> | undefined) => void;
}) {
  const { t } = useTranslation();
  const [text, setText] = useState(() => (value ? JSON.stringify(value, null, 2) : ''));
  const [invalid, setInvalid] = useState(false);
  return (
    <>
      <JsonEditor
        value={text}
        minHeight="160px"
        onChange={(next) => {
          setText(next);
          if (next.trim() === '') {
            setInvalid(false);
            onChange?.(undefined);
            return;
          }
          try {
            const parsed: unknown = JSON.parse(next);
            const isObject = !!parsed && typeof parsed === 'object' && !Array.isArray(parsed);
            setInvalid(!isObject);
            if (isObject) onChange?.(parsed as Record<string, unknown>);
          } catch {
            setInvalid(true);
          }
        }}
      />
      {invalid && (
        <Typography.Text type="danger">
          {t('pages.inbounds.form.xdrive.templateInvalid')}
        </Typography.Text>
      )}
    </>
  );
}

export default function XDriveForm() {
  const { t } = useTranslation();
  const form = Form.useFormInstance();
  const service = (Form.useWatch([...BASE, 'service'], form) as string | undefined) ?? 'Google Drive';
  const serviceOptions = XDriveServiceSchema.options.map((value) => {
    const labelKey = SERVICE_LABEL_KEYS[value];
    return { value, label: labelKey ? t(labelKey) : value };
  });

  return (
    <>
      <Form.Item label={t('pages.inbounds.form.xdrive.service')} name={[...BASE, 'service']}>
        <Select options={serviceOptions} />
      </Form.Item>
      <Form.Item
        label={t('pages.inbounds.form.xdrive.remoteFolder')}
        name={[...BASE, 'remoteFolder']}
        required
      >
        <Input placeholder={service === 'local' ? '/var/lib/xdrive' : 'xray-tunnel'} />
      </Form.Item>
      {service === 'Google Drive' &&
        GOOGLE_DRIVE_SECRETS.map((key, index) => (
          <Form.Item
            key={key}
            label={t(`pages.inbounds.form.xdrive.${key}`)}
            name={[...BASE, 'secrets', index]}
            required
          >
            {key === 'clientId' ? <Input /> : <Input.Password />}
          </Form.Item>
        ))}
      {service === 'template' && (
        <>
          <Form.Item
            label={t('pages.inbounds.form.xdrive.secrets')}
            tooltip={t('pages.inbounds.form.xdrive.secretsDesc')}
            name={[...BASE, 'secrets']}
          >
            <Select mode="tags" tokenSeparators={[',']} open={false} />
          </Form.Item>
          <Form.Item
            label={t('pages.inbounds.form.xdrive.template')}
            name={[...BASE, 'template']}
            required
          >
            <TemplateEditor />
          </Form.Item>
        </>
      )}
      {service !== 'local' && (
        <>
          <Form.Item
            label={t('pages.inbounds.form.xdrive.front')}
            tooltip={t('pages.inbounds.form.xdrive.frontDesc')}
            name={['streamSettings', 'address']}
            normalize={(raw: unknown) => String(raw ?? '').trim() || undefined}
          >
            <Input placeholder="www.googleapis.com" />
          </Form.Item>
          <Form.Item
            label={t('pages.inbounds.form.xdrive.frontPort')}
            name={['streamSettings', 'port']}
          >
            <InputNumber min={1} max={65535} placeholder="443" />
          </Form.Item>
        </>
      )}
      {TUNING_FIELDS.map(([key, label, placeholder]) => (
        <Form.Item key={key} label={t(`pages.inbounds.form.xdrive.${label}`)} name={[...BASE, key]}>
          <InputNumber min={0} placeholder={String(placeholder)} style={{ width: '100%' }} />
        </Form.Item>
      ))}
    </>
  );
}
