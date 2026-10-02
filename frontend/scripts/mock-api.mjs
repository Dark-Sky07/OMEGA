#!/usr/bin/env node
/**
 * Dev-only mock of the OMEGA panel API.
 *
 * Lets the Vite dev server run the real UI against believable data when no Go
 * backend is available (design reviews, screenshots, UI tests). It is never
 * bundled or shipped — `npm run dev:mock` starts it on the same port the Vite
 * proxy already targets (2053, override with MOCK_PORT / VITE_BACKEND_TARGET).
 *
 * Coverage is intentionally broad-but-shallow: every endpoint answers with the
 * panel's `{ success, msg, obj }` envelope; the ones the overview, inbounds,
 * clients and settings pages read get realistic payloads, everything else gets
 * a harmless `obj: null`. Mutations are acknowledged but not persisted.
 */
import http from 'node:http';
import crypto from 'node:crypto';

const PORT = Number(process.env.MOCK_PORT || 2053);
const START = Date.now();

const ok = (obj = null, msg = '') => ({ success: true, msg, obj });
const rnd = (min, max) => min + Math.random() * (max - min);
const GB = 1024 ** 3;
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
const uuid = () => crypto.randomUUID();
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
    subId: crypto.randomBytes(8).toString('hex'),
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
    settings: { clients: EMAILS.slice(6, 12).map((e, i) => client(`${e}@trojan`, i, { password: crypto.randomBytes(6).toString('hex') })), fallbacks: [] },
    streamSettings: { network: 'grpc', security: 'tls', grpcSettings: { serviceName: 'grpc' }, tlsSettings: { serverName: 'edge.example.com', certificates: [] } },
    sniffing: { enabled: true, destOverride: ['http', 'tls'] } },
  { id: 5, remark: 'Shadowsocks 2022', protocol: 'shadowsocks', port: 8388, listen: '', tag: 'inbound-8388',
    settings: { method: '2022-blake3-aes-128-gcm', password: crypto.randomBytes(16).toString('base64'), network: 'tcp,udp', clients: EMAILS.slice(2, 7).map((e, i) => client(`${e}@ss`, i, { password: crypto.randomBytes(16).toString('base64') })) },
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

function respond(res, body, code = 200) {
  const json = JSON.stringify(body);
  res.writeHead(code, { 'Content-Type': 'application/json; charset=utf-8', 'Content-Length': Buffer.byteLength(json) });
  res.end(json);
}

function route(method, path, query) {
  const p = path.replace(/\/+$/, '') || '/';
  if (p === '/csrf-token') return ok('mock-csrf-token');
  if (p === '/login') return ok(null, 'Login successfully');
  if (p === '/logout') return ok();
  if (p === '/getTwoFactorEnable') return ok(false);
  if (p === '/panel/api/auth/me') return ok({ role: 'admin', username: 'admin', name: 'Administrator' });
  if (p === '/panel/api/server/status') return ok(status());
  if (p === '/panel/api/server/getPanelUpdateInfo') return ok({ currentVersion: '3.3.37-omega', latestVersion: '3.3.38-omega', updateAvailable: true });
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
  if (p === '/panel/api/clients/groups') return ok(['vip', 'trial']);
  if (p === '/panel/api/clients/list') return ok(clientRecords());
  if (p === '/panel/api/clients/list/paged') return ok(clientsPaged(query));
  if (p.startsWith('/panel/api/clients/get/')) {
    const email = decodeURIComponent(p.split('/').pop());
    const c = clientRecords().find((x) => x.email === email);
    return ok(c ? { client: c, inboundIds: c.inboundIds } : null);
  }
  if (p.startsWith('/panel/api/clients/ips/')) return ok([]);
  if (p === '/panel/api/resellers/list') return ok([]);
  if (p === '/panel/api/resellers/assignments') return ok([]);
  if (p === '/panel/api/nodes/list') return ok([]);
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
function acceptWebSocket(req, socket) {
  const key = req.headers['sec-websocket-key'];
  if (!key) { socket.destroy(); return; }
  const accept = crypto.createHash('sha1').update(key + '258EAFA5-E914-47DA-95CA-C5AB0DC85B11').digest('base64');
  socket.write(
    'HTTP/1.1 101 Switching Protocols\r\n' +
    'Upgrade: websocket\r\n' +
    'Connection: Upgrade\r\n' +
    `Sec-WebSocket-Accept: ${accept}\r\n\r\n`,
  );
  socket.on('error', () => {});
  socket.on('data', () => {});
}

const server = http.createServer((req, res) => {
  const url = new URL(req.url, `http://${req.headers.host || 'localhost'}`);
  // Vite rewrites `/<basePath>/...` to the backend; the mock serves root only.
  const path = url.pathname;
  let body = '';
  req.on('data', (chunk) => { body += chunk; if (body.length > 1e6) req.destroy(); });
  req.on('end', () => {
    try {
      respond(res, route(req.method, path, url.searchParams));
    } catch (err) {
      respond(res, { success: false, msg: String(err && err.message || err), obj: null }, 500);
    }
  });
});

server.on('upgrade', (req, socket) => {
  if (req.url.replace(/\?.*$/, '').endsWith('/ws')) acceptWebSocket(req, socket);
  else socket.destroy();
});

server.listen(PORT, '127.0.0.1', () => {
  // eslint-disable-next-line no-console
  console.log(`[mock-api] OMEGA mock backend listening on http://127.0.0.1:${PORT}`);
});
