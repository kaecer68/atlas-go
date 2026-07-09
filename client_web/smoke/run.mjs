#!/usr/bin/env node
// client_web/smoke/run.mjs — Atlas investor-facing frontend smoke runner.
//
// 用途：在 CI 內由 scripts/ci/frontend_smoke.sh 啟動的 Playwright smoke test。
// 設計：根據 SMOKE_PAGES env（逗號分隔的 page id 清單）逐一切換 SPA page，
//       等待 3 秒給 fetch/JS 完成，掃描 #page-X 內文有無 NaN/undefined/null。
//       Console error 會先過 allowlist（client_web/smoke/known-issues.json），
//       known → warn only, unknown → gate fail.
//
// 退出碼：0 = 全部通過；1 = 任一 page 抓出 bad pattern 或 fetch timeout，或 unknown console error。
//
// 環境變數：
//   ATLAS_PORT     — atlas server port（預設 18080）
//   SMOKE_PAGES    — 要 smoke 的 page 清單（逗號分隔，預設 crossmarket,narrative,live,portfolio,strategies）
//   SMOKE_TIMEOUT  — 每個 page 切換後等待 fetch 完成的秒數（預設 5）

import { chromium } from "playwright";
import { readFileSync, existsSync } from "fs";
import { dirname, join } from "path";
import { fileURLToPath } from "url";

const __dirname = dirname(fileURLToPath(import.meta.url));

// 載入 known-issue allowlist
function loadKnownIssues() {
  const p = join(__dirname, "known-issues.json");
  if (!existsSync(p)) {
    console.warn("⚠ known-issues.json not found — all console errors will fail the gate");
    return [];
  }
  try {
    const raw = JSON.parse(readFileSync(p, "utf-8"));
    const list = raw.patterns || raw;
    return list.map((e) => ({ re: new RegExp(e.pattern, "i"), reason: e.reason }));
  } catch (e) {
    console.warn(`⚠ failed to parse known-issues.json: ${e.message}`);
    return [];
  }
}

function classifyError(msg, knownIssues) {
  for (const { re, reason } of knownIssues) {
    if (re.test(msg)) return { known: true, reason };
  }
  return { known: false, reason: null };
}

const PORT = process.env.ATLAS_PORT || "18080";
const BASE = `http://localhost:${PORT}/client`;
const FETCH_WAIT = parseInt(process.env.SMOKE_TIMEOUT || "5", 10) * 1000;
const PAGES_ARG = (process.env.SMOKE_PAGES || "home,crossmarket,industry,narrative,pipeline,portfolio,strategies,login,register,premium,stock-quote")
  .split(",")
  .map((s) => s.trim())
  .filter(Boolean);

const PAGE_SELECTORS = {
  home: "#page-home",
  crossmarket: "#page-crossmarket",
  industry: "#page-industry",
  narrative: "#page-narrative",
  pipeline: "#page-pipeline",
  portfolio: "#page-portfolio",
  strategies: "#page-strategies",
  login: "#page-login",
  register: "#page-register",
  premium: "#page-premium",
  "stock-quote": "#page-stock-quote",
};

// 真正會造成 bug 的字串 pattern：浮點數 / 型別錯誤
const BAD_PATTERNS = [
  { re: /\bNaN\b/, label: "NaN" },
  { re: /\bundefined\b/i, label: "undefined" },
  { re: /\bnull\b/i, label: "null" },
];

const failures = [];
const successes = [];

