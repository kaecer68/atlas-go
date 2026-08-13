import http from 'node:http';
import fs from 'node:fs';
import path from 'node:path';
import { execSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const DIR = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.resolve(DIR, '../..');
const ADMIN_DIST = path.join(ROOT, 'admin_web', 'dist');
const CLIENT_DIST = path.join(ROOT, 'client_web', 'dist');

const MIME = {
  '.js': 'application/javascript',
  '.css': 'text/css',
  '.html': 'text/html',
  '.png': 'image/png',
  '.svg': 'image/svg+xml',
  '.ico': 'image/x-icon',
  '.json': 'application/json',
  '.woff2': 'font/woff2',
  '.woff': 'font/woff',
  '.ttf': 'font/ttf',
};

function mimeType(ext) {
  return MIME[ext] || 'application/octet-stream';
}

function hasFileExt(p) {
  return /\.\w{2,4}$/.test(p);
}

function isDistStale(projectDir) {
  const distHtml = path.join(projectDir, 'dist', 'index.html');
  const staticHtml = path.join(projectDir, 'static', 'index.html');
  try {
    const distMtime = fs.statSync(distHtml).mtimeMs;
    const staticMtime = fs.statSync(staticHtml).mtimeMs;
    return staticMtime > distMtime;
  } catch (e) {
    if (e.code === 'ENOENT') return true;
    throw e;
  }
}

function ensureDistFresh(projectDir) {
  if (!isDistStale(projectDir)) return;
  const name = path.basename(projectDir);
  try {
    console.log(`[mock-server] ${name}/dist stale or missing — rebuilding…`);
    execSync('node esbuild.config.mjs', { cwd: projectDir, stdio: 'pipe' });
    console.log(`[mock-server] ${name}/dist rebuilt`);
  } catch (err) {
    console.warn(`[mock-server] could not rebuild ${name}/dist: ${err.message}`);
    console.warn('[mock-server] run `npm run build` in admin_web/ + client_web/ manually if tests misbehave');
  }
}

function sendJSON(res, data, status) {
  res.writeHead(status || 200, {
    'Content-Type': 'application/json',
    'Access-Control-Allow-Origin': '*',
  });
  res.end(JSON.stringify(data));
}

function serveStaticFile(res, filePath) {
  try {
    const data = fs.readFileSync(filePath);
    const ext = path.extname(filePath).toLowerCase();
    res.writeHead(200, { 'Content-Type': mimeType(ext) });
    res.end(data);
  } catch (e) {
    res.writeHead(404);
    res.end('Not found');
  }
}

const MOCK_MODELS = {
  models: [
    { name: '鷹派聯準會', weight: 0.143, recent_error: 0.21, hit_rate: 0.67, rationale: 'FOMC dot plot 偏鷹' },
    { name: 'AI 超級週期', weight: 0.0001, recent_error: 0.5, hit_rate: 0.5, rationale: 'NVDA 強勢' },
    { name: '財報驚喜', weight: 0.0003, recent_error: 0.3, hit_rate: 0.65, rationale: '半導體 Q2 財報' },
  ],
};

const MOCK_PREDICTIONS = {
  predictions: [
    { date: '2026-07-15', direction: 'inflow', confidence: 0.85, reasons: ['AI 資本支出擴張'], sectors: ['半導體業', '伺服器'] },
    { date: '2026-07-16', direction: 'outflow', confidence: 0.65, reasons: ['短期獲利了結'], sectors: ['金融股'] },
    { date: '2026-07-17', direction: 'neutral', confidence: 0.40, reasons: ['等待 FOMC 決議'], sectors: [] },
    { date: '2026-07-18', direction: 'inflow', confidence: 0.55, reasons: ['季底作帳行情'], sectors: ['傳產股'] },
    { date: '2026-07-19', direction: 'outflow', confidence: 0.30, reasons: ['週五避險'], sectors: [] },
  ],
  summary: '偏多',
};

const MOCK_CALENDAR = {
  events: [
    { name: 'FOMC 利率決議', expected_flow_impact: 'bearish', start_date: '2026-07-15' },
    { name: 'TSMC 法說會', expected_flow_impact: 'bullish', start_date: '2026-07-17' },
    { name: 'ETF 季度調整', expected_flow_impact: 'bullish', start_date: '2026-07-20' },
    { name: '美國 GDP 初值', expected_flow_impact: 'neutral', start_date: '2026-07-22' },
    { name: '央行理監事會', expected_flow_impact: 'bearish', start_date: '2026-07-24' },
  ],
};

const MOCK_CAPITAL_FLOW_SUMMARY = {
  forces: [
    { force: 'foreign', z_score: 1.2, direction: 'inflow' },
    { force: 'futures', z_score: -0.8, direction: 'outflow' },
    { force: 'tsm_adr', z_score: 1.5, direction: 'inflow' },
    { force: 'institutional', z_score: 0.6, direction: 'inflow' },
    { force: 'dealer', z_score: -0.3, direction: 'neutral' },
    { force: 'government', z_score: 0.2, direction: 'neutral' },
    { force: 'retail', z_score: -1.1, direction: 'outflow' },
  ],
  resonance: { direction: 'mixed', coefficient: 0.32, aligned: ['foreign', 'institutional'], opposing: ['retail'] },
  quality_label: 'inflow',
  quality_score: 0.65,
  summary: '外資與投信同步流入，散戶反向流出。短期偏多格局。',
};

const MOCK_RECOMMENDATIONS = {
  tier: 'premium',
  strategies: { active: '動能策略', entry_signal: '外資連續買超', stop_loss: '跌破月線' },
  market: {
    regime_label: 'RISK_ON',
    capital_flow_summary: '資金轉向流入',
    capital_flow_detail: {
      quality_label: 'inflow',
      dominant_force: 'foreign',
      resonance_dir: 'mixed',
      date: '2026-07-14',
    },
  },
};

const MOCK_USER_PROFILE = {
  email: 'guest@atlas-go.local',
  tier: 'free',
  effective_tier: 'free',
};

const MOCK_PIPELINE = {
  session_id: 'window-20260720-20260809',
  recorded_at: '2026-08-09T22:00:00Z',
  items: [
    {
      symbol: '2330',
      agent_id: 'ai_supercycle_agent',
      skill: 'ai_supercycle',
      layer: 'sector',
      side: 'BUY',
      price: 1000,
      target_price: 1100,
      stop_loss_price: 950,
      conviction: 8,
      passed_guards: true,
      forward_return: 0.012,
      reason: 'AI 資本支出上修，訂單滿載',
      reasoning_chain: ['AI_capex_surge (Global, confidence 0.80)', 'Chain ai_supercycle: 輝達上修 AI 資本支出'],
      supporting_events: ['evt-ai-20260809'],
      factor_scores: { momentum: 0.6, quality: 0.4 },
    },
  ],
  guard_outcomes: [],
};

const MOCK_SESSIONS = {
  sessions: [
    { session_id: 'window-20260720-20260809', regime: 'RISK_ON', recorded_at: '2026-08-09T22:00:00Z' },
  ],
};

function handleAPI(pathname, res) {
  if (pathname === '/api/narrative/models') return sendJSON(res, MOCK_MODELS);
  if (pathname === '/api/events/prediction') return sendJSON(res, MOCK_PREDICTIONS);
  if (pathname === '/api/events/calendar') return sendJSON(res, MOCK_CALENDAR);
  if (pathname === '/api/capital-flow/summary') return sendJSON(res, MOCK_CAPITAL_FLOW_SUMMARY);
  if (pathname === '/api/recommendations') return sendJSON(res, MOCK_RECOMMENDATIONS);
  if (pathname === '/api/dashboard/recommendation-pipeline') return sendJSON(res, MOCK_PIPELINE);
  if (pathname === '/api/dashboard/sessions') return sendJSON(res, MOCK_SESSIONS);

  if (pathname === '/api/user/profile') return sendJSON(res, MOCK_USER_PROFILE);

  if (pathname === '/api/dashboard/system-health') return sendJSON(res, { status: 'ok' });
  if (pathname === '/api/macro/snapshot/latest') return sendJSON(res, {});
  if (pathname === '/api/taiwan/stress-index') return sendJSON(res, { score: 35, regime: 'normal' });
  if (pathname === '/api/narrative/bundle') return sendJSON(res, { events: [], chains: [], models: MOCK_MODELS.models, templates: [] });
  if (pathname === '/api/dashboard/retail-sentiment') return sendJSON(res, { sentiment: 0.2 });
  if (pathname === '/api/dashboard/regime-history') return sendJSON(res, { history: [] });
  if (pathname === '/api/scheduler/status') return sendJSON(res, { tasks: [] });

  res.writeHead(404);
  res.end('Not found');
}

const server = http.createServer((req, res) => {
  const p = new URL(req.url, `http://${req.headers.host}`);
  const pathname = p.pathname;

  if (pathname.startsWith('/api/')) {
    return handleAPI(pathname, res);
  }

  if (pathname.startsWith('/admin/') || pathname === '/admin') {
    const sub = pathname === '/admin' ? '' : pathname.slice('/admin/'.length);
    if (!sub || !hasFileExt(sub)) {
      return serveStaticFile(res, path.join(ADMIN_DIST, 'index.html'));
    }
    return serveStaticFile(res, path.join(ADMIN_DIST, sub));
  }

  if (pathname.startsWith('/client/') || pathname === '/client' || pathname === '/') {
    const sub = (pathname === '/' || pathname === '/client') ? '' : pathname.slice('/client/'.length);
    if (!sub || !hasFileExt(sub)) {
      return serveStaticFile(res, path.join(CLIENT_DIST, 'index.html'));
    }
    return serveStaticFile(res, path.join(CLIENT_DIST, sub));
  }

  if (pathname === '/favicon.ico') {
    res.writeHead(204);
    return res.end();
  }

  res.writeHead(302, { Location: '/client/' });
  res.end();
});

const PORT = parseInt(process.env.MOCK_PORT || '8001', 10);
ensureDistFresh(path.join(ROOT, 'admin_web'));
ensureDistFresh(path.join(ROOT, 'client_web'));
server.listen(PORT, () => {
  console.log(`Mock server listening on http://localhost:${PORT}`);
  console.log(`  Admin: http://localhost:${PORT}/admin/`);
  console.log(`  Client: http://localhost:${PORT}/client/`);
});

export default server;
