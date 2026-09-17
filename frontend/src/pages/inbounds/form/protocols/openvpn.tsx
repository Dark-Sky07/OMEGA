import { useTranslation } from 'react-i18next';
import { Alert, Form, Input, Select, Switch } from 'antd';

// OpenVPN inbound options. The openvpn daemon binds the inbound port
// directly (no Xray transport), so this block only carries the daemon's own
// knobs. Client certificates are generated server-side from the attached
// clients' emails — nothing to enter per client.
export default function OpenvpnFields() {
  const { t } = useTranslation();
  const pushDNS = Form.useWatch(['settings', 'pushDNS']) as boolean | undefined;
  return (
    <>
      <Alert
        type="info"
        showIcon
        message={t('pages.inbounds.form.openvpnDaemonHint')}
        className="mb-12"
      />
      <Form.Item
        name={['settings', 'proto']}
        label={t('pages.inbounds.form.openvpnProto')}
        tooltip={t('pages.inbounds.form.openvpnProtoHint')}
      >
        <Select
          options={[
            { value: 'udp', label: 'UDP' },
            { value: 'tcp', label: 'TCP' },
          ]}
        />
      </Form.Item>
      <Form.Item
        name={['settings', 'redirectGateway']}
        label={t('pages.inbounds.form.openvpnRedirectGateway')}
        tooltip={t('pages.inbounds.form.openvpnRedirectGatewayHint')}
        valuePropName="checked"
      >
        <Switch />
      </Form.Item>
      <Form.Item
        name={['settings', 'pushDNS']}
        label={t('pages.inbounds.form.openvpnPushDNS')}
        valuePropName="checked"
      >
        <Switch />
      </Form.Item>
      {pushDNS !== false && (
        <>
          <Form.Item name={['settings', 'dns1']} label={t('pages.inbounds.form.openvpnDns1')}>
            <Input placeholder="1.1.1.1" />
          </Form.Item>
          <Form.Item name={['settings', 'dns2']} label={t('pages.inbounds.form.openvpnDns2')}>
            <Input placeholder="8.8.8.8" />
          </Form.Item>
        </>
      )}
    </>
  );
}
