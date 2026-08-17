/**
 * Atlas API key management for admin_web.
 *
 * The backend now requires ATLAS_API_KEY for mutating methods (POST/PUT/DELETE/PATCH).
 * This module stores the key in localStorage and provides a simple, one-time prompt
 * so admin users can enter it on the first page that needs write access.
 */

const STORAGE_KEY = 'ATLAS_API_KEY';

export function getApiKey() {
  try {
    return localStorage.getItem(STORAGE_KEY) || '';
  } catch (_) {
    return '';
  }
}

export function hasApiKey() {
  return getApiKey() !== '';
}

export function setApiKey(key) {
  try {
    if (key) {
      localStorage.setItem(STORAGE_KEY, key);
    } else {
      localStorage.removeItem(STORAGE_KEY);
    }
  } catch (_) {
    // localStorage may be unavailable in some environments.
  }
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
