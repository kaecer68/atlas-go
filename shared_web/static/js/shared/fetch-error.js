// web/static/js/shared/fetch-error.js
//
// 對應 INVESTIGATION.md 模式 A/B/E：把 raw fetch / fetchJSON 拋出的錯誤
// 轉成結構化分類，讓前端可以顯示可行動錯誤訊息（不再原樣吐「Failed to fetch」）。
//
// 設計對齊 atlas-data-visibility L4 規範：
//   - kind 標記錯誤類別（network / http_503 / http_5xx / ...）
//   - message 給使用者看的一句話
//   - recoverable 標記是否值得 retry
//   - hint 給使用者可行動的下一步（啟動指令、檔案路徑）

/**
 * 分類 fetch / fetchJSON 拋出的錯誤。
 *
 * @param {Error|null|undefined} err  fetch 本身拋出的 TypeError（網路層），
 *                                    或 fetchJSON 附加 .status 的 Error（HTTP 層）。
 * @param {string} url  觸發此錯誤的 endpoint（用於錯誤訊息脈絡）。
 * @returns {{kind: string, message: string, recoverable: boolean, hint: string}}
 */
export function classifyFetchError(err, url) {
  // 1) Timeout：AbortController 中止後 fetch 拋 AbortError（DOMException）
  if (err && (err.name === 'AbortError' || err.code === 'ABORT_ERR')) {
    return {
      kind: 'timeout',
      message: '後端回應逾時（30 秒）',
      recoverable: true,
      hint: '請稍後重試，或檢查後端是否 hang',
    };
  }

  // 2) 網路層錯誤：fetch() 拋 TypeError（CORS、DNS、TCP RST、連線拒絕）
  if (err && (err instanceof TypeError || err.name === 'TypeError')) {
    return {
      kind: 'network',
      message: '後端未啟動或無法連線',
      recoverable: true,
      hint: '請確認 Go API 服務於 :8080 運行（執行 go run ./cmd/atlas --api）',
    };
  }

  // 3) HTTP 層錯誤：以 .status 區分子類別
  const status = err && typeof err.status === 'number' ? err.status : null;

  if (status === 503) {
    return {
      kind: 'http_503',
      message: '策略心法 registry 未初始化',
      recoverable: true,
      hint: '請檢查 data/seeds/strategy_techniques.json 是否存在',
    };
  }
  if (status === 410) {
    return {
      kind: 'http_410',
      message: '此資源已下線（HTTP 410）',
      recoverable: false,
      hint: '請重新整理頁面或聯絡管理員',
    };
  }
  if (status === 404) {
    return {
      kind: 'http_404',
      message: '找不到對應的資源（HTTP 404）',
      recoverable: false,
      hint: '該心法可能已被刪除',
    };
  }
  if (status >= 500 && status < 600) {
    return {
      kind: 'http_5xx',
      message: '後端服務錯誤（HTTP ' + status + '）',
      recoverable: true,
      hint: '請稍後重試',
    };
  }
  if (status >= 400 && status < 500) {
    return {
      kind: 'http_4xx',
      message: '請求被拒（HTTP ' + status + '）',
      recoverable: false,
      hint: '',
    };
  }

  // 4) Fallback：無 status 屬性的 Error、null、undefined
  var rawMessage = err && err.message ? err.message : '未知錯誤';
  return {
    kind: 'unknown',
    message: rawMessage,
    recoverable: false,
    hint: url ? ('URL: ' + url) : '',
  };
}
