/**
 * Atlas API key management for admin_web.
 *
 * The backend now requires ATLAS_API_KEY for mutating methods (POST/PUT/DELETE/PATCH).
 * This module provides a one-time prompt so admin users can enter the key on the
 * first page that needs write access.
 *
 * 2026-08-18 (PR-7, Code Disposition Protocol「可退役」判定修正): storage/key 邏輯
 * 與 shared app-utils 的 getAtlasApiKey/setAtlasApiKey 完全重疊（同一 localStorage
 * key 'ATLAS_API_KEY'）→ 本模組保留 API 面（getApiKey/setApiKey/hasApiKey/
 * ensureApiKey 改為薄 re-export 指向 app-utils），刪除重複 localStorage 邏輯；
 * modal UI 邏輯（show/hide/initApiKeyPrompt）為活碼（admin main.js 使用）→ 保留不動。
 *
 * 注意: 為讓 node --test 可直接解析，import 用跨樹相對路徑指向 shared_web
 * （esbuild 與 node 皆可解析；admin main.js 的 './shared/...' 樣式依賴 esbuild
 * shared-static-fallback plugin，node 無法解析）。
 */

import { getAtlasApiKey, setAtlasApiKey } from '../../../../shared_web/static/js/shared/app-utils.js';

/** 讀取已儲存的管理員 API Key（薄 re-export，儲存實作統一在 app-utils）。 */
export function getApiKey() {
  return getAtlasApiKey();
}

/** 是否已儲存 API Key。 */
export function hasApiKey() {
  return getApiKey() !== '';
}

/** 儲存（或清除，傳空字串）管理員 API Key（薄 re-export）。 */
export function setApiKey(key) {
  setAtlasApiKey(key);
}

/**
 * Show the API key prompt if no key is stored.
 * @returns {boolean} true if a key is available (already stored or just entered).
 */
export function ensureApiKey() {
  if (hasApiKey()) return true;
  showApiKeyPrompt();
  return hasApiKey();
}

/** Show the API key prompt modal. */
export function showApiKeyPrompt() {
  const modal = document.getElementById('apiKeyModal');
  if (modal) {
    modal.classList.remove('hidden');
  }
}

/** Hide the API key prompt modal. */
export function hideApiKeyPrompt() {
  const modal = document.getElementById('apiKeyModal');
  if (modal) {
    modal.classList.add('hidden');
  }
}

/** Wire the modal save button on page load. */
export function initApiKeyPrompt() {
  const saveBtn = document.getElementById('apiKeySave');
  const input = document.getElementById('apiKeyInput');
  if (!saveBtn || !input) return;

  saveBtn.addEventListener('click', () => {
    const key = input.value.trim();
    if (key) {
      setApiKey(key);
      hideApiKeyPrompt();
    }
  });

  input.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') {
      saveBtn.click();
    }
  });
}
