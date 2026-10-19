import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import axios from 'axios';

// The component test setup replaces axios with a stub; this suite needs the real
// client so the adapter swap under test is exercised end to end.
vi.unmock('axios');

/**
 * The static design preview runs the real UI against an in-browser mock.
 * Guard the wiring: base-path stripping, the axios adapter, the fetch shim
 * and the silent WebSocket stand-in.
 */
describe('preview mock bootstrap', () => {
  const originalBase = window.X_UI_BASE_PATH;
  const originalBaseURL = axios.defaults.baseURL;
  const originalAdapter = axios.defaults.adapter;
  const originalFetch = window.fetch;
  const originalWebSocket = window.WebSocket;

  beforeAll(async () => {
    window.X_UI_BASE_PATH = '/OMEGA/';
    axios.defaults.baseURL = '/OMEGA/';
    await import('@/preview/mock-bootstrap');
  });

  afterAll(() => {
    window.X_UI_BASE_PATH = originalBase;
    axios.defaults.baseURL = originalBaseURL;
    axios.defaults.adapter = originalAdapter;
    window.fetch = originalFetch;
    window.WebSocket = originalWebSocket;
  });

  it('answers panel API calls through axios with the mock envelope', async () => {
    const res = await axios.get('/panel/api/server/status');
    expect(res.status).toBe(200);
    expect(res.data.success).toBe(true);
    expect(res.data.obj.xray.state).toBe('running');

    const inbounds = await axios.get('/panel/api/inbounds/list');
    expect(Array.isArray(inbounds.data.obj)).toBe(true);
    expect(inbounds.data.obj.length).toBeGreaterThan(0);

    const login = await axios.post('/login', { username: 'a', password: 'b' });
    expect(login.data.success).toBe(true);
  });

  it('honours query parameters when routing', async () => {
    const res = await axios.get('/panel/api/clients/list/paged', { params: { page: 1, pageSize: 5 } });
    expect(res.data.success).toBe(true);
    expect(res.data.obj.items.length).toBeLessThanOrEqual(5);
  });

  it('answers the CSRF bootstrap fetch', async () => {
    const res = await window.fetch('/OMEGA/csrf-token');
    expect(res.ok).toBe(true);
    const json = await res.json();
    expect(json.obj).toBe('mock-csrf-token');
  });

  it('replaces the live-update socket with a silent one', async () => {
    const ws = new window.WebSocket(`ws://${window.location.host}/OMEGA/ws`);
    expect(ws.constructor.name).toBe('PreviewSocket');
    await new Promise<void>((resolve) => ws.addEventListener('open', () => resolve()));
    expect(ws.readyState).toBe(WebSocket.OPEN);
    ws.close();
    expect(ws.readyState).toBe(WebSocket.CLOSED);
  });
});
