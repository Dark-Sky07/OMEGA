import { useTranslation } from 'react-i18next';
import { Form, Input, Select, Switch } from 'antd';

import { HeaderMapEditor } from '@/components/form';
import { MASQUE_DEFAULT_PATH } from '@/schemas/protocols/stream/masque';

const BASE = ['streamSettings', 'masqueSettings'] as const;

export default function MasqueForm() {
  const { t } = useTranslation();
  const form = Form.useFormInstance();
  const warp = Form.useWatch([...BASE, 'warp'], form);
  const warpOn = warp != null;

  return (
    <>
      <Form.Item label={t('host')} name={[...BASE, 'host']}>
        <Input placeholder={warpOn ? 'cloudflareaccess.com' : undefined} />
      </Form.Item>
      <Form.Item label={t('path')} name={[...BASE, 'path']}>
        <Input placeholder={warpOn ? '/' : MASQUE_DEFAULT_PATH} />
      </Form.Item>
      <Form.Item label="WARP" tooltip={t('pages.xray.outboundForm.masqueWarpDesc')}>
        <Switch
          checked={warpOn}
          onChange={(on) => {
            form.setFieldValue(
              [...BASE, 'warp'],
              on ? { privateKey: '', publicKey: '', address: [] } : undefined,
            );
            if (on) {
              form.setFieldValue([...BASE, 'user'], undefined);
              form.setFieldValue([...BASE, 'pass'], undefined);
            }
          }}
        />
      </Form.Item>
      {warpOn ? (
        <>
          <Form.Item
            label={t('pages.xray.warp.privateKey')}
            name={[...BASE, 'warp', 'privateKey']}
            required
          >
            <Input.TextArea
              autoSize={{ minRows: 2, maxRows: 6 }}
              placeholder="-----BEGIN PRIVATE KEY-----"
            />
          </Form.Item>
          <Form.Item
            label={t('pages.xray.outboundForm.masqueWarpPublicKey')}
            name={[...BASE, 'warp', 'publicKey']}
            required
          >
            <Input.TextArea
              autoSize={{ minRows: 2, maxRows: 6 }}
              placeholder="-----BEGIN PUBLIC KEY-----"
            />
          </Form.Item>
          <Form.Item
            label={t('pages.xray.outboundForm.masqueWarpAddress')}
            name={[...BASE, 'warp', 'address']}
            required
          >
            <Select
              mode="tags"
              tokenSeparators={[',', ' ']}
              open={false}
              placeholder="172.16.0.2, 2606:4700:110:8a36::2"
            />
          </Form.Item>
        </>
      ) : (
        <>
          <Form.Item label={t('username')} name={[...BASE, 'user']}>
            <Input />
          </Form.Item>
          <Form.Item label={t('password')} name={[...BASE, 'pass']}>
            <Input.Password />
          </Form.Item>
        </>
      )}
      <Form.Item label={t('pages.inbounds.form.headers')} name={[...BASE, 'headers']}>
        <HeaderMapEditor mode="v1" />
      </Form.Item>
    </>
  );
}
