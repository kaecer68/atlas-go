import * as esbuild from "esbuild";
import { copyFileSync, writeFileSync, rmSync } from "node:fs";

const dist = "dist";
const mode = process.argv[2] || "build"; // "build" or "watch"

/**
 * esbuild config for the legacy /web/ single-app entry.
 *
 * Modes:
 *   node esbuild.config.mjs           one-shot build (default, used by `npm run build`)
 *   node esbuild.config.mjs watch     incremental watch + rebuild on file change
 *
 * Watch mode uses esbuild.context() so subsequent rebuilds reuse the same
 * module graph — much faster than a full rebuild per change. The onEnd
 * plugin re-copies static/index.html into dist/ on every successful rebuild
 * so a pure-JS change doesn't need a manual index.html copy.
 */

const opts = {
  entryPoints: [
    { in: "static/js/bootstrap-utils.js", out: "js/bootstrap-utils" },
    { in: "static/js/main.js", out: "js/main" },
    { in: "static/js/component-init.js", out: "js/component-init" },
    { in: "static/js/event-listeners.js", out: "js/event-listeners" },
    // CSS entry — esbuild resolves @import at build time into single file
    { in: "static/css/main.css", out: "css/main" },
  ],
  bundle: true,
  format: "esm",
  splitting: true,
  outdir: dist,
  metafile: true,
  chunkNames: "chunks/[name]-[hash]",
  // Stable entry names for index.html compatibility; only chunks get hashed
  entryNames: "[dir]/[name]",
  assetNames: "assets/[name]-[hash]",
  minify: true,
  sourcemap: false,
  target: ["es2020"],
  loader: {
    ".js": "jsx", // allow JSX in .js files if any
    ".ts": "ts",   // strip TypeScript from field_types.ts
  },
  plugins: [
    {
      name: "copy-index-html",
      setup(build) {
        build.onEnd((result) => {
          if (result.errors.length > 0) return;
          copyFileSync("static/index.html", "dist/index.html");
        });
      },
    },
  ],
  // esbuild preserves dynamic import() by default — no special config needed
  logLevel: "info",
};

async function run() {
  if (mode === "watch") {
    const ctx = await esbuild.context(opts);
    // Initial build (errors thrown synchronously)
    const initial = await ctx.rebuild();
    if (initial.errors.length > 0) {
      console.error("❌ Initial build failed");
      process.exit(1);
    }
    writeFileSync("dist/meta.json", JSON.stringify(initial.metafile, null, 2));
    await ctx.watch();
    console.log("👀 Watching for changes... (Ctrl+C to stop)");
  } else {
    rmSync(dist, { recursive: true, force: true });
    const result = await esbuild.build(opts);
    if (result.errors.length > 0) {
      console.error("❌ Build failed");
      process.exit(1);
    }
    writeFileSync("dist/meta.json", JSON.stringify(result.metafile, null, 2));
    copyFileSync("static/index.html", "dist/index.html");
    console.log("✅ esbuild build complete");
  }
}

run().catch((err) => {
  console.error(err);
  process.exit(1);
});