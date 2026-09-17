import { z } from 'zod';

// OpenVPN inbound. Served by a local openvpn daemon process, not Xray — so it
// has no Xray clients array and no stream settings (the daemon binds the
// inbound port directly with its own transport). These fields map onto the
// generated openvpn.conf; booleans default on server-side when omitted.
export const OpenvpnInboundSettingsSchema = z.object({
  // Transport the daemon listens on: "udp" (default) or "tcp".
  proto: z.enum(['udp', 'tcp']).default('udp'),
  // Push "redirect-gateway def1 bypass-dns" (full tunnel). Default true.
  redirectGateway: z.boolean().default(true),
  // Push configured DNS servers to the clients. Default true.
  pushDNS: z.boolean().default(true),
  dns1: z.string().default('1.1.1.1'),
  dns2: z.string().default('8.8.8.8'),
});
export type OpenvpnInboundSettings = z.infer<typeof OpenvpnInboundSettingsSchema>;
