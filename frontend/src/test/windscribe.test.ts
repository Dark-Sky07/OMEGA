import { describe, expect, it } from 'vitest';

import { parseWindscribeConfig } from '@/pages/xray/overrides/windscribe';

const key = (letter: string) => `${letter.repeat(43)}=`;

const profile = `[Interface]
PrivateKey = ${key('A')}
Address = 10.20.30.4/32, fd00:1234::4/128
DNS = 10.255.255.3, fd00::53
MTU = 1420

[Peer]
PublicKey = ${key('B')}
PresharedKey = ${key('C')}
AllowedIPs = 0.0.0.0/0, ::/0
Endpoint = [2001:db8::10]:443
PersistentKeepalive = 25
`;

describe('parseWindscribeConfig', () => {
  it('maps Windscribe fields into a WireGuard outbound without exposing secrets', () => {
    const parsed = parseWindscribeConfig(profile, 'windscribe-us.conf');
    expect(parsed.protocol).toBe('wireguard');
    expect(parsed.settings.address).toEqual(['10.20.30.4/32', 'fd00:1234::4/128']);
    expect(parsed.settings.remoteDNS).toEqual(['10.255.255.3', 'fd00::53']);
    expect(parsed.settings.peers[0]).toMatchObject({
      publicKey: key('B'),
      preSharedKey: key('C'),
      endpoint: '[2001:db8::10]:443',
      keepAlive: 25,
    });
    expect(parsed.settings.noKernelTun).toBe(true);
  });

  it('rejects a profile with a missing or malformed private key', () => {
    expect(() => parseWindscribeConfig(profile.replace(key('A'), 'not-a-key'))).toThrow(/PrivateKey/);
  });

  it('rejects a peer endpoint without a port', () => {
    expect(() => parseWindscribeConfig(profile.replace('[2001:db8::10]:443', 'vpn.example.com'))).toThrow(/endpoint/i);
  });
});
