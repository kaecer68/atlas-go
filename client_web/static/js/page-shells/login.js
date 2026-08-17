export const template = `
  <div class="auth-page">
    <div class="auth-card panel">
      <h2>登入</h2>
      <p class="auth-subtitle">登入後可解鎖個人儀表板與策略推薦</p>
      <form id="loginForm" class="auth-form">
        <div class="form-group">
          <label for="loginEmail">Email</label>
          <input type="email" id="loginEmail" name="email" placeholder="your@email.com" required autocomplete="email">
        </div>
        <div class="form-group">
          <label for="loginPassword">密碼</label>
          <input type="password" id="loginPassword" name="password" placeholder="至少 8 碼" required minlength="8" autocomplete="current-password">
        </div>
        <div id="loginError" class="auth-error hidden"></div>
        <button type="submit" class="btn btn--primary btn-full">登入</button>
      </form>
      <p class="auth-footer">還沒有帳號？<a href="/client/register" data-page="register" class="auth-link" onclick="event.preventDefault();window.switchPage('register')">立即註冊</a></p>
      <p class="auth-footer"><a href="/client/home" data-page="home" class="auth-link" onclick="event.preventDefault();window.switchPage('home')">← 先看看公開內容</a></p>
    </div>
  </div>
`;

export async function init() {
  const { login, isLoggedIn, renderNavState } = await import('../services/auth.js');
  const loggedIn = await isLoggedIn();
  if (loggedIn) {
    window.switchPage('home');
    return;
  }
  const form = document.getElementById('loginForm');
  const errorEl = document.getElementById('loginError');
  if (!form) return;
  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    errorEl.classList.add('hidden');
    errorEl.textContent = '';
    const btn = form.querySelector('button[type="submit"]');
    btn.disabled = true;
    btn.textContent = '登入中…';
    try {
      await login(form.email.value, form.loginPassword.value);
      await renderNavState();
      window.switchPage('home');
    } catch (err) {
      errorEl.textContent = err.message || '登入失敗，請檢查帳號密碼';
      errorEl.classList.remove('hidden');
    } finally {
      btn.disabled = false;
      btn.textContent = '登入';
    }
  });
}
