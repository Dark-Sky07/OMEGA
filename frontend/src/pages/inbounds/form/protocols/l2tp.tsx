import { useTranslation } from 'react-i18next';
import { Alert, Form, Input, Select, Switch } from 'antd';
import { useOutboundTags } from '@/api/queries/useOutboundTags';

// L2TP/IPsec has a single global server listener. Client accounts are managed
// by the existing clients page; the inbound form only owns daemon/network
// parameters and displays them later in the inbound info modal.
export default function L2tpFields() {
  const { t } = useTranslation();
  const form = Form.useFormInstance();
  const routeThroughXray = Form.useWatch(['settings', 'routeThroughXray'], form) as boolean | undefined;
  const { data: outboundTags } = useOutboundTags();
  return (
    <>
      <Alert
        type="info"
        showIcon
        message={t('pages.inbounds.form.l2tpDaemonHint')}
        className="mb-12"
      />
      <Form.Item
        name={['settings', 'psk']}
        label={t('pages.inbounds.form.l2tpPsk')}
        rules={[{ required: true, message: t('pages.inbounds.form.l2tpPskRequired') }]}
        extra={t('pages.inbounds.form.l2tpPskHint')}
      >
        <Input.Password visibilityToggle />
      </Form.Item>
      <Form.Item
        name={['settings', 'poolCIDR']}
        label={t('pages.inbounds.form.l2tpPoolCIDR')}
        extra={t('pages.inbounds.form.l2tpPoolCIDRHint')}
      >
        <Input placeholder="10.252.0.0/24" />
      </Form.Item>
      <Form.Item name={['settings', 'localIP']} label={t('pages.inbounds.form.l2tpLocalIP')}>
        <Input placeholder="10.252.0.1" />
      </Form.Item>
      <Form.Item name={['settings', 'poolStart']} label={t('pages.inbounds.form.l2tpPoolStart')}>
        <Input placeholder="10.252.0.10" />
      </Form.Item>
      <Form.Item name={['settings', 'poolEnd']} label={t('pages.inbounds.form.l2tpPoolEnd')}>
        <Input placeholder="10.252.0.250" />
      </Form.Item>
      <Form.Item name={['settings', 'dns1']} label={t('pages.inbounds.form.l2tpDns1')}>
        <Input placeholder="1.1.1.1" />
      </Form.Item>
      <Form.Item name={['settings', 'dns2']} label={t('pages.inbounds.form.l2tpDns2')}>
        <Input placeholder="8.8.8.8" />
      </Form.Item>
      <Form.Item
        name={['settings', 'outboundInterface']}
        label={t('pages.inbounds.form.l2tpOutboundInterface')}
        extra={t('pages.inbounds.form.l2tpOutboundInterfaceHint')}
      >
        <Input placeholder="eth0" />
      </Form.Item>
      <Form.Item
        name={['settings', 'redirectGateway']}
        label={t('pages.inbounds.form.l2tpFullTunnelHint')}
        valuePropName="checked"
        extra={t('pages.inbounds.form.l2tpFullTunnelExtra')}
      >
        <Switch />
      </Form.Item>
      <Form.Item
        name={['settings', 'routeThroughXray']}
        label={t('pages.inbounds.form.daemonRouteThroughXray')}
        tooltip={t('pages.inbounds.form.daemonRouteThroughXrayHint')}
        valuePropName="checked"
      >
        <Switch />
      </Form.Item>
      {routeThroughXray && (
        <Form.Item
          name={['settings', 'outboundTag']}
          label={t('pages.inbounds.form.mtgRouteOutbound')}
          tooltip={t('pages.inbounds.form.mtgRouteOutboundHint')}
        >
          <Select
            allowClear
            showSearch
            placeholder={t('pages.inbounds.form.mtgRouteOutboundPlaceholder')}
            options={(outboundTags ?? []).map((tag) => ({ value: tag, label: tag }))}
          />
        </Form.Item>
      )}
    </>
  );
}
