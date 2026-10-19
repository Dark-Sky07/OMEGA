import { describe, expect, it } from 'vitest';
import { genAllLinks, resolveSmartExternalProxy } from '@/lib/xray/inbound-link';
import { InboundSchema } from '@/schemas/api/inbound';

describe('resolveSmartExternalProxy & genAllLinks', () => {
  const baseInbound = InboundSchema.parse({
    id: 1,
    port: 443,
    protocol: 'vless',
    remark: 'Edge-DE',
    enable: true,
    settings: {
      clients: [
        {
          id: '11111111-1111-1111-1111-111111111111',
          email: 'alice',
          enable: true,
        },
      ],
      decryption: 'none',
    },
    streamSettings: {
      network: 'ws',
      security: 'tls',
      wsSettings: { path: '/ws', headers: {} },
      tlsSettings: { serverName: 'origin.example.com' },
      externalProxy: [
        {
          forceTls: 'tls',
          dest: '*.cdn.example.com',
          port: 2053,
          sni: '{USERNAME}.sni.example.com',
          remark: '{REMARK}-{USERNAME}',
        },
      ],
    },
  });

  it('resolves wildcard *.domain and {USERNAME}/{REMARK} templates', () => {
    const ep = baseInbound.streamSettings!.externalProxy![0];
    const resolved = resolveSmartExternalProxy(ep, baseInbound, 'alice');
    expect(resolved.dest).toMatch(/^[a-z0-9]{6}\.cdn\.example\.com$/);
    expect(resolved.sni).toBe('alice.sni.example.com');
    expect(resolved.remark).toBe('Edge-DE-alice');
  });

  it('keeps paired index when dest and sni are comma-separated pools', () => {
    const ep = {
      forceTls: 'tls' as const,
      dest: 'h1.example.com, h2.example.com',
      port: 443,
      sni: 's1.example.com, s2.example.com',
      remark: 'pool',
    };
    for (let i = 0; i < 15; i++) {
      const resolved = resolveSmartExternalProxy(ep, baseInbound, 'bob');
      if (resolved.dest === 'h1.example.com') {
        expect(resolved.sni).toBe('s1.example.com');
      } else {
        expect(resolved.dest).toBe('h2.example.com');
        expect(resolved.sni).toBe('s2.example.com');
      }
    }
  });

  it('integrates into genAllLinks without leaving raw *.wildcard', () => {
    const entries = genAllLinks({
      inbound: baseInbound,
      remark: 'Edge-DE',
      client: {
        id: '11111111-1111-1111-1111-111111111111',
        email: 'alice',
      },
      fallbackHostname: 'panel.example.com',
    });
    expect(entries).toHaveLength(1);
    expect(entries[0].link).toContain('.cdn.example.com:2053');
    expect(entries[0].link).not.toContain('*.cdn.example.com');
    expect(entries[0].link).toContain('sni=alice.sni.example.com');
  });
});
