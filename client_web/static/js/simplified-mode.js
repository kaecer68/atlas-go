/**
 * 向後相容層 — 委派給 shared/mode-manager.js。
 * 僅保留 isEnabled() legacy API，其餘邏輯（三級模式、localStorage、UI 綁定）由 mode-manager.js 處理。
 *
 * 路徑：client_web/ 找不到 ./shared/mode-manager.js → esbuild shared plugin fallback 到 shared_web/static/js/shared/。
 */
import { getMode, init as initModeManager } from './shared/mode-manager.js';

/** @deprecated 使用 mode-manager.js 的 isSimple() 取代 */
export function isEnabled() { return getMode() === 'simple'; }

/** 委派給 mode-manager.js — 綁定 #simplifiedToggle、還原 localStorage 狀態、更新 UI */
export function init() { initModeManager(); }
