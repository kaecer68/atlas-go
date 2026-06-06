#!/usr/bin/env node
// web/smoke/run.mjs — Atlas frontend smoke runner.
//
// 用途：在 CI 內由 scripts/ci/frontend_smoke.sh 啟動的 Playwright smoke test。
// 設計：根據 SMOKE_PAGES env（逗號分隔的 page id 清單）逐一切換 SPA page，
//       等待 3 秒給 fetch/JS 完成，掃描 #page-X 內文有無 NaN/undefined/null。
//       Console error 透過 known-issues.json allowlist 分類：已知→warn，未知→fail。
//
// 退出碼：0 = 全部通過；1 = bad pattern / unknown console error / exception。
//
// 環境變數：
//   ATLAS_PORT     — atlas server port（預設 18080）
//   SMOKE_PAGES    — 要 smoke 的 page 清單（逗號分隔，預設 overview,narrative,live,portfolio）
//   SMOKE_TIMEOUT  — 每個 page 切換後等待 fetch 完成的秒數（預設 5）

import { chromium } from "playwright";
import { readFileSync } from "node:fs";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));

const PORT = process.env.ATLAS_PORT || "18080";
const BASE = `http://localhost:${PORT}`;
const FETCH_WAIT = parseInt(process.env.SMOKE_TIMEOUT || "5", 10) * 1000;
const PAGES_ARG = (process.env.SMOKE_PAGES || "overview,narrative,live,portfolio")
  .split(",")
  .map((s) => s.trim())
  .filter(Boolean);

// 允許的 page id 與其關鍵 selector（用來確認 page 已切換 active）
const PAGE_SELECTORS = {
  overview: "#page-overview",
  narrative: "#page-narrative",
  live: "#page-live",
  portfolio: "#page-portfolio",
  industry: "#page-industry",
  decision: "#page-decision",
  pipeline: "#page-pipeline",
};

// 真正會造成 bug 的字串 pattern：浮點數 / 型別錯誤
const BAD_PATTERNS = [
  { re: /\bNaN\b/, label: "NaN" },
  { re: /\bundefined\b/i, label: "undefined" },
  { re: /\bnull\b/i, label: "null" },
];

// 載入 known-issues allowlist（區分已知 vs 新的 console error）
let knownIssues = [];
try {
  const json = readFileSync(resolve(__dirname, "known-issues.json"), "utf-8");
  knownIssues = JSON.parse(json);
  console.log(`✓ Loaded ${knownIssues.length} known-issue patterns`);
} catch (e) {
  console.warn(`⚠ Could not load known-issues.json: ${e.message}. All console errors will fail.`);
}

function classifyError(msg) {
  for (const issue of knownIssues) {
    if (msg.includes(issue.pattern)) {
      return { known: true, note: issue.note };
    }
  }
  return { known: false };
}
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
      consoleErrors.push(`console.error: ${msg.text()}`);
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

        if (hits.length > 0) {
          console.error(`✗ ${pageId}: bad pattern found`);
          for (const h of hits) console.error(`    ${h}`);
          failures.push({ pageId, reason: "bad-pattern", hits });
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
      const known = [];
      const unknown = [];
      for (const err of consoleErrors) {
        const result = classifyError(err);
        if (result.known) {
          known.push({ msg: err, note: result.note });
        } else {
          unknown.push(err);
        }
      }

      if (known.length > 0) {
        console.warn(`\n⚠ ${known.length} known console issue(s) (allowlisted):`);
        for (const k of known) {
          console.warn(`    [KNOWN] ${k.msg}`);
          console.warn(`             → ${k.note}`);
        }
      }

      if (unknown.length > 0) {
        console.error(`\n✗ ${unknown.length} UNKNOWN console error(s) — gate FAIL:`);
        for (const e of unknown.slice(0, 10)) {
          console.error(`    ${e}`);
        }
        for (const e of unknown) {
          failures.push({ pageId: "*", reason: "unknown-console-error", error: e });
        }
      } else if (known.length > 0) {
        console.warn("    (all console errors are allowlisted — gate continues)");
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
