/**
 * Browser-safe core of the OMEGA mock panel API (dev/preview only, never shipped).
 *
 * `route(method, path, query)` answers every panel endpoint with the
 * `{ success, msg, obj }` envelope. It has no Node dependencies so the same
 * data can back both `scripts/mock-api.mjs` (local Vite proxy target) and the
 * static design preview, where `src/preview/mock-bootstrap.ts` wires it into
 * axios directly inside the browser.
 */
function randomBytes(n) {
  const buf = new Uint8Array(n);
  globalThis.crypto.getRandomValues(buf);
  return buf;
}
const randomHex = (n) => Array.from(randomBytes(n), (b) => b.toString(16).padStart(2, '0')).join('');
const randomBase64 = (n) => btoa(String.fromCharCode(...randomBytes(n)));

const START = Date.now();

const ok = (obj = null, msg = '') => ({ success: true, msg, obj });
const rnd = (min, max) => min + Math.random() * (max - min);
const GB = 1024 ** 3;

const DAY = 86400 * 1000;
const reseller = (id, username, name, enable, trafficLimit, clientLimit, expiryTime, extra) => ({
  reseller: { id, username, name, comment: '', enable, trafficLimit, clientLimit, expiryTime, createdAt: START - 90 * DAY, updatedAt: START - DAY },
  inboundCount: 2, clientCount: 0, onlineCount: 0, usedTraffic: 0, allocatedTraffic: 0, remainingTraffic: 0,
  expired: false, overQuota: false, disabled: !enable,
  ...extra,
});
const RESELLERS = [
  reseller(1, 'ali', 'Ali Store', true, 2000 * GB, 100, START + 40 * DAY, { clientCount: 31, onlineCount: 9, usedTraffic: 1260 * GB, allocatedTraffic: 1800 * GB, remainingTraffic: 740 * GB }),
  reseller(2, 'sara', 'Sara VPN', true, 500 * GB, 40, START + 6 * DAY, { clientCount: 18, onlineCount: 4, usedTraffic: 470 * GB, allocatedTraffic: 500 * GB, remainingTraffic: 30 * GB }),
  reseller(3, 'nima', 'Nima Shop', true, 300 * GB, 20, START - 2 * DAY, { clientCount: 11, onlineCount: 0, usedTraffic: 305 * GB, allocatedTraffic: 300 * GB, remainingTraffic: 0, expired: true, overQuota: true }),
  reseller(4, 'demo', 'Demo (disabled)', false, 0, 0, 0, { clientCount: 3, onlineCount: 0, usedTraffic: 12 * GB, allocatedTraffic: 50 * GB, remainingTraffic: 0 }),
];
const NODES = [
  { id: 1, name: 'fin-1', remark: 'Helsinki edge', scheme: 'https', address: 'fin1.example.net', port: 2053, basePath: '/', enable: true, status: 'online', latencyMs: 24, cpuPct: 31, memPct: 58, xrayVersion: '25.9.11', panelVersion: '4.0.0-omega', uptimeSecs: 86400 * 12, inboundCount: 3, clientCount: 140, onlineCount: 37, depletedCount: 4, lastHeartbeat: Math.floor(START / 1000), xrayState: 'running', inboundSyncMode: 'all', inboundTags: [], guid: 'n-fin-1' },
  { id: 2, name: 'de-1', remark: 'Frankfurt', scheme: 'https', address: 'de1.example.net', port: 2053, basePath: '/', enable: true, status: 'online', latencyMs: 41, cpuPct: 72, memPct: 81, xrayVersion: '25.9.11', panelVersion: '3.3.36-omega', uptimeSecs: 86400 * 3, inboundCount: 2, clientCount: 96, onlineCount: 22, depletedCount: 1, lastHeartbeat: Math.floor(START / 1000), xrayState: 'running', inboundSyncMode: 'selected', inboundTags: ['inbound-443'], guid: 'n-de-1' },
  { id: 3, name: 'tr-1', remark: 'Istanbul relay', scheme: 'http', address: '10.20.0.7', port: 2053, basePath: '/', enable: true, status: 'offline', latencyMs: 0, cpuPct: 0, memPct: 0, xrayVersion: '', panelVersion: '', uptimeSecs: 0, inboundCount: 0, clientCount: 0, onlineCount: 0, depletedCount: 0, lastHeartbeat: Math.floor((START - 2 * 3600 * 1000) / 1000), lastError: 'dial tcp 10.20.0.7:2053: i/o timeout', xrayState: 'stop', allowPrivateAddress: true, inboundSyncMode: 'all', inboundTags: [], guid: 'n-tr-1' },
];
const MB = 1024 ** 2;

// --- live-ish server status ---------------------------------------------------
let cpu = 23;
let up = 4.2 * MB;
let down = 11.8 * MB;
let sent = 1.21 * 1024 * GB;
let recv = 3.84 * 1024 * GB;
let tcp = 412;
let udp = 96;

function tick() {
  cpu = Math.max(4, Math.min(96, cpu + rnd(-6, 6)));
  up = Math.max(0.2 * MB, up + rnd(-1.2 * MB, 1.2 * MB));
  down = Math.max(0.5 * MB, down + rnd(-2.5 * MB, 2.5 * MB));
  sent += up * 2;
  recv += down * 2;
  tcp = Math.max(50, Math.round(tcp + rnd(-25, 25)));
  udp = Math.max(10, Math.round(udp + rnd(-10, 10)));
}
setInterval(tick, 2000);

function status() {
  const uptime = 86400 * 37 + Math.floor((Date.now() - START) / 1000);
  return {
    cpu: Number(cpu.toFixed(1)),
    cpuCores: 4,
    logicalPro: 8,
    cpuSpeedMhz: 2900,
    disk: { current: 61.3 * GB, total: 160 * GB },
    loads: [0.42, 0.51, 0.47],
    mem: { current: 3.1 * GB, total: 8 * GB },
    netIO: { up: Math.round(up), down: Math.round(down) },
    netTraffic: { sent: Math.round(sent), recv: Math.round(recv) },
    publicIP: { ipv4: '185.92.14.37', ipv6: '2a02:4780:12:ab::1' },
    swap: { current: 0.6 * GB, total: 2 * GB },
    tcpCount: tcp,
    udpCount: udp,
    uptime,
    appUptime: 86400 * 3 + 7200,
    appStats: { threads: 23, mem: 148 * MB, uptime: 86400 * 3 + 7200 },
    xray: { state: 'running', errorMsg: '', version: '25.9.11', color: 'green' },
    openvpn: { state: 'running', errorMsg: '', inboundCount: 1, onlineClients: 14, manualStop: false, color: 'green' },
    l2tp: { state: 'running', errorMsg: '', inboundCount: 1, onlineClients: 6, manualStop: false, color: 'green' },
  };
}

// --- inbounds & clients ---------------------------------------------------------
const uuid = () => globalThis.crypto.randomUUID();
const now = Date.now();
const days = (n) => now + n * 86400000;

function client(email, i, extra = {}) {
  return {
    id: uuid(),
    email,
    enable: i % 7 !== 3,
    totalGB: [0, 50 * GB, 100 * GB, 200 * GB][i % 4],
    expiryTime: [0, days(12), days(31), days(-2)][i % 4],
    limitIp: i % 3,
    subId: randomHex(8),
    tgId: i % 5 === 0 ? 123456789 + i : '',
    comment: i % 4 === 0 ? 'VIP customer' : '',
    group: i % 3 === 0 ? 'vip' : i % 3 === 1 ? 'trial' : '',
    reset: 0,
    flow: '',
    ...extra,
  };
}

const EMAILS = ['ali', 'sara', 'reza', 'maryam', 'amir', 'neda', 'hossein', 'nazanin', 'kian', 'parisa', 'arman', 'yasmin'];

const INBOUNDS = [
  { id: 1, remark: 'VLESS Reality', protocol: 'vless', port: 443, listen: '', tag: 'inbound-443',
    settings: { clients: EMAILS.slice(0, 8).map((e, i) => client(`${e}@reality`, i, { flow: 'xtls-rprx-vision' })), decryption: 'none', fallbacks: [] },
    streamSettings: { network: 'tcp', security: 'reality', realitySettings: { show: false, dest: 'www.microsoft.com:443', serverNames: ['www.microsoft.com'], privateKey: 'x', shortIds: ['ab12'], settings: { publicKey: 'y', fingerprint: 'chrome', spiderX: '/' } }, tcpSettings: { header: { type: 'none' } } },
    sniffing: { enabled: true, destOverride: ['http', 'tls', 'quic'] } },
  { id: 2, remark: 'VMess WS CDN', protocol: 'vmess', port: 8443, listen: '', tag: 'inbound-8443',
    settings: { clients: EMAILS.slice(4, 10).map((e, i) => client(`${e}@vmess`, i)) },
    streamSettings: { network: 'ws', security: 'tls', wsSettings: { path: '/ws', headers: {} }, tlsSettings: { serverName: 'cdn.example.com', certificates: [] } },
    sniffing: { enabled: true, destOverride: ['http', 'tls'] } },
  { id: 3, remark: 'OpenVPN Office', protocol: 'openvpn', port: 1194, listen: '', tag: 'inbound-1194',
    settings: { clients: EMAILS.slice(0, 5).map((e, i) => client(`${e}@ovpn`, i)), poolCIDR: '10.3.0.0/24', routeThroughXray: false },
    streamSettings: { network: 'udp' }, sniffing: { enabled: false } },
  { id: 4, remark: 'Trojan gRPC', protocol: 'trojan', port: 2053, listen: '', tag: 'inbound-2053',
    settings: { clients: EMAILS.slice(6, 12).map((e, i) => client(`${e}@trojan`, i, { password: randomHex(6) })), fallbacks: [] },
    streamSettings: { network: 'grpc', security: 'tls', grpcSettings: { serviceName: 'grpc' }, tlsSettings: { serverName: 'edge.example.com', certificates: [] } },
    sniffing: { enabled: true, destOverride: ['http', 'tls'] } },
  { id: 5, remark: 'Shadowsocks 2022', protocol: 'shadowsocks', port: 8388, listen: '', tag: 'inbound-8388',
    settings: { method: '2022-blake3-aes-128-gcm', password: randomBase64(16), network: 'tcp,udp', clients: EMAILS.slice(2, 7).map((e, i) => client(`${e}@ss`, i, { password: randomBase64(16) })) },
    streamSettings: { network: 'tcp', security: 'none' }, sniffing: { enabled: true, destOverride: ['http', 'tls'] } },
  { id: 15, remark: 'L2TP / IPsec', protocol: 'l2tp', port: 1701, listen: '', tag: 'inbound-1701',
    settings: { clients: EMAILS.slice(8, 12).map((e, i) => client(`${e}@l2tp`, i)), poolCIDR: '10.252.0.0/24', psk: 'omega-psk', routeThroughXray: false },
    streamSettings: { network: 'udp' }, sniffing: { enabled: false } },
  { id: 21, remark: 'AmneziaWG', protocol: 'amneziawg', port: 51820, listen: '', tag: 'inbound-51820',
    settings: { clients: EMAILS.slice(1, 6).map((e, i) => client(`${e}@awg`, i)), mtu: 1420 },
    streamSettings: { network: 'udp' }, sniffing: { enabled: false } },
].map((ib, idx) => {
  const clients = ib.settings.clients || [];
  const upB = Math.round(rnd(20, 400) * GB);
  const downB = Math.round(rnd(80, 1500) * GB);
  return {
    ...ib,
    userId: 1,
    up: upB,
    down: downB,
    total: idx % 3 === 0 ? 0 : 2000 * GB,
    enable: idx !== 4,
    expiryTime: idx % 2 === 0 ? 0 : days(60),
    trafficReset: 'never',
    lastTrafficResetTime: 0,
    settings: JSON.stringify(ib.settings),
    streamSettings: JSON.stringify(ib.streamSettings),
    sniffing: JSON.stringify(ib.sniffing),
    clientStats: clients.map((c, i) => ({
      id: i + 1,
      inboundId: ib.id,
      enable: c.enable,
      email: c.email,
      up: Math.round(rnd(1, 40) * GB),
      down: Math.round(rnd(5, 160) * GB),
      total: c.totalGB,
      expiryTime: c.expiryTime,
      reset: 0,
      lastOnline: i % 2 === 0 ? now - rnd(0, 3600) * 1000 : now - rnd(1, 20) * 86400000,
    })),
    nodeId: null,
    shareAddrStrategy: '',
    shareAddr: '',
    subSortIndex: idx,
  };
});

