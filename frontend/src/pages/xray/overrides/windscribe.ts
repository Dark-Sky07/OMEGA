export interface WindscribePeer {
  publicKey: string;
  preSharedKey?: string;
  allowedIPs: string[];
  endpoint: string;
  keepAlive?: number;
}

export interface WindscribeOutboundSettings {
  secretKey: string;
  address: string[];
  remoteDNS: string[];
  peers: WindscribePeer[];
  mtu?: number;
  domainStrategy: 'ForceIPv4v6';
  noKernelTun: true;
}

export interface ParsedWindscribeConfig {
  tag: string;
  protocol: 'wireguard';
  settings: WindscribeOutboundSettings;
}

export class WindscribeConfigError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'WindscribeConfigError';
  }
}

function keyLooksBase64(value: string): boolean {
  return /^(?:[A-Za-z0-9+/]{42,43}=?)$/.test(value);
}

function validAddress(value: string): boolean {
  const [host, prefixText] = value.trim().split('/');
  if (!host || prefixText == null || !/^\d+$/.test(prefixText)) return false;
  const prefix = Number(prefixText);
  if (host.includes(':')) {
    return prefix >= 0 && prefix <= 128 && /^[0-9a-f:]+$/i.test(host);
  }
  const octets = host.split('.');
  return prefix >= 0 && prefix <= 32 && octets.length === 4 && octets.every((part) => {
    const n = Number(part);
    return /^\d{1,3}$/.test(part) && n >= 0 && n <= 255;
  });
}

function validHost(host: string): boolean {
  if (!host || /[\s/]/.test(host)) return false;
  if (host.includes(':')) return /^[0-9a-f:]+$/i.test(host);
  return /^[A-Za-z0-9._-]+$/.test(host);
}

function validateEndpoint(raw: string): string {
  const endpoint = raw.trim();
  let host: string;
  let portText: string;
  if (endpoint.startsWith('[')) {
    const close = endpoint.indexOf(']');
    if (close < 0 || endpoint[close + 1] !== ':') throw new WindscribeConfigError('Peer endpoint must contain a port');
    host = endpoint.slice(1, close);
    portText = endpoint.slice(close + 2);
  } else {
    const colon = endpoint.lastIndexOf(':');
    if (colon <= 0) throw new WindscribeConfigError('Peer endpoint must contain a port');
    host = endpoint.slice(0, colon);
    portText = endpoint.slice(colon + 1);
  }
  const port = Number(portText);
  if (!validHost(host) || !Number.isInteger(port) || port < 1 || port > 65535) {
    throw new WindscribeConfigError('Peer endpoint is invalid');
  }
  return endpoint;
}

