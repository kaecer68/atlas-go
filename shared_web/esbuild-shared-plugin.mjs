import path from "node:path";
import fs from "node:fs";

/**
 * Resolve missing relative imports by falling back between app and shared trees.
 *
 * Files unique to each app stay in `admin_web/static`/`client_web/static`, while
 * shared CSS/JS lives in `shared_web/static` with the same relative layout. This
 * plugin lets both sides find each other without rewriting imports:
 *
 * - Imports from the app tree that miss in the app tree are looked up in the
 *   shared tree.
 * - Imports from the shared tree that miss in the shared tree are looked up in
 *   the app tree (e.g. shared components importing app-specific `main.js`).
 */
export default function createSharedPlugin({ appStaticDir, sharedStaticDir }) {
  const appStaticDirAbs = path.resolve(appStaticDir);
  const sharedStaticDirAbs = path.resolve(sharedStaticDir);

  function isInside(dir, file) {
    return file === dir || file.startsWith(dir + path.sep);
  }

  function resolveToRoot(rootAbs, resolveDir, importPath) {
    const resolved = path.resolve(resolveDir, importPath);
    const rel = path.relative(rootAbs, resolved);
    if (rel.startsWith("..") || path.isAbsolute(rel)) {
      return null;
    }
    return { resolved, rel };
  }

  return {
    name: "shared-static-fallback",
    setup(build) {
      build.onResolve(
        { filter: /^\.{1,2}\//, namespace: "file" },
        (args) => {
          const importerAbs = path.resolve(args.importer);

          // Imports from the app static tree: app first, then shared fallback.
          if (isInside(appStaticDirAbs, importerAbs)) {
            const app = resolveToRoot(
              appStaticDirAbs,
              args.resolveDir,
              args.path
            );
            if (!app) return undefined;
            if (fs.existsSync(app.resolved)) return undefined;

            const sharedResolved = path.resolve(sharedStaticDirAbs, app.rel);
            if (fs.existsSync(sharedResolved)) {
              return { path: sharedResolved, namespace: "file" };
            }
            return undefined;
          }

          // Imports from the shared static tree: shared first, then app fallback.
          if (isInside(sharedStaticDirAbs, importerAbs)) {
            const shared = resolveToRoot(
              sharedStaticDirAbs,
              args.resolveDir,
              args.path
            );
            if (!shared) return undefined;
            if (fs.existsSync(shared.resolved)) return undefined;

            const appResolved = path.resolve(appStaticDirAbs, shared.rel);
            if (fs.existsSync(appResolved)) {
              return { path: appResolved, namespace: "file" };
            }
            return undefined;
          }

          // Any other importer (config file, CSS entry, etc.) uses default resolution.
          return undefined;
        }
      );
    },
  };
}