function allClientStats() {
  return INBOUNDS.flatMap((ib) => ib.clientStats);
}

function clientRecords() {
  const out = [];
  for (const ib of INBOUNDS) {
    const settings = JSON.parse(ib.settings);
    for (const c of settings.clients || []) {
      const stat = ib.clientStats.find((s) => s.email === c.email);
      out.push({
        ...c,
        inboundIds: [ib.id],
        traffic: stat ? { up: stat.up, down: stat.down, total: stat.total, expiryTime: stat.expiryTime, enable: stat.enable, lastOnline: stat.lastOnline } : null,
        createdAt: now - 30 * 86400000,
        updatedAt: now - 86400000,
      });
    }
  }
  return out;
}

const ONLINE = ['ali@reality', 'reza@reality', 'maryam@vmess', 'amir@trojan', 'kian@l2tp', 'sara@awg'];

function clientsPaged(query) {
  const page = Number(query.get('page') || 1);
  const pageSize = Number(query.get('pageSize') || 25);
  const search = (query.get('search') || '').toLowerCase();
  let items = clientRecords();
  if (search) items = items.filter((c) => c.email.toLowerCase().includes(search));
  const total = clientRecords().length;
  const filtered = items.length;
  const slice = items.slice((page - 1) * pageSize, page * pageSize);
  const expiring = items.filter((c) => c.expiryTime > 0 && c.expiryTime - now < 7 * 86400000 && c.expiryTime > now).map((c) => c.email);
  const depleted = items.filter((c) => c.expiryTime > 0 && c.expiryTime < now).map((c) => c.email);
  const deactive = items.filter((c) => !c.enable).map((c) => c.email);
  return {
    items: slice,
    total,
    filtered,
    page,
    pageSize,
    summary: { total: filtered, active: filtered - depleted.length - deactive.length, online: ONLINE, depleted, expiring, deactive },
    groups: ['vip', 'trial'],
  };
}

const XRAY_TEMPLATE = {
  log: { access: 'none', dnsLog: false, error: './error.log', loglevel: 'warning', maskAddress: '' },
  api: { tag: 'api', services: ['HandlerService', 'LoggerService', 'StatsService'] },
  inbounds: [{ tag: 'api', listen: '127.0.0.1', port: 62789, protocol: 'dokodemo-door', settings: { address: '127.0.0.1' } }],
  outbounds: [
    { tag: 'direct', protocol: 'freedom', settings: { domainStrategy: 'AsIs' } },
    { tag: 'blocked', protocol: 'blackhole', settings: {} },
    { tag: 'warp', protocol: 'wireguard', settings: { secretKey: 'x', address: ['172.16.0.2/32'], peers: [{ publicKey: 'y', endpoint: 'engage.cloudflareclient.com:2408' }] } },
  ],
  policy: { levels: { 0: { statsUserDownlink: true, statsUserUplink: true } }, system: { statsInboundDownlink: true, statsInboundUplink: true } },
  routing: {
    domainStrategy: 'AsIs',
    rules: [
      { type: 'field', inboundTag: ['api'], outboundTag: 'api' },
      { type: 'field', outboundTag: 'blocked', ip: ['geoip:private'] },
      { type: 'field', outboundTag: 'blocked', protocol: ['bittorrent'] },
      { type: 'field', outboundTag: 'warp', domain: ['geosite:openai', 'geosite:google'] },
    ],
  },
  stats: {},
};

