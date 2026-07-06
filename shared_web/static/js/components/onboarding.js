const STORAGE_KEY = 'atlas-onboarded';
const STEPS = [
  {
    title: 'Atlas 平台歡迎你',
    body: '你的 AI 投資研究平台 — 每日市場建議、策略績效、產業分析，3 步快速上手。',
  },
  {
    title: '每日摘要',
    body: '首頁頂部的 4 個指標（方向、壓力、外資、大盤）幫你在 10 秒內判斷今日市場。',
  },
  {
    title: '開始使用',
    body: '點擊「查看市場詳情」深入數據，或從側邊欄探索更多功能。祝投資順利！',
  },
];

export function initOnboarding() {
  if (typeof document === 'undefined' || !document.body) return;
  try {
    if (localStorage.getItem(STORAGE_KEY) === '1') return;
  } catch (_) { return; }

  let step = 0;
  const overlay = document.createElement('div');
  overlay.className = 'onboard-overlay';

  function dismiss() {
    overlay.remove();
    try { localStorage.setItem(STORAGE_KEY, '1'); } catch (_) {}
  }

  function render() {
    const s = STEPS[step];
    const isLast = step === STEPS.length - 1;
    overlay.innerHTML = `<div class="onboard-card" role="dialog" aria-label="${s.title}">
      <div class="onboard-card__step">${step + 1}/${STEPS.length}</div>
      <div class="onboard-card__title">${s.title}</div>
      <div class="onboard-card__body">${s.body}</div>
      <div class="onboard-card__actions">
        <button class="onboard-card__btn onboard-card__btn--skip" id="ob-skip">略過</button>
        <button class="onboard-card__btn onboard-card__btn--primary" id="ob-next">${isLast ? '開始使用 →' : '下一步 →'}</button>
      </div>
    </div>`;
    overlay.querySelector('#ob-next').addEventListener('click', () => {
      if (isLast) { dismiss(); } else { step++; render(); }
    });
    overlay.querySelector('#ob-skip').addEventListener('click', dismiss);
  }

  overlay.addEventListener('click', (e) => {
    if (e.target === overlay) dismiss();
  });
  document.addEventListener('keydown', function escHandler(e) {
    if (e.key === 'Escape') { dismiss(); document.removeEventListener('keydown', escHandler); }
  });

  render();
  if (document.body) document.body.appendChild(overlay);
}
