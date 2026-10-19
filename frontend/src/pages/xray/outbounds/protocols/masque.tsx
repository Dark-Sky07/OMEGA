import { useTranslation } from 'react-i18next';
import { Form, Select } from 'antd';

export default function MasqueFields() {
  const { t } = useTranslation();
  return (
    <Form.Item label={t('pages.xray.outboundForm.remoteDNS')} name={['settings', 'remoteDNS']}>
      <Select
        mode="tags"
        tokenSeparators={[',', ' ']}
        open={false}
        placeholder="1.1.1.1, 2606:4700:4700::1111"
      />
    </Form.Item>
  );
}
