import { z } from 'zod';

// L2TP/IPsec is served by the host's strongSwan + xl2tpd/PPP daemons, not by
// Xray. Client accounts are the panel's normal clients and are attached to
// this singleton inbound; the backend reads email as the PPP username and
// Password as the MS-CHAPv2 password.
export const L2tpInboundSettingsSchema = z.object({
  psk: z.string().default(''),
  poolCIDR: z.string().default('10.252.0.0/24'),
  localIP: z.string().default('10.252.0.1'),
  poolStart: z.string().default('10.252.0.10'),
  poolEnd: z.string().default('10.252.0.250'),
  dns1: z.string().default('1.1.1.1'),
  dns2: z.string().default('8.8.8.8'),
  outboundInterface: z.string().default(''),
  // This is a connection hint shown by the panel. L2TP has no profile file
  // or server-side route-push extension; clients enable full tunnel in their
  // native L2TP settings. The server always enables forwarding/NAT rules.
  redirectGateway: z.boolean().default(true),
  clients: z.array(z.any()).optional(),
});
export type L2tpInboundSettings = z.infer<typeof L2tpInboundSettingsSchema>;
