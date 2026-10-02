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
import { route } from './mock/core.mjs';

const PORT = Number(process.env.MOCK_PORT || 2053);

function respond(res, body, code = 200) {
  const json = JSON.stringify(body);
  res.writeHead(code, { 'Content-Type': 'application/json; charset=utf-8', 'Content-Length': Buffer.byteLength(json) });
  res.end(json);
}

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
  // eslint-disable-next-line no-console
  if (process.env.MOCK_LOG) console.log(new Date().toISOString(), req.method, path);
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
