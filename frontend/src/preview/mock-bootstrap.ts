/**
 * Static design-preview bootstrap (never part of the production bundle).
 *
 * The Aurora preview is published as a plain static site, so there is no Go
 * backend behind it. This module is injected before `main.tsx` only when Vite
 * runs with `--mode preview` and it:
 *
 *  - answers every panel API call from the same in-memory mock the local
 *    `scripts/mock-api.mjs` server uses (wired in as an axios adapter),
 *  - answers the few plain `fetch()` calls (CSRF token, swagger spec),
 *  - replaces the live-update WebSocket with a silent stand-in so the client
 *    does not keep reconnecting against a static host.
 *
 * Nothing here touches application code: the real UI runs unchanged on top.
 */
import axios from 'axios';
import type { AxiosAdapter, AxiosResponse, InternalAxiosRequestConfig } from 'axios';

import { route } from '../../scripts/mock/core.mjs';

type Envelope = { success: boolean; msg: string; obj: unknown };

const basePath = (window.X_UI_BASE_PATH || '/').replace(/\/+$/, '') + '/';

/** Map a request URL (absolute or base-relative) to the mock's root-relative path. */
function toMockPath(input: string): { path: string; query: URLSearchParams } | null {
  const url = new URL(input, window.location.href);
  if (url.origin !== window.location.origin) return null;
  let p = url.pathname;
  if (basePath !== '/' && p.startsWith(basePath)) p = '/' + p.slice(basePath.length);
  else if (basePath !== '/' && p + '/' === basePath) p = '/';
  return { path: p, query: url.searchParams };
}

function answer(method: string, target: string): Envelope | null {
  const mapped = toMockPath(target);
  if (!mapped) return null;
  return route(method.toUpperCase(), mapped.path, mapped.query) as Envelope;
}

const mockAdapter: AxiosAdapter = async (config: InternalAxiosRequestConfig) => {
  const body = answer(config.method || 'get', axios.getUri(config));
  if (!body) throw new Error(`preview mock: unsupported request ${config.url}`);
  // Mimic a real network hop so loading states stay visible in the preview.
  await new Promise((resolve) => setTimeout(resolve, 60 + Math.random() * 120));
  const response: AxiosResponse = {
    data: body,
    status: 200,
    statusText: 'OK',
    headers: { 'content-type': 'application/json; charset=utf-8' },
    config,
    request: {},
  };
  return response;
};

axios.defaults.adapter = mockAdapter;

// Plain fetch() users: the CSRF bootstrap in axios-init and swagger-ui.
const realFetch: typeof fetch = typeof window.fetch === 'function'
  ? window.fetch.bind(window)
  : () => Promise.reject(new Error('fetch is not available'));
window.fetch = async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
  const target = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
  const mapped = toMockPath(target);
  if (mapped && mapped.path === '/panel/api/openapi.json') {
    // The spec is emitted as a static asset by the preview build.
    return realFetch(`${basePath}openapi.json`, init);
  }
  if (mapped && (mapped.path === '/csrf-token' || mapped.path.startsWith('/panel/api/'))) {
    const method = (init?.method || (typeof input !== 'string' && !(input instanceof URL) ? input.method : 'GET')) || 'GET';
    const body = answer(method, target);
    if (body) {
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { 'Content-Type': 'application/json; charset=utf-8' },
      });
    }
  }
  return realFetch(input, init);
};

// Silent WebSocket: opens, never speaks, never reconnect-spams the static host.
class PreviewSocket extends EventTarget {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;

  readonly url: string;
  readyState = PreviewSocket.CONNECTING;
  onopen: ((ev: Event) => void) | null = null;
  onmessage: ((ev: MessageEvent) => void) | null = null;
  onerror: ((ev: Event) => void) | null = null;
  onclose: ((ev: Event) => void) | null = null;

  constructor(url: string | URL) {
    super();
    this.url = String(url);
    setTimeout(() => {
      this.readyState = PreviewSocket.OPEN;
      const ev = new Event('open');
      this.onopen?.(ev);
      this.dispatchEvent(ev);
    }, 30);
  }

  send(): void {}

  close(): void {
    if (this.readyState === PreviewSocket.CLOSED) return;
    this.readyState = PreviewSocket.CLOSED;
    const ev = new Event('close');
    this.onclose?.(ev);
    this.dispatchEvent(ev);
  }
}

const RealWebSocket = window.WebSocket;
function previewWebSocket(url: string | URL, protocols?: string | string[]) {
  const mapped = toMockPath(String(url).replace(/^ws/, 'http'));
  if (mapped && mapped.path === '/ws') return new PreviewSocket(url);
  if (!RealWebSocket) throw new Error('WebSocket is not available');
  return new RealWebSocket(url, protocols);
}
previewWebSocket.CONNECTING = 0;
previewWebSocket.OPEN = 1;
previewWebSocket.CLOSING = 2;
previewWebSocket.CLOSED = 3;
window.WebSocket = previewWebSocket as unknown as typeof WebSocket;

// Raw-file CDNs have no directory index or SPA fallback. When the build asks
// for it, a tiny service worker maps <base>/ and <base>/panel/* onto the two
// HTML documents, and file-style entry URLs are normalised so the router sees
// the real route (the SW is registered before the first navigation away).
if (import.meta.env.VITE_PREVIEW_SW === '1') {
  if ('serviceWorker' in navigator) {
    navigator.serviceWorker.register(`${basePath}sw.js`, { scope: basePath }).catch((err) => {
      console.warn('[omega-preview] service worker registration failed', err);
    });
  }
  const here = window.location.pathname;
  if (here === `${basePath}panel/index.html`) {
    window.history.replaceState(window.history.state, '', `${basePath}panel/${window.location.search}${window.location.hash}`);
  }
}

// Make the preview obvious in dev tools without touching the UI itself.
console.info('[omega-preview] static design preview — all data is simulated, nothing is saved.');
