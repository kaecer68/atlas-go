export const template = `
  <div class="error-page">
    <div class="error-card panel">
      <h1 class="error-code">404</h1>
      <p class="error-message">找不到這個頁面</p>
      <p class="error-hint">可能是網址輸入錯誤，或是頁面已被移動。</p>
      <div class="error-actions">
        <button class="btn btn--primary" onclick="window.switchPage('home')">回首頁</button>
      </div>
    </div>
  </div>
`;

export async function init() {
  // Static page — no dynamic data needed.
}