async function run() {
  const browser = await chromium.launch({ headless: true });
  const ctx = await browser.newContext();
  const page = await ctx.newPage();

  // 收集 console error（前端 JS 拋錯也是 bug 訊號）
  const consoleErrors = [];
  page.on("pageerror", (err) => {
    consoleErrors.push(`pageerror: ${err.message}`);
  });
  page.on("console", (msg) => {
    if (msg.type() === "error") {
      // Browser-generated resource load failures are already captured by the
      // response handler above; do not double-count them as frontend JS errors.
      const text = msg.text();
      if (text && text.startsWith("Failed to load resource")) {
        return;
      }
      consoleErrors.push(`console.error: ${text}`);
    }
  });
  // 捕捉 404/500 回應的 URL（Chrome console 不會在 text() 中顯示 URL）
  const notFoundUrls = new Set();
  const serverErrorUrls = new Set();
  page.on("response", (resp) => {
    const url = resp.url();
    if (resp.status() === 404) {
      if (!notFoundUrls.has(url)) {
        notFoundUrls.add(url);
        console.warn(`[404] ${url} (from ${resp.request().resourceType()})`);
      }
    } else if (resp.status() >= 500) {
      if (!serverErrorUrls.has(url)) {
        serverErrorUrls.add(url);
        console.error(`[${resp.status()}] ${url} (from ${resp.request().resourceType()})`);
      }
    }
  });

  try {
    await page.goto(BASE, { waitUntil: "domcontentloaded", timeout: 30000 });
    // 等 JS bootstrap 完成
    await page.waitForFunction(
      () => typeof window.switchPage === "function",
      { timeout: 20000 },
    );
    console.log(`✓ Loaded ${BASE} (switchPage ready)`);

    for (const pageId of PAGES_ARG) {
      const selector = PAGE_SELECTORS[pageId];
      if (!selector) {
        console.error(`✗ Unknown page id: ${pageId}`);
        failures.push({ pageId, reason: "unknown-page-id" });
        continue;
      }

      try {
        // 切換 SPA page（silent mode，不改 URL）
        await page.evaluate((id) => window.switchPage(id, true), pageId);
        // 等該 page 變 active
        await page.waitForSelector(`${selector}.active`, { timeout: 8000 });
        // 等 fetch + render 跑一輪
        await page.waitForTimeout(FETCH_WAIT);

        // 抓 page 全部 innerText（包含 fetch 注入的資料）
        const text = await page.locator(selector).innerText();

        // 掃 bad pattern
        const hits = [];
        for (const { re, label } of BAD_PATTERNS) {
          const m = text.match(re);
          if (m) hits.push(`${label} at "${text.slice(Math.max(0, m.index - 20), m.index + 30)}"`);
        }

        // E2E data-flow check: on home page, verify home sections are rendered
        // (not just an empty shell). This catches API → frontend → DOM pipeline breaks.
        let dataFlowIssue = null;
        if (pageId === 'home') {
          const sectionCount = await page.locator('#page-home .home-section, #page-home #home-tier-sections').count();
          if (sectionCount === 0) {
            dataFlowIssue = 'home page rendered no home-section or tier sections (empty DOM)';
          }
        }

        if (hits.length > 0) {
          console.error(`✗ ${pageId}: bad pattern found`);
          for (const h of hits) console.error(`    ${h}`);
          failures.push({ pageId, reason: "bad-pattern", hits });
        } else if (dataFlowIssue) {
          console.error(`✗ ${pageId}: ${dataFlowIssue}`);
          failures.push({ pageId, reason: "data-flow", error: dataFlowIssue });
        } else {
          const len = text.length;
          console.log(`✓ ${pageId}: ${len} chars, no bad patterns`);
          successes.push({ pageId, chars: len });
        }
      } catch (e) {
        console.error(`✗ ${pageId}: ${e.message.split("\n")[0]}`);
        failures.push({ pageId, reason: "exception", error: e.message });
      }
    }

    if (consoleErrors.length > 0) {
      const knownIssues = loadKnownIssues();
      let knownCount = 0;
      let unknownCount = 0;
      console.error(`\n⚠ Captured ${consoleErrors.length} console error(s) (${knownIssues.length} known pattern(s) loaded):`);
      for (const err of consoleErrors.slice(0, 20)) {
        const { known, reason } = classifyError(err, knownIssues);
        if (known) {
          knownCount++;
          console.error(`    ⚠ Console error (known: ${reason}): ${err}`);
        } else {
          unknownCount++;
          console.error(`    ✗ Console error (UNKNOWN — gate fail): ${err}`);
        }
      }
      if (unknownCount > 0) {
        failures.push({ pageId: "console", reason: `unknown-console-errors`, hits: [unknownCount] });
      }
      if (knownCount > 0) {
        console.error(`    → ${knownCount} known, ${unknownCount} unknown`);
      }
    }
  } finally {
    await browser.close();
  }

  // Summary
  console.log("\n=== Smoke Summary ===");
  console.log(`Passed: ${successes.length} | Failed: ${failures.length}`);
  if (failures.length > 0) {
    console.log("\nFailures:");
    for (const f of failures) {
      console.log(`  - ${f.pageId}: ${f.reason}`);
    }
    process.exit(1);
  }
  process.exit(0);
}

run().catch((err) => {
  console.error(`Fatal: ${err.message}`);
  process.exit(2);
});
