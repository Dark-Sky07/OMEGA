/* eslint-disable no-undef */
/**
 * Service worker for the static Aurora design preview on hosts that have no
 * directory-index or SPA fallback (raw file CDNs such as rawcdn.githack.com).
 *
 * It only rewrites top-level navigations inside its own scope:
 *   <scope>              → <scope>index.html        (login page)
 *   <scope>panel[/...]   → <scope>panel/index.html  (panel SPA, deep links)
 * Everything else (assets, mock data) passes straight through to the network.
 * Preview-only: never emitted by the production build.
 */
const SCOPE = self.registration.scope;

function previewDocumentFor(href) {
  if (!href.startsWith(SCOPE)) return null;
  const rel = href.slice(SCOPE.length).split('?')[0].split('#')[0];
  if (rel === '' || rel === 'index.html') return SCOPE + 'index.html';
  if (rel === 'panel' || rel.startsWith('panel/')) return SCOPE + 'panel/index.html';
  return null;
}

self.addEventListener('install', () => {
  self.skipWaiting();
});

self.addEventListener('activate', (event) => {
  event.waitUntil(self.clients.claim());
});

self.addEventListener('fetch', (event) => {
  const request = event.request;
  if (request.method !== 'GET' || request.mode !== 'navigate') return;
  const target = previewDocumentFor(request.url);
  if (!target) return;
  event.respondWith(
    fetch(target, { cache: 'no-cache' }).then((response) => {
      if (!response.ok) return response;
      // Serve the document from the requested URL so the router sees the real path.
      return new Response(response.body, {
        status: 200,
        headers: { 'Content-Type': 'text/html; charset=utf-8' },
      });
    }),
  );
});
