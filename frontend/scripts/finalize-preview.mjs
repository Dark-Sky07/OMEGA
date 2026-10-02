#!/usr/bin/env node
/**
 * Arrange `vite build --mode preview` output for a static host (GitHub Pages).
 *
 * The real panel is served by the Go backend, which maps `/` to the login page
 * and `/panel/*` to the SPA. A static host only knows files, so this script
 * lays the build out the same way:
 *
 *   index.html            ← login page            (served at <base>/)
 *   panel/index.html      ← panel SPA             (served at <base>/panel/)
 *   404.html              ← panel SPA             (deep links: <base>/panel/inbounds …)
 *   panel/api/openapi.json← spec for the API docs page
 *   .nojekyll             ← keep Pages from mangling asset folders
 *   sw.js                 ← (VITE_PREVIEW_SW=1) navigation fallback for raw-file CDNs
 *
 * All asset URLs are absolute under the configured base, so moving the HTML
 * files around is safe.
 */
import { copyFileSync, existsSync, mkdirSync, renameSync, rmSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const dist = join(__dirname, '..', 'preview-dist');

if (!existsSync(join(dist, 'index.html')) || !existsSync(join(dist, 'login.html'))) {
  console.error('[preview] run `vite build --mode preview` first — preview-dist is incomplete');
  process.exit(1);
}

mkdirSync(join(dist, 'panel', 'api'), { recursive: true });
renameSync(join(dist, 'index.html'), join(dist, 'panel', 'index.html'));
copyFileSync(join(dist, 'panel', 'index.html'), join(dist, '404.html'));
renameSync(join(dist, 'login.html'), join(dist, 'index.html'));
// The subscription page needs server-rendered data; it is not part of the preview.
rmSync(join(dist, 'subpage.html'), { force: true });

if (existsSync(join(dist, 'openapi.json'))) {
  copyFileSync(join(dist, 'openapi.json'), join(dist, 'panel', 'api', 'openapi.json'));
}
writeFileSync(join(dist, '.nojekyll'), '');

// Raw-file CDN variant (VITE_PREVIEW_SW=1): ship the navigation service worker
// that stands in for the missing directory index / SPA fallback.
if (process.env.VITE_PREVIEW_SW === '1') {
  copyFileSync(join(__dirname, 'preview-sw.js'), join(dist, 'sw.js'));
}

console.log(`[preview] static preview ready in ${dist}`);
