import * as esbuild from "esbuild";
import { copyFileSync, writeFileSync, rmSync } from "node:fs";
import path from "node:path";
import createSharedPlugin from "../shared_web/esbuild-shared-plugin.mjs";

const dist = "dist";

// Clean previous build output
rmSync(dist, { recursive: true, force: true });

const result = await esbuild.build({
  entryPoints: [
    { in: "../shared_web/static/js/bootstrap-utils.js", out: "js/bootstrap-utils" },
    { in: "static/js/main.js", out: "js/main" },
    { in: "static/js/component-init.js", out: "js/component-init" },
    { in: "static/js/event-listeners.js", out: "js/event-listeners" },
    // CSS entry — esbuild resolves @import at build time into single file
    { in: "../shared_web/static/css/main.css", out: "css/main" },
  ],
  bundle: true,
  format: "esm",
  splitting: false,
  outdir: dist,
  metafile: true,
  // Stable entry names for index.html compatibility
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
    createSharedPlugin({
      appStaticDir: path.resolve("static"),
      sharedStaticDir: path.resolve("../shared_web/static"),
    }),
  ],
  // esbuild preserves dynamic import() by default — no special config needed
  logLevel: "info",
});

// Write metadata for cache-busting
writeFileSync("dist/meta.json", JSON.stringify(result.metafile, null, 2));

// Copy index.html into dist/
copyFileSync("static/index.html", "dist/index.html");

console.log("✅ esbuild build complete");
