#!/usr/bin/env node
// admin_web/smoke/run.mjs — Atlas admin/operator frontend smoke runner.

import { chromium } from "playwright";
import { readFileSync, existsSync } from "fs";
import { dirname, join } from "path";
import { fileURLToPath } from "url";

const __dirname = dirname(fileURLToPath(import.meta.url));

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
const BASE = `http://localhost:${PORT}/admin`;
const FETCH_WAIT = parseInt(process.env.SMOKE_TIMEOUT || "5", 10) * 1000;
const PAGES_ARG = (process.env.SMOKE_PAGES || "home,live,reports,experiments,pipeline,portfolio,performance-report,parameters,alerts,datachannels,metrics,config")
  .split(",")
  .map((s) => s.trim())
  .filter(Boolean);

const PAGE_SELECTORS = {
  home: "#page-home",
  live: "#page-live",
  reports: "#page-reports",
  experiments: "#page-experiments",
  pipeline: "#page-pipeline",
  portfolio: "#page-portfolio",
  "performance-report": "#page-performance-report",
  parameters: "#page-parameters",
  alerts: "#page-alerts",
  datachannels: "#page-datachannels",
  metrics: "#page-metrics",
  config: "#page-config",
};

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

  const consoleErrors = [];
  page.on("pageerror", (err) => { consoleErrors.push(`pageerror: ${err.message}`); });
  page.on("console", (msg) => {
    if (msg.type() === "error") {
      const text = msg.text();
      if (text && text.startsWith("Failed to load resource")) return;
      consoleErrors.push(`console.error: ${text}`);
    }
  });

  const notFoundUrls = new Set();
  const serverErrorUrls = new Set();
  page.on("response", (resp) => {
    const url = resp.url();
    const status = resp.status();
    if (!url.includes("/api/")) return;
    if (status === 404) {
      if (!notFoundUrls.has(url)) { notFoundUrls.add(url); console.warn(`[404] ${url}`); }
    } else if (status >= 400) {
      if (!serverErrorUrls.has(url)) { serverErrorUrls.add(url); console.error(`[${status}] ${url}`); }
    }
  });

  try {
    await page.goto(BASE, { waitUntil: "domcontentloaded", timeout: 30000 });
    await page.waitForFunction(() => typeof window.switchPage === "function", { timeout: 20000 });
    console.log(`✓ Loaded ${BASE} (switchPage ready)`);

    for (const pageId of PAGES_ARG) {
      const selector = PAGE_SELECTORS[pageId];
      if (!selector) {
        console.error(`✗ Unknown page id: ${pageId}`);
        failures.push({ pageId, reason: "unknown-page-id" });
        continue;
      }
      try {
        await page.evaluate((id) => window.switchPage(id, true), pageId);
        await page.waitForSelector(`${selector}.active`, { timeout: 8000 });
        await page.waitForTimeout(FETCH_WAIT);
        const text = await page.locator(selector).innerText();
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
          console.log(`✓ ${pageId}: ${text.length} chars, no bad patterns`);
          successes.push({ pageId, chars: text.length });
        }
      } catch (e) {
        console.error(`✗ ${pageId}: ${e.message.split("\n")[0]}`);
        failures.push({ pageId, reason: "exception", error: e.message });
      }
    }

    const knownIssues = loadKnownIssues();
    // 比對時同時傳入 URL 和狀態碼，allowlist pattern 只匹配 URL path
    const httpFailureUrls = [...notFoundUrls, ...serverErrorUrls].filter((url) => {
      const { known } = classifyError(url, knownIssues);
      return !known;
    });
    if (httpFailureUrls.length > 0) {
      console.error(`\n✗ ${httpFailureUrls.length} unallowed API error(s) detected:`);
      for (const url of httpFailureUrls) console.error(`    ✗ ${url}`);
      failures.push({ pageId: "http-api-errors", reason: "unallowed-api-errors", hits: httpFailureUrls });
    }

    if (consoleErrors.length > 0) {
      let knownCount = 0, unknownCount = 0;
      console.error(`\n⚠ Captured ${consoleErrors.length} console error(s):`);
      for (const err of consoleErrors.slice(0, 20)) {
        const { known, reason } = classifyError(err, knownIssues);
        if (known) { knownCount++; console.error(`    ⚠ (known: ${reason}): ${err}`); }
        else { unknownCount++; console.error(`    ✗ UNKNOWN: ${err}`); }
      }
      if (unknownCount > 0) failures.push({ pageId: "console", reason: "unknown-console-errors", hits: [unknownCount] });
      if (knownCount > 0) console.error(`    → ${knownCount} known, ${unknownCount} unknown`);
    }
  } finally {
    await browser.close();
  }

  console.log("\n=== Smoke Summary ===");
  console.log(`Passed: ${successes.length} | Failed: ${failures.length}`);
  if (failures.length > 0) {
    for (const f of failures) console.log(`  - ${f.pageId}: ${f.reason}`);
    process.exit(1);
  }
  process.exit(0);
}

run().catch((err) => { console.error(`Fatal: ${err.message}`); process.exit(2); });