const SETTINGS = {
  webListen: '', webDomain: '', webPort: 2053, webCertFile: '', webKeyFile: '', webBasePath: '/',
  sessionMaxAge: 60, pageSize: 25, expireDiff: 0, trafficDiff: 0, remarkModel: '-ieo',
  tgBotEnable: false, tgBotToken: '', tgBotProxy: '', tgBotAPIServer: '', tgBotChatId: '', tgRunTime: '@daily',
  tgBotBackup: false, tgBotLoginNotify: true, tgCpu: 80, tgLang: 'en-US', timeLocation: 'Asia/Tehran',
  secretEnable: false, subEnable: true, subJsonEnable: false, subClashEnable: false, subTitle: 'OMEGA', subListen: '',
  subPort: 2096, subPath: '/sub/', subJsonPath: '/json/', subDomain: '', subCertFile: '', subKeyFile: '',
  subUpdates: 12, subEncrypt: true, subShowInfo: true, subURI: '', subJsonURI: '', datepicker: 'gregorian',
  warp: '', externalTrafficInformEnable: false, externalTrafficInformURI: '', ldapEnable: false,
  twoFactorEnable: false, twoFactorToken: '',
};

// --- router ---------------------------------------------------------------------
const LOG_LINES = [
  '2026/10/02 10:12:01 [Info] Xray-core 25.9.11 started',
  '2026/10/02 10:12:01 [Info] OpenVPN daemon: inbound 3 pool 10.3.0.0/24 ready',
  '2026/10/02 10:12:02 [Info] L2TP/IPsec: xl2tpd listening on :1701',
  '2026/10/02 10:14:40 [Warning] client ali@reality exceeded IP limit (2)',
  '2026/10/02 10:20:11 [Info] subscription served for subId 3f9c…',
];

