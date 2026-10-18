import { useTranslation } from 'react-i18next';
import { Form, Input, InputNumber, Select } from 'antd';

import { MASQUE_DEFAULT_PATH } from '@/schemas/protocols/stream/masque';

export function MasqueSettingsFields() {
  const { t } = useTranslation();
  return (
    <>
      <Form.Item
        label={t('pages.inbounds.form.masque.addressPool')}
        tooltip={t('pages.inbounds.form.masque.addressPoolDesc')}
        name={['settings', 'address']}
        required
      >
        <Select mode="tags" tokenSeparators={[',', ' ']} open={false} />
      </Form.Item>
      <Form.Item label={t('pages.inbounds.info.mtu')} name={['settings', 'mtu']}>
        <InputNumber min={1280} max={65535} placeholder="1280" style={{ width: '100%' }} />
      </Form.Item>
    </>
  );
}

export default function MasqueStreamFields() {
  const { t } = useTranslation();
  return (
    <Form.Item label={t('path')} name={['streamSettings', 'masqueSettings', 'path']}>
      <Input placeholder={MASQUE_DEFAULT_PATH} />
    </Form.Item>
  );
}
