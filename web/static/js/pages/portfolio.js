export async function loadPortfolioPage(getJSON, agentNameFn) {
  const el = document.getElementById('portfolioContent');
  if (!el) return;
  el.innerHTML = '<div style="padding:20px;text-align:center;color:var(--muted)">組合持倉頁面載入中…</div>';
  try {
    const data = await getJSON('/api/dashboard/live-status');
    if (data && data.portfolio) {
      const p = data.portfolio;
      el.innerHTML = '<div class="panel"><h3>組合持倉</h3><pre>' + JSON.stringify(p, null, 2) + '</pre></div>';
    } else {
      el.innerHTML = '<div style="padding:20px;text-align:center;color:var(--muted)">尚無持倉資料</div>';
    }
  } catch (e) {
    el.innerHTML = '<div style="padding:20px;text-align:center;color:var(--down)">載入失敗</div>';
  }
}