function stripComment(line: string): string {
  // WireGuard keys and endpoints do not use # or ;, so inline comments can
  // be removed safely after trimming whitespace.
  return line.replace(/[;#].*$/, '').trim();
}

function readValues(text: string): { iface: Record<string, string>; peers: Record<string, string>[] } {
  const iface: Record<string, string> = {};
  const peers: Record<string, string>[] = [];
  let section: 'interface' | 'peer' | '' = '';
  for (const original of text.replace(/^\uFEFF/, '').split(/\r?\n/)) {
    const line = stripComment(original);
    if (!line) continue;
    const sectionMatch = line.match(/^\[\s*(Interface|Peer)\s*\]$/i);
    if (sectionMatch) {
      section = sectionMatch[1].toLowerCase() as 'interface' | 'peer';
      if (section === 'peer') peers.push({});
      continue;
    }
    const separator = line.indexOf('=');
    if (separator <= 0 || !section) throw new WindscribeConfigError('Every WireGuard setting must be inside Interface or Peer');
    const key = line.slice(0, separator).trim().toLowerCase();
    const value = line.slice(separator + 1).trim();
    if (!value) throw new WindscribeConfigError(`WireGuard setting ${key} is empty`);
    if (section === 'interface') iface[key] = value;
    else peers[peers.length - 1][key] = value;
  }
  return { iface, peers };
}

function splitList(value: string | undefined): string[] {
  return (value || '').split(',').map((item) => item.trim()).filter(Boolean);
}

function tagFromEndpoint(endpoint: string): string {
  const withoutPort = endpoint.startsWith('[')
    ? endpoint.slice(1, endpoint.indexOf(']'))
    : endpoint.slice(0, endpoint.lastIndexOf(':'));
  const safe = withoutPort.replace(/[^A-Za-z0-9_-]+/g, '-').replace(/^-+|-+$/g, '');
  return `windscribe-${safe || 'wireguard'}`.slice(0, 64);
}

/**
 * Parse a Windscribe WireGuard profile without ever printing its secret key.
 * The returned object is directly consumable by the WireGuard outbound form.
 */
export function parseWindscribeConfig(text: string, fileName = ''): ParsedWindscribeConfig {
  if (!text.trim()) throw new WindscribeConfigError('WireGuard profile is empty');
  const { iface, peers: rawPeers } = readValues(text);
  const secretKey = iface.privatekey;
  if (!secretKey || !keyLooksBase64(secretKey)) throw new WindscribeConfigError('Interface PrivateKey is missing or invalid');

  const address = splitList(iface.address);
  if (address.length === 0 || address.some((item) => !validAddress(item))) {
    throw new WindscribeConfigError('Interface Address must contain valid IPv4 or IPv6 CIDR addresses');
  }
  const dns = splitList(iface.dns);
  if (dns.some((item) => !validAddress(`${item}/32`) && !validAddress(`${item}/128`))) {
    throw new WindscribeConfigError('DNS must contain valid IPv4 or IPv6 addresses');
  }

  const mtu = iface.mtu == null ? undefined : Number(iface.mtu);
  if (mtu != null && (!Number.isInteger(mtu) || mtu < 576 || mtu > 65535)) {
    throw new WindscribeConfigError('MTU must be an integer between 576 and 65535');
  }
  if (rawPeers.length === 0) throw new WindscribeConfigError('At least one WireGuard Peer is required');

  const peers: WindscribePeer[] = rawPeers.map((raw, index) => {
    const publicKey = raw.publickey;
    if (!publicKey || !keyLooksBase64(publicKey)) throw new WindscribeConfigError(`Peer ${index + 1} PublicKey is missing or invalid`);
    const endpoint = validateEndpoint(raw.endpoint || '');
    const allowedIPs = splitList(raw.allowedips);
    if (allowedIPs.length === 0 || allowedIPs.some((item) => !validAddress(item))) {
      throw new WindscribeConfigError(`Peer ${index + 1} AllowedIPs is missing or invalid`);
    }
    const keepAlive = raw.persistentkeepalive == null ? undefined : Number(raw.persistentkeepalive);
    if (keepAlive != null && (!Number.isInteger(keepAlive) || keepAlive < 0 || keepAlive > 65535)) {
      throw new WindscribeConfigError(`Peer ${index + 1} PersistentKeepalive is invalid`);
    }
    const psk = raw.presharedkey;
    if (psk && !keyLooksBase64(psk)) throw new WindscribeConfigError(`Peer ${index + 1} PresharedKey is invalid`);
    return {
      publicKey,
      preSharedKey: psk || undefined,
      allowedIPs,
      endpoint,
      keepAlive,
    };
  });

  const tag = fileName
    ? `windscribe-${fileName.replace(/\.[^.]+$/, '').replace(/[^A-Za-z0-9_-]+/g, '-').slice(0, 48)}`
    : tagFromEndpoint(peers[0].endpoint);
  return {
    tag: tag.replace(/-+$/, '') || 'windscribe-wireguard',
    protocol: 'wireguard',
    settings: {
      secretKey,
      address,
      remoteDNS: dns,
      peers,
      mtu,
      domainStrategy: 'ForceIPv4v6',
      noKernelTun: true,
    },
  };
}
