/**
 * Minimal SPA-aware static + auth-mock server for client_web Playwright tests.
 *
 * Replaces python3 -m http.server for the CI frontend-tests job (no Go, no
 * PostgreSQL). Behavior:
 *   - Serves files from `dist/` at both `/` and `/client/` paths (strips the
 *     `/client/` prefix that the Go server normally handles).
 *   - Falls back to `dist/index.html` for any path without an extension
 *     (SPA routing: /client/evolution_panel → dist/index.html).
 *   - Replies 404 for paths WITH an extension whose file is missing.
 *   - For `/api/*` paths used by the SPA's init chain, returns a minimal
 *     200 JSON so the SPA can boot. Tests that need richer mocks use
 *     page.route() on top of this baseline.
 *
 * This is NOT for production — bound to 127.0.0.1, no TLS, no caching,
 * no auth.
 */
import http from 'node:http';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const DIST = path.resolve(__dirname, '../dist');
const PORT = Number(process.argv[2] || 8085);
const HOST = '127.0.0.1';

const MIME = {
  '.html': 'text/html; charset=utf-8',
  '.js':   'application/javascript; charset=utf-8',
  '.mjs':  'application/javascript; charset=utf-8',
  '.css':  'text/css; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.svg':  'image/svg+xml',
  '.png':  'image/png',
  '.jpg':  'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.ico':  'image/x-icon',
  '.woff2':'font/woff2',
  '.map':  'application/json; charset=utf-8',
};

// Auth + init APIs the SPA hits unconditionally on every page load. Without
// these, initAuth() throws and loadAll()/loadModules() never run, leaving
// every page empty. Tests can layer page.route() on top to override.
const API_FALLBACKS = {
  // GUEST_MODE=false：回傳有效 email+tier 讓 initAuth/isLoggedIn 判定已登入，
  // 否則登入 gate 會把功能頁導向 /login，e2e 無法渲染功能頁。
  '/api/user/profile':   { email: 'test@atlas.test', tier: 'registered', effective_tier: 'registered' },
  '/api/auth/whoami':    { email: 'test@atlas.test', tier: 'registered' },
  '/api/auth/refresh':   { ok: true },
  '/api/system/status':  { status: 'ok' },
};

const server = http.createServer((req, res) => {
  let urlPath = decodeURIComponent((req.url || '/').split('?')[0]);
  if (urlPath.includes('..')) {
    res.writeHead(400);
    return res.end('bad path');
  }

  // Strip /client/ prefix — the Go server mounts client_web at /client/,
  // so the HTML references assets as `css/main.css` which the browser
  // resolves to `/client/css/main.css`. Our static server serves from
  // dist/ root, so we need to remove the prefix.
  if (urlPath.startsWith('/client/')) {
    urlPath = urlPath.slice('/client'.length);
  }

  // API fallback
  if (urlPath in API_FALLBACKS) {
    res.writeHead(200, { 'content-type': 'application/json', 'cache-control': 'no-store' });
    return res.end(JSON.stringify(API_FALLBACKS[urlPath]));
  }

  // For /api/* paths not in fallback: return empty JSON instead of SPA
  // fallback, so the SPA's silentGetJSON doesn't get HTML.
  if (urlPath.startsWith('/api/')) {
    res.writeHead(200, { 'content-type': 'application/json', 'cache-control': 'no-store' });
    return res.end('{}');
  }

  // Static file serving
  const candidate = path.join(DIST, urlPath);
  const ext = path.extname(urlPath);

  if (urlPath === '/' || urlPath === '') {
    return serveFile(path.join(DIST, 'index.html'), res);
  }
  if (ext && fs.existsSync(candidate) && fs.statSync(candidate).isFile()) {
    return serveFile(candidate, res);
  }
  if (!ext) {
    // SPA fallback: any path with no extension → index.html
    return serveFile(path.join(DIST, 'index.html'), res);
  }
  res.writeHead(404, { 'content-type': 'text/plain' });
  res.end('not found');
});

function serveFile(p, res) {
  fs.readFile(p, (err, buf) => {
    if (err) {
      res.writeHead(500, { 'content-type': 'text/plain' });
      return res.end('read error: ' + err.message);
    }
    const type = MIME[path.extname(p)] || 'application/octet-stream';
    res.writeHead(200, { 'content-type': type, 'cache-control': 'no-store' });
    res.end(buf);
  });
}

server.listen(PORT, HOST, () => {
  console.log(`[spa-server] serving ${DIST} on http://${HOST}:${PORT}`);
});