export function route(method, path, query) {
  const p = path.replace(/\/+$/, '') || '/';
  if (p === '/csrf-token') return ok('mock-csrf-token');
  if (p === '/login') return ok(null, 'Login successfully');
  if (p === '/logout') return ok();
  if (p === '/getTwoFactorEnable') return ok(false);
  if (p === '/panel/api/auth/me') return ok({ role: 'admin', username: 'admin', name: 'Administrator' });
  if (p === '/panel/api/server/status') return ok(status());
  if (p === '/panel/api/server/getPanelUpdateInfo') return ok({ currentVersion: '4.0.0-omega', latestVersion: '4.0.0-omega', updateAvailable: false });
  if (p === '/panel/api/server/getXrayVersion') return ok(['25.9.11', '25.8.3', '25.6.8', '25.3.6']);
  if (p === '/panel/api/server/getConfigJson') return ok(XRAY_TEMPLATE);
  if (p.startsWith('/panel/api/server/logs/')) return ok(LOG_LINES);
  if (p.startsWith('/panel/api/server/xraylogs/')) return ok([]);
  if (p.startsWith('/panel/api/server/amneziawglogs/')) return ok([]);
  if (p.startsWith('/panel/api/server/history/')) {
    const pts = Array.from({ length: 60 }, (_, i) => ({ t: Math.floor((now - (59 - i) * 60000) / 1000), v: Number(rnd(10, 70).toFixed(1)) }));
    return ok(pts);
  }
  if (p === '/panel/api/server/xrayMetricsState') return ok({ enabled: false });
  if (p === '/panel/api/setting/all') return ok(SETTINGS);
  if (p === '/panel/api/setting/defaultSettings') {
    return ok({ accessLogEnable: false, expireDiff: 0, trafficDiff: 0, defaultCert: '', defaultKey: '', tgBotEnable: false, subEnable: true, subURI: '', subJsonURI: '', remarkModel: '-ieo', datepicker: 'gregorian', ipLimitEnable: false, pageSize: 25, subTitle: 'OMEGA' });
  }
  if (p === '/panel/api/setting/getDefaultJsonConfig') return ok(XRAY_TEMPLATE);
  if (p === '/panel/api/setting/apiTokens') return ok([]);
  if (p === '/panel/api/inbounds/list/slim' || p === '/panel/api/inbounds/list') return ok(INBOUNDS);
  if (p === '/panel/api/inbounds/options') {
    return ok(INBOUNDS.map((ib) => ({ id: ib.id, remark: ib.remark, tag: ib.tag, protocol: ib.protocol, port: ib.port, listen: ib.listen, nodeId: null })));
  }
  if (p.startsWith('/panel/api/inbounds/get/')) {
    const id = Number(p.split('/').pop());
    return ok(INBOUNDS.find((ib) => ib.id === id) || null);
  }
  if (p === '/panel/api/clients/onlines') return ok(ONLINE);
  if (p === '/panel/api/clients/onlinesByGuid') return ok({});
  if (p === '/panel/api/clients/activeInbounds') return ok({});
  if (p === '/panel/api/clients/lastOnline') {
    return ok(Object.fromEntries(allClientStats().map((s) => [s.email, s.lastOnline])));
  }
  if (p === '/panel/api/clients/groups') {
    return ok([
      { name: 'vip', clientCount: 12, trafficUsed: 418 * GB, up: 61 * GB, down: 357 * GB },
      { name: 'trial', clientCount: 7, trafficUsed: 23 * GB, up: 4 * GB, down: 19 * GB },
      { name: 'reseller-ali', clientCount: 31, trafficUsed: 1260 * GB, up: 180 * GB, down: 1080 * GB },
    ]);
  }
  if (p === '/panel/api/clients/list') return ok(clientRecords());
  if (p === '/panel/api/clients/list/paged') return ok(clientsPaged(query));
  if (p.startsWith('/panel/api/clients/get/')) {
    const email = decodeURIComponent(p.split('/').pop());
    const c = clientRecords().find((x) => x.email === email);
    return ok(c ? { client: c, inboundIds: c.inboundIds } : null);
  }
  if (p.startsWith('/panel/api/clients/ips/')) return ok([]);
  if (p === '/panel/api/resellers/list') return ok(RESELLERS);
  if (p === '/panel/api/resellers/assignments') {
    return ok(RESELLERS.map((r) => ({ resellerId: r.reseller.id, inboundIds: [1, 2], emails: [] })));
  }
  if (p.startsWith('/panel/api/resellers/report/')) {
    const id = Number(p.split('/').pop());
    const stat = RESELLERS.find((r) => r.reseller.id === id) || RESELLERS[0];
    return ok({ stat, clients: [] });
  }
  if (p === '/panel/api/nodes/list') return ok(NODES);
  if (p === '/panel/api/xray') {
    return ok(JSON.stringify({ xraySetting: XRAY_TEMPLATE, inboundTags: INBOUNDS.map((ib) => ib.tag), clientReverseTags: [] }));
  }
  if (p === '/panel/api/xray/getOutboundsTraffic') return ok([{ tag: 'direct', up: 820 * GB, down: 2900 * GB }, { tag: 'warp', up: 40 * GB, down: 210 * GB }]);
  if (p === '/panel/api/xray/outbound-subs') return ok([]);
  if (p === '/panel/api/xray/warp/data') return ok(null);
  if (p === '/panel/api/xray/nord/data') return ok(null);
  if (p === '/panel/api/xray/balancerStatus') return ok([]);
  // Mutations and anything unknown: acknowledge without side effects.
  if (method !== 'GET') return ok(null, 'OK (mock, not persisted)');
  return ok(null);
}

// Minimal WebSocket acceptor so the panel's live bridge connects cleanly.
