export const template = `
  <div class="auth-page">
    <div class="auth-card panel">
      <h2>註冊</h2>
      <p class="auth-subtitle">建立帳號即可獲得 7 天 Premium 免費試用</p>
      <form id="registerForm" class="auth-form">
        <div class="form-group">
          <label for="registerEmail">Email</label>
          <input type="email" id="registerEmail" name="email" placeholder="your@email.com" required autocomplete="email">
        </div>
        <div class="form-group">
          <label for="registerPassword">密碼</label>
          <input type="password" id="registerPassword" name="password" placeholder="至少 8 碼" required minlength="8" autocomplete="new-password">
        </div>
        <div id="registerError" class="auth-error hidden"></div>
        <div id="registerSuccess" class="auth-success hidden"></div>
        <button type="submit" class="btn btn--primary btn-full">註冊</button>
      </form>
      <p class="auth-footer">已有帳號？<a href="/client/login" data-page="login" class="auth-link" onclick="event.preventDefault();window.switchPage('login')">立即登入</a></p>
      <p class="auth-footer"><a href="/client/home" data-page="home" class="auth-link" onclick="event.preventDefault();window.switchPage('home')">← 先看看公開內容</a></p>
    </div>
  </div>
`;

export async function init() {
  const { register, isLoggedIn, renderNavState } = await import('../services/auth.js');
  const loggedIn = await isLoggedIn();
  if (loggedIn) {
    window.switchPage('home');
    return;
  }
  const form = document.getElementById('registerForm');
  if (!form) return;
  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    const errorEl = document.getElementById('registerError');
    const successEl = document.getElementById('registerSuccess');
    errorEl.classList.add('hidden');
    successEl.classList.add('hidden');
    const btn = form.querySelector('button[type="submit"]');
    btn.disabled = true;
    btn.textContent = '註冊中…';
    try {
      // M4b: register is now a thin proxy to go-member, which requires email
      // verification before login — it returns {id,email,message} and NO
      // token, so the user is not auto-logged-in. Show the verify message.
      const res = await register(form.email.value, form.registerPassword.value);
      await renderNavState();
      successEl.textContent = (res && res.message) || '註冊成功！請檢查電子郵件完成驗證';
      successEl.classList.remove('hidden');
      btn.disabled = false;
      btn.textContent = '註冊';
    } catch (err) {
      errorEl.textContent = err.message || '註冊失敗，請稍後再試';
      errorEl.classList.remove('hidden');
      btn.disabled = false;
      btn.textContent = '註冊';
    }
  });
}
