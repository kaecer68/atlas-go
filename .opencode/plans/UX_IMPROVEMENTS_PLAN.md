# Atlas-Go Dashboard UX Improvements — Execution Plan

**Target File**: `web/static/index.html` (2999 lines, single-file vanilla HTML/CSS/JS dashboard)
**Project**: atlas-go (Taiwan equity investment research system)
**Approach**: Sequential, section-by-section (CSS → HTML → JS) to minimize re-reading
**Total Items**: 15
**Estimated Total Time**: 6-8 hours
**Atomic Commits**: 1 per phase (4 total)

---

## File Structure Map

```
Lines 1-264    CSS Styles (:root variables, layout, components)
Lines 265-584  HTML Structure (sidebar, pages, modals)
Lines 585-2999 JavaScript (utilities, render functions, event handlers)
```

**Key Functions & Their Line Numbers**:
- `loadAll()` — Line 2592 (main refresh, 20+ parallel API calls)
- `notify()` — Line 914 (toast notification system, currently ignores `type` param)
- `switchPage()` — Line 589 (page navigation)
- `getJSON()/postJSON()` — Lines 891-912 (API wrappers)
- `renderEmptyState()` — Line 938 (empty state helper)
- `renderOverview()` — Line 808
- `renderPipeline()` — Line 999+
- `renderAlerts()` — Line 2506

---

## Phase 1: CSS Foundation (Items 6, 8, 10, 12, 13)

**Why first**: All visual improvements need CSS support before JS can use them. Skeleton screens, color unification, and typography must exist before JS references them.

### 1.1 Item 12 — Color Variable Unification (Complexity: M)
**Line Range**: Lines 10-22 (CSS variables)

**Current State**:
```css
--up: #26a17b; --down: #d93a3a; --warn: #f5a623;
--status-ok: #10b981; --status-warn: #f59e0b; --status-err: #ef4444;
```

**Problem**: Two naming conventions for the same semantic colors.

**Action**:
1. Add unified semantic variables after line 16:
   ```css
   --color-success: var(--up);
   --color-danger: var(--down);
   --color-warning: var(--warn);
   --color-info: var(--accent);
   ```
2. Replace all `--status-*` usages with `--color-*` equivalents throughout CSS (lines 51-53, 205, etc.)
3. Keep old variables for backward compatibility during transition
4. Update `.badge.ok` background to use `rgba()` with `--color-success`

**Success Criteria**:
- [ ] All semantic colors use unified naming
- [ ] No visual regression in badges, alerts, or KPI cards
- [ ] Dark/light theme both work correctly

**Dependencies**: None (foundational)

---

### 1.2 Item 13 — Typography Hierarchy (Complexity: S)
**Line Range**: Lines 26-28, 37-38, 201-203 (headers and text sizes)

**Current State**:
- `header h1` — 20px
- `.panel h2` — 15px
- `.kpi-card .kpi-value` — 22px
- `.kpi-card .kpi-label` — 12px
- `.kpi-card .kpi-hint` — 11px
- Body text — inherited (typically 16px)

**Problem**: Insufficient contrast between heading levels and body text.

**Action**:
1. Add CSS variables after line 14:
   ```css
   --text-xs: 11px; --text-sm: 12px; --text-md: 14px; --text-lg: 16px;
   --text-xl: 20px; --text-2xl: 24px; --text-3xl: 32px;
   ```
2. Update heading hierarchy:
   - `header h1`: 20px → 24px (line 27)
   - `.panel h2`: 15px → 18px (line 38)
   - Add `.panel h3`: 14px (for sub-sections)
   - `.kpi-card .kpi-value`: 22px → 28px (line 202)
   - `.kpi-card .kpi-label`: 12px → 11px (reduce for contrast, line 201)
   - `.kpi-card .kpi-hint`: 11px → 11px (keep, add `letter-spacing: 0.02em`)

**Success Criteria**:
- [ ] Clear visual hierarchy: page title > section headers > card titles > values > labels > hints
- [ ] All text sizes use CSS variables
- [ ] No overlap or clipping at any viewport width

**Dependencies**: None

---

### 1.3 Item 6 — Sidebar Width Increase (Complexity: S)
**Line Range**: Lines 119-162, 246-249

**Current State**:
```css
#sidebar { width: 144px; }
#main { margin-left: 144px; }
```

**Problem**: Labels like "AI 觀測台" and "信息通道" may truncate.

**Action**:
1. Update sidebar width (line 120):
   ```css
   #sidebar { width: 170px; }
   ```
2. Update main margin (line 162):
   ```css
   #main { margin-left: 170px; }
   ```
3. Add to sidebar nav links (line 138-153):
   ```css
   #sidebar nav a {
     white-space: nowrap;
     overflow: hidden;
     text-overflow: ellipsis;
   }
   ```
4. Add `title` attributes to all nav `<a>` tags in HTML (lines 275-288):
   ```html
   <a data-page="agents" onclick="switchPage('agents')" title="AI 觀測台">AI 觀測台</a>
   ```

**Success Criteria**:
- [ ] Sidebar width is 170px
- [ ] Long labels show ellipsis instead of wrapping
- [ ] Hovering shows full label via tooltip
- [ ] Mobile breakpoint (900px) still works

**Dependencies**: None

---

### 1.4 Item 8 — Skeleton Loading Screens (Complexity: M)
**Line Range**: Lines 251-262 (add after existing loading styles)

**Current State**:
```css
.loading::before { /* spinner animation */ }
```
And HTML elements show "載入中…" text.

**Problem**: Text-only loading states are jarring and don't indicate content structure.

**Action**:
1. Add skeleton CSS after line 262:
   ```css
   @keyframes shimmer {
     0% { background-position: -200% 0; }
     100% { background-position: 200% 0; }
   }
   .skeleton {
     background: linear-gradient(90deg, var(--bg) 25%, var(--border) 50%, var(--bg) 75%);
     background-size: 200% 100%;
     animation: shimmer 1.5s infinite;
     border-radius: 4px;
   }
   .skeleton-text { height: 14px; margin: 4px 0; }
   .skeleton-title { height: 18px; width: 60%; margin: 8px 0; }
   .skeleton-card {
     background: var(--panel);
     border: 1px solid var(--border);
     border-radius: 12px;
     padding: var(--space-md);
   }
   ```
2. Create `showSkeleton(elementId, type)` JS function (add after line 587):
   ```javascript
   function showSkeleton(elementId, type = 'text') {
     const el = document.getElementById(elementId);
     if (!el) return;
     el.innerHTML = `<div class="skeleton skeleton-${type}"></div>`;
     el.classList.add('loading');
   }
   ```

**Success Criteria**:
- [ ] All `.loading` elements show animated skeleton blocks instead of text
- [ ] Skeletons match the rough shape of content (title bars, text lines)
- [ ] Animation is smooth and not distracting
- [ ] Content replaces skeleton seamlessly when loaded

**Dependencies**: None (but JS integration in Phase 3)

---

### 1.5 Item 10 — Mobile Optimization (Complexity: M)
**Line Range**: Lines 239-249 (existing media queries), plus additions

**Current State**:
- Mobile sidebar toggle exists
- Tables have `.table-wrapper` for horizontal scroll
- Touch targets may be too small

**Problem**: Touch targets < 44px, no scroll indicators for tables.

**Action**:
1. Add touch target minimums (after line 249):
   ```css
   @media (pointer: coarse) {
     button, .pipeline-action, .badge, #sidebar nav a {
       min-height: 44px;
       min-width: 44px;
     }
     .control-group input, .control-group select {
       min-height: 44px;
     }
   }
   ```
2. Add table scroll indicator (after line 114):
   ```css
   .table-wrapper {
     overflow-x: auto;
     max-width: 100%;
     /* Scroll hint shadow */
     background: linear-gradient(to right, var(--panel) 30%, rgba(0,0,0,0)),
                 linear-gradient(to left, var(--panel) 30%, rgba(0,0,0,0));
     background-attachment: local, local;
     background-position: left center, right center;
     background-repeat: no-repeat;
     background-size: 20px 100%, 20px 100%;
   }
   ```
3. Improve mobile sidebar (line 239-244):
   ```css
   @media (max-width: 900px) {
     #sidebar { 
       transform: translateX(-100%); 
       transition: transform .25s cubic-bezier(0.4, 0, 0.2, 1);
       width: 260px; /* Larger for touch */
     }
     #sidebar.open { transform: translateX(0); }
     #main { margin-left: 0; }
     #menuToggle { 
       display: inline-block; 
       min-height: 44px;
       min-width: 44px;
     }
   }
   ```

**Success Criteria**:
- [ ] All interactive elements are ≥ 44px on touch devices
- [ ] Tables show scroll shadow indicators
- [ ] Mobile sidebar is wider and easier to tap
- [ ] No layout breakage on iPhone SE (375px width)

**Dependencies**: None

---

## Phase 2: HTML Structure (Items 1, 5, 7, 11)

**Why second**: HTML changes add structural elements that JS will manipulate.

### 2.1 Item 1 — Loading State Indicator (Complexity: M)
**Line Range**: Lines 294-300 (topbar area)

**Current State**:
```html
<div id="topbar">
  <div class="flex-center-gap">
    <button id="menuToggle" onclick="toggleSidebar()">☰</button>
    <h2 id="pageTitle">總覽</h2>
  </div>
  <div class="meta" id="lastRefresh">-</div>
</div>
```

**Problem**: No visual feedback when `loadAll()` is running.

**Action**:
1. Add loading indicator to topbar (after line 298):
   ```html
   <div id="loadingIndicator" class="loading-spinner hidden">
     <span class="spinner"></span>
     <span class="loading-text">更新中...</span>
   </div>
   ```
2. Add CSS for loading indicator (in Phase 1 CSS):
   ```css
   .loading-spinner {
     display: flex;
     align-items: center;
     gap: 8px;
     font-size: 12px;
     color: var(--accent);
   }
   .loading-spinner.hidden { display: none; }
   .loading-spinner .spinner {
     width: 14px; height: 14px;
     border: 2px solid var(--border);
     border-top-color: var(--accent);
     border-radius: 50%;
     animation: spin 1s linear infinite;
   }
   ```
3. Update `loadAll()` (line 2592) to show/hide:
   ```javascript
   async function loadAll() {
     const loader = document.getElementById('loadingIndicator');
     if (loader) loader.classList.remove('hidden');
     try {
       // ... existing code ...
     } finally {
       if (loader) loader.classList.add('hidden');
     }
   }
   ```

**Success Criteria**:
- [ ] Spinner appears when `loadAll()` starts
- [ ] Spinner hides when `loadAll()` completes (success or error)
- [ ] Text reads "更新中..." during load
- [ ] Doesn't interfere with existing topbar layout

**Dependencies**: Phase 1 CSS for spinner animation

---

### 2.2 Item 5 — Update Time Visualization (Complexity: M)
**Line Range**: Lines 294-300 (replace existing `lastRefresh`)

**Current State**:
```html
<div class="meta" id="lastRefresh">-</div>
```

**Problem**: Plain text, no indication of auto-refresh status.

**Action**:
1. Replace line 300 with:
   ```html
   <div class="refresh-status" id="lastRefresh">
     <span class="refresh-badge" id="refreshBadge">更新於 --:--:--</span>
     <button class="refresh-toggle" id="refreshToggle" onclick="toggleAutoRefresh()" title="暫停自動更新">
       <span class="refresh-icon">⏸</span>
     </button>
   </div>
   ```
2. Add CSS (in Phase 1):
   ```css
   .refresh-status {
     display: flex;
     align-items: center;
     gap: 8px;
   }
   .refresh-badge {
     background: var(--panel);
     border: 1px solid var(--border);
     padding: 4px 10px;
     border-radius: 999px;
     font-size: 12px;
     color: var(--muted);
   }
   .refresh-badge.active {
     border-color: var(--up);
     color: var(--up);
   }
   .refresh-badge.paused {
     border-color: var(--warn);
     color: var(--warn);
   }
   .refresh-toggle {
     background: transparent;
     border: 1px solid var(--border);
     border-radius: 6px;
     padding: 4px 8px;
     cursor: pointer;
     font-size: 12px;
   }
   ```
3. Add JS function (after line 2744):
   ```javascript
   let autoRefreshEnabled = true;
   let refreshIntervalId = null;
   
   function toggleAutoRefresh() {
     autoRefreshEnabled = !autoRefreshEnabled;
     const badge = document.getElementById('refreshBadge');
     const toggle = document.getElementById('refreshToggle');
     
     if (autoRefreshEnabled) {
       refreshIntervalId = setInterval(loadAll, 30000);
       badge.classList.add('active');
       badge.classList.remove('paused');
       toggle.innerHTML = '<span class="refresh-icon">⏸</span>';
       toggle.title = '暫停自動更新';
       notify('自動更新已啟用', 'success');
     } else {
       clearInterval(refreshIntervalId);
       badge.classList.remove('active');
       badge.classList.add('paused');
       toggle.innerHTML = '<span class="refresh-icon">▶</span>';
       toggle.title = '恢復自動更新';
       notify('自動更新已暫停', 'warn');
     }
   }
   ```
4. Update `loadAll()` line 2636:
   ```javascript
   const badge = document.getElementById('refreshBadge');
   if (badge) {
     badge.textContent = '更新於 ' + new Date().toLocaleTimeString('zh-TW', {hour:'2-digit', minute:'2-digit', second:'2-digit'});
     badge.classList.add('active');
   }
   ```

**Success Criteria**:
- [ ] Badge shows "更新於 HH:MM:SS" format
- [ ] Badge has green border when active, yellow when paused
- [ ] Pause/resume button toggles auto-refresh
- [ ] Notification confirms state change
- [ ] Manual refresh button still works

**Dependencies**: Phase 1 CSS, Item 3 (notify types)

---

### 2.3 Item 7 — Keyboard Navigation (Complexity: M)
**Line Range**: Lines 275-288 (sidebar nav links)

**Current State**:
```html
<a data-page="overview" onclick="switchPage('overview')">總覽</a>
```

**Problem**: No `href`, no `tabindex`, not keyboard accessible.

**Action**:
1. Update all nav links to be keyboard accessible:
   ```html
   <a class="active" 
      data-page="overview" 
      href="#overview"
      tabindex="0"
      role="button"
      aria-label="總覽頁面"
      onclick="event.preventDefault(); switchPage('overview')"
      onkeydown="if(event.key==='Enter'||event.key===' ') { event.preventDefault(); switchPage('overview'); }">
     總覽
   </a>
   ```
2. Add CSS for focus states (in Phase 1):
   ```css
   #sidebar nav a:focus-visible {
     outline: 2px solid var(--accent);
     outline-offset: -2px;
     background: rgba(79,193,255,.12);
   }
   ```
3. Update `switchPage()` (line 589) to handle focus:
   ```javascript
   function switchPage(id) {
     // ... existing code ...
     // Set focus to page content for screen readers
     const pageContent = document.getElementById('page-' + id);
     if (pageContent) {
       pageContent.setAttribute('tabindex', '-1');
       pageContent.focus({ preventScroll: true });
     }
   }
   ```

**Success Criteria**:
- [ ] Tab key cycles through all nav items
- [ ] Enter/Space activates nav item
- [ ] Focus indicator is visible
- [ ] Screen reader announces page changes
- [ ] No JavaScript errors on keyboard interaction

**Dependencies**: Phase 1 CSS for focus styles

---

### 2.4 Item 11 — Dynamic Backtest Dates (Complexity: S)
**Line Range**: Lines 404-408

**Current State**:
```html
<input type="date" id="backtestStart" value="2026-03-26">
<input type="date" id="backtestEnd" value="2026-03-27">
```

**Problem**: Hardcoded dates will become stale.

**Action**:
1. Add JS function to compute recent trading day (after line 2744):
   ```javascript
   function getRecentTradingDay(daysAgo = 1) {
     const date = new Date();
     date.setDate(date.getDate() - daysAgo);
     // Skip weekends
     while (date.getDay() === 0 || date.getDay() === 6) {
       date.setDate(date.getDate() - 1);
     }
     return date.toISOString().split('T')[0];
   }
   
   function initBacktestDates() {
     const startInput = document.getElementById('backtestStart');
     const endInput = document.getElementById('backtestEnd');
     if (startInput && !startInput.value) {
       startInput.value = getRecentTradingDay(2); // 2 days ago
     }
     if (endInput && !endInput.value) {
       endInput.value = getRecentTradingDay(1); // 1 day ago
     }
   }
   ```
2. Call `initBacktestDates()` in initialization (after line 2743):
   ```javascript
   populateAgentSelect();
   initBacktestDates();
   loadAll();
   ```

**Success Criteria**:
- [ ] Default start date is 2 trading days ago
- [ ] Default end date is 1 trading day ago
- [ ] Skips weekends automatically
- [ ] User can still override manually
- [ ] Dates are in YYYY-MM-DD format

**Dependencies**: None

---

## Phase 3: JavaScript Logic (Items 2, 3, 4, 9, 14)

**Why third**: JS builds on CSS and HTML changes. Error handling needs loading states, pagination needs skeletons, etc.

### 3.1 Item 2 — Error Handling Layers (Complexity: L)
**Line Range**: Lines 891-912 (`getJSON`/`postJSON`), Lines 2592-2641 (`loadAll`)

**Current State**:
- `getJSON()` catches errors and calls `notify()` with 'error' type
- `loadAll()` catches all errors in one block
- No differentiation between initial load and auto-refresh

**Problem**: Initial load failure should show error state; auto-refresh failure should be quieter.

**Action**:
1. Add error handling state tracking (after line 587):
   ```javascript
   const loadState = {
     isInitialLoad: true,
     lastSuccessfulLoad: null,
     consecutiveErrors: 0,
     maxConsecutiveErrors: 3
   };
   ```
2. Update `getJSON()` (line 891):
   ```javascript
   async function getJSON(url, options = {}) {
     const { silent = false, retryCount = 0 } = options;
     try {
       const r = await fetch(url);
       if (!r.ok) throw new Error(`HTTP ${r.status}`);
       return r.json();
     } catch (err) {
       if (!silent && err.message && !err.message.includes('404')) {
         if (loadState.isInitialLoad) {
           // Initial load: show prominent error
           notify(`載入失敗: ${url.split('/').pop()} - ${err.message}`, 'error');
         } else if (loadState.consecutiveErrors < loadState.maxConsecutiveErrors) {
           // Auto-refresh: show subtle warning, keep old data
           notify(`更新失敗 (${loadState.consecutiveErrors + 1}/${loadState.maxConsecutiveErrors}): ${url.split('/').pop()}`, 'warn');
         }
       }
       throw err;
     }
   }
   ```
3. Update `loadAll()` (line 2592):
   ```javascript
   async function loadAll() {
     const loader = document.getElementById('loadingIndicator');
     if (loader) loader.classList.remove('hidden');
     
     try {
       const results = await Promise.all([
         getJSON('/api/dashboard/system-health', { silent: !loadState.isInitialLoad }).catch(() => null),
         // ... other calls with silent option ...
       ]);
       
       // Render all components
       // ... existing render calls ...
       
       loadState.lastSuccessfulLoad = new Date();
       loadState.consecutiveErrors = 0;
       
       if (loadState.isInitialLoad) {
         loadState.isInitialLoad = false;
       }
     } catch (e) {
       loadState.consecutiveErrors++;
       console.error('loadAll error:', e);
       
       if (loadState.isInitialLoad) {
         // Show full error state
         document.getElementById('content').innerHTML = `
           <div class="panel wide" style="text-align:center;padding:40px">
             <div style="font-size:18px;color:var(--down);margin-bottom:12px">⚠ 系統載入失敗</div>
             <div style="color:var(--muted);margin-bottom:20px">無法連接到 Atlas 後端服務</div>
             <button class="primary" onclick="location.reload()">重新整理頁面</button>
           </div>
         `;
       }
     } finally {
       if (loader) loader.classList.add('hidden');
     }
   }
   ```

**Success Criteria**:
- [ ] Initial load failure shows full-page error with reload button
- [ ] Auto-refresh failure shows subtle toast (max 3 times)
- [ ] Old data persists during auto-refresh failures
- [ ] Error counter resets on successful load
- [ ] 404 errors are silently ignored (existing behavior preserved)

**Dependencies**: Item 1 (loading indicator), Item 3 (notify types)

---

### 3.2 Item 3 — Notification Type Colors (Complexity: S)
**Line Range**: Lines 914-920 (`notify()` function), Lines 93-96 (notification CSS)

**Current State**:
```javascript
function notify(msg, type='info') {
  // type parameter is completely ignored!
  const n = document.createElement('div');
  n.className = 'notification';
  // ...
}
```

**Problem**: `type` parameter is ignored; all notifications look identical.

**Action**:
1. Update `notify()` function (line 914):
   ```javascript
   function notify(msg, type = 'info') {
     const nc = document.getElementById('notificationCenter');
     const n = document.createElement('div');
     
     // Map type to CSS class and icon
     const typeConfig = {
       info:    { class: 'notification-info',    icon: 'ℹ', borderColor: 'var(--accent)' },
       success: { class: 'notification-success', icon: '✓', borderColor: 'var(--up)' },
       warn:    { class: 'notification-warn',    icon: '⚠', borderColor: 'var(--warn)' },
       error:   { class: 'notification-error',   icon: '✕', borderColor: 'var(--down)' }
     };
     
     const config = typeConfig[type] || typeConfig.info;
     n.className = `notification ${config.class}`;
     n.style.borderLeft = `3px solid ${config.borderColor}`;
     n.innerHTML = `
       <span class="close" onclick="this.parentElement.remove()">×</span>
       <span class="notification-icon">${config.icon}</span>
       <div class="notification-content">${msg}</div>
     `;
     
     nc.appendChild(n);
     
     // Auto-remove after delay based on severity
     const delay = type === 'error' ? 12000 : type === 'warn' ? 10000 : 8000;
     setTimeout(() => n.remove(), delay);
   }
   ```
2. Add CSS (in Phase 1):
   ```css
   .notification {
     display: flex;
     align-items: flex-start;
     gap: 8px;
   }
   .notification-icon {
     font-size: 14px;
     margin-top: 1px;
   }
   .notification-content {
     flex: 1;
   }
   .notification-info    { border-left-color: var(--accent); }
   .notification-success { border-left-color: var(--up); }
   .notification-warn    { border-left-color: var(--warn); }
   .notification-error   { border-left-color: var(--down); }
   ```

**Success Criteria**:
- [ ] Info notifications have blue left border
- [ ] Success notifications have green left border
- [ ] Warning notifications have yellow left border
- [ ] Error notifications have red left border
- [ ] Each type has appropriate icon
- [ ] Error notifications persist longer (12s)

**Dependencies**: Phase 1 CSS (color variables)

---

### 3.3 Item 4 — Modal ESC Close (Complexity: S)
**Line Range**: Lines 547-583 (modal HTML), plus JS event listener

**Current State**:
- 3 modals: diffModal, promoteModal, infoModal
- Each has close button and overlay click handler
- No keyboard support

**Action**:
1. Add global ESC key listener (after line 2744):
   ```javascript
   document.addEventListener('keydown', (e) => {
     if (e.key === 'Escape') {
       closeModal();
       closePromoteModal();
       closeInfoModal();
     }
   });
   ```
2. Ensure close functions exist and are robust:
   ```javascript
   function closeModal() {
     const modal = document.getElementById('diffModal');
     if (modal) modal.classList.remove('show');
   }
   
   function closePromoteModal() {
     const modal = document.getElementById('promoteModal');
     if (modal) modal.classList.remove('show');
   }
   
   function closeInfoModal() {
     const modal = document.getElementById('infoModal');
     if (modal) modal.classList.remove('show');
   }
   ```

**Success Criteria**:
- [ ] Pressing ESC closes any open modal
- [ ] Modal close functions are idempotent (no errors if modal already closed)
- [ ] Focus returns to trigger element when modal closes
- [ ] Works for all 3 modals

**Dependencies**: None

---

### 3.4 Item 9 — Table Pagination (Complexity: L)
**Line Range**: Multiple table render functions (Lines 999+, 2506+, etc.)

**Current State**:
- Tables render all rows at once
- No pagination controls
- Large datasets cause performance issues

**Problem**: Tables with >50 rows are unwieldy.

**Action**:
1. Add pagination state (after line 587):
   ```javascript
   const paginationState = {
     pipeline: { page: 1, perPage: 50 },
     alerts: { page: 1, perPage: 50 },
     agents: { page: 1, perPage: 50 }
   };
   ```
2. Create pagination utility (after line 943):
   ```javascript
   function renderPagination(tableId, currentPage, totalPages, onPageChange) {
     if (totalPages <= 1) return '';
     
     let html = '<div class="pagination" style="display:flex;justify-content:center;align-items:center;gap:8px;margin-top:12px;padding-top:12px;border-top:1px solid var(--border)">';
     
     // Previous button
     html += `<button class="pipeline-action" ${currentPage === 1 ? 'disabled' : ''} onclick="${onPageChange}(${currentPage - 1})">← 上一頁</button>`;
     
     // Page info
     html += `<span style="font-size:12px;color:var(--muted)">第 ${currentPage} / ${totalPages} 頁</span>`;
     
     // Next button
     html += `<button class="pipeline-action" ${currentPage === totalPages ? 'disabled' : ''} onclick="${onPageChange}(${currentPage + 1})">下一頁 →</button>`;
     
     html += '</div>';
     return html;
   }
   
   function paginateData(data, page, perPage) {
     const start = (page - 1) * perPage;
     const end = start + perPage;
     return {
       items: data.slice(start, end),
       totalPages: Math.ceil(data.length / perPage),
       totalItems: data.length
     };
   }
   ```
3. Update `renderPipeline()` (around line 999+):
   ```javascript
   function renderPipeline(data, showFiltered = false, filterSymbol = '') {
     // ... existing filtering logic ...
     
     const state = paginationState.pipeline;
     const paginated = paginateData(items, state.page, state.perPage);
     
     // Render paginated.items instead of items
     // ... existing rendering ...
     
     // Add pagination controls
     html += renderPagination('pipeline', state.page, paginated.totalPages, 'changePipelinePage');
     
     el.innerHTML = html;
   }
   
   function changePipelinePage(page) {
     paginationState.pipeline.page = page;
     loadPageData('pipeline');
   }
   ```
4. Do the same for `renderAlerts()` and other large tables.

**Success Criteria**:
- [ ] Tables with ≤50 rows show all data (no pagination)
- [ ] Tables with >50 rows show pagination controls
- [ ] Page navigation works correctly
- [ ] Previous/Next buttons disable at boundaries
- [ ] Page state persists during auto-refresh

**Dependencies**: None

---

### 3.5 Item 14 — CSV Export (Complexity: M)
**Line Range**: Add to table render functions

**Current State**:
- No export functionality
- Tables are HTML-only

**Action**:
1. Add CSV export utility (after line 943):
   ```javascript
   function exportTableToCSV(tableId, filename) {
     const table = document.querySelector(`#${tableId} table`);
     if (!table) return;
     
     let csv = [];
     const rows = table.querySelectorAll('tr');
     
     rows.forEach(row => {
       const cols = row.querySelectorAll('td, th');
       const rowData = [];
       cols.forEach(col => {
         // Clean text content
         let text = col.textContent.replace(/"/g, '""').trim();
         if (text.includes(',') || text.includes('\n')) {
           text = `"${text}"`;
         }
         rowData.push(text);
       });
       csv.push(rowData.join(','));
     });
     
     const blob = new Blob(['\uFEFF' + csv.join('\n')], { type: 'text/csv;charset=utf-8;' });
     const link = document.createElement('a');
     link.href = URL.createObjectURL(blob);
     link.download = filename;
     link.click();
     URL.revokeObjectURL(link.href);
     
     notify(`已匯出 ${filename}`, 'success');
   }
   ```
2. Add export buttons to table panels. Example for pipeline (around line 364):
   ```html
   <div class="panel wide">
     <div class="flex-between mb-sm">
       <h2 class="m-0">最新場次推薦明細</h2>
       <button class="pipeline-action" onclick="exportTableToCSV('recommendationPipeline', 'recommendations.csv')">
         📥 匯出 CSV
       </button>
     </div>
     <div id="recommendationPipeline" class="empty loading">載入中…</div>
   </div>
   ```

**Success Criteria**:
- [ ] Export button appears on tables with data
- [ ] CSV file downloads with UTF-8 BOM (for Excel compatibility)
- [ ] Filename includes timestamp
- [ ] Special characters are properly escaped
- [ ] Success notification confirms download

**Dependencies**: Item 3 (notify types for success message)

---

## Phase 4: Final Verification (Item 15)

### 4.1 Verification Checklist

**Line Range**: Entire file

**Action**:
1. Run Go formatting and build:
   ```bash
   test -z "$(gofmt -l .)"
   go build ./...
   go test ./...
   go vet ./...
   staticcheck ./...
   ```
2. Manual browser testing:
   - [ ] All pages load without console errors
   - [ ] Loading spinner appears during initial load
   - [ ] Auto-refresh works and shows badge updates
   - [ ] Pause/resume toggle works
   - [ ] Notifications show correct colors for each type
   - [ ] ESC closes modals
   - [ ] Keyboard navigation works in sidebar
   - [ ] Skeleton screens appear during loading
   - [ ] Tables paginate correctly
   - [ ] CSV export downloads valid files
   - [ ] Mobile view (375px) is usable
   - [ ] Touch targets are large enough
   - [ ] Dark/light theme toggle works
   - [ ] Backtest dates default to recent trading days

3. Accessibility check:
   - [ ] All interactive elements have focus indicators
   - [ ] ARIA labels are present
   - [ ] Color contrast meets WCAG 2.1 AA
   - [ ] Keyboard-only navigation is possible

**Success Criteria**:
- [ ] All CI checks pass
- [ ] No console errors
- [ ] All 15 items verified working
- [ ] No visual regressions

---

## Atomic Commit Strategy

```bash
# Commit 1: Phase 1 — CSS Foundation
git add web/static/index.html
git commit -m "feat(dashboard): CSS foundation for UX improvements

- Unify color variables (--color-success/danger/warning/info)
- Add typography hierarchy with CSS variables
- Increase sidebar width to 170px with tooltips
- Add skeleton loading screen styles
- Add mobile touch targets and scroll indicators

Refs: items 6, 8, 10, 12, 13"

# Commit 2: Phase 2 — HTML Structure
git add web/static/index.html
git commit -m "feat(dashboard): HTML structure improvements

- Add loading spinner indicator to topbar
- Add refresh status badge with pause/resume toggle
- Make sidebar nav keyboard accessible (tabindex, Enter/Space)
- Add dynamic backtest date initialization

Refs: items 1, 5, 7, 11"

# Commit 3: Phase 3 — JavaScript Logic
git add web/static/index.html
git commit -m "feat(dashboard): JavaScript UX enhancements

- Differentiate initial load vs auto-refresh error handling
- Add notification type colors (info/success/warn/error)
- Add ESC key modal close support
- Add table pagination for large datasets
- Add CSV export functionality

Refs: items 2, 3, 4, 9, 14"

# Commit 4: Phase 4 — Verification
git add web/static/index.html
git commit -m "test(dashboard): Final verification and polish

- Run gofmt, go build, go test
- Verify all 15 items working correctly
- Fix any regressions

Refs: item 15"
```

---

## TDD-Oriented Planning

### Test Criteria Per Item

| Item | Test Input | Expected Output | Verification Method |
|------|-----------|-----------------|-------------------|
| 1 (Loading) | Click refresh button | Spinner appears in topbar | Visual inspection |
| 2 (Error handling) | Block network, reload page | Full error page with reload button | DevTools Network tab |
| 2 (Error handling) | Block network after load | Subtle toast, old data persists | DevTools Network tab |
| 3 (Notify colors) | `notify('test', 'error')` | Red-bordered notification | Visual inspection |
| 4 (ESC close) | Open modal, press ESC | Modal closes | Keyboard test |
| 5 (Refresh badge) | Wait 30s | Badge updates with timestamp | Visual inspection |
| 5 (Refresh toggle) | Click pause button | Badge turns yellow, auto-refresh stops | Visual + console |
| 6 (Sidebar) | Resize to 170px | Labels don't wrap, tooltips on hover | Visual inspection |
| 7 (Keyboard nav) | Press Tab 3 times | Focus moves to 3rd nav item | Keyboard test |
| 8 (Skeleton) | Throttle network to 3G | Skeleton blocks appear | DevTools Network |
| 9 (Pagination) | Load table with 100 rows | Shows page 1 of 2, navigation works | Visual inspection |
| 10 (Mobile) | DevTools iPhone SE | All buttons ≥ 44px, tables scrollable | DevTools Device Mode |
| 11 (Backtest dates) | Clear inputs, reload | Defaults to last 2 trading days | Visual inspection |
| 12 (Colors) | Check badge colors | All use unified variables | CSS inspection |
| 13 (Typography) | Check heading sizes | Clear hierarchy 24px > 18px > 16px | CSS inspection |
| 14 (CSV export) | Click export button | Downloads valid CSV file | File download test |

### Regression Tests

- [ ] All existing pages still render correctly
- [ ] All existing API calls still work
- [ ] Theme toggle still works
- [ ] Mobile sidebar still toggles
- [ ] All modals still open/close correctly
- [ ] No new console errors

---

## Dependency Graph

```
Phase 1 (CSS)
├── Item 12 (Color vars) ──→ Item 3 (Notify colors)
├── Item 13 (Typography) ──→ All visual items
├── Item 6 (Sidebar) ──────→ Item 7 (Keyboard nav)
├── Item 8 (Skeleton) ─────→ Item 1 (Loading state)
└── Item 10 (Mobile) ──────→ All touch-related items

Phase 2 (HTML)
├── Item 1 (Loading) ──────→ Item 2 (Error handling)
├── Item 5 (Refresh badge) ─→ Item 3 (Notify types)
├── Item 7 (Keyboard) ─────→ Item 4 (Modal ESC)
└── Item 11 (Backtest) ────→ None

Phase 3 (JS)
├── Item 2 (Error handling) ─→ All data loading
├── Item 3 (Notify colors) ──→ Item 14 (CSV export)
├── Item 4 (Modal ESC) ──────→ None
├── Item 9 (Pagination) ─────→ All table renders
└── Item 14 (CSV export) ────→ None
```

---

## Complexity Summary

| Item | Complexity | Time Estimate | Risk Level |
|------|-----------|---------------|------------|
| 12. Color vars | M | 30 min | Low |
| 13. Typography | S | 20 min | Low |
| 6. Sidebar width | S | 15 min | Low |
| 8. Skeleton screens | M | 45 min | Medium |
| 10. Mobile optimization | M | 40 min | Medium |
| 1. Loading indicator | M | 30 min | Low |
| 5. Refresh badge | M | 45 min | Medium |
| 7. Keyboard nav | M | 35 min | Medium |
| 11. Backtest dates | S | 15 min | Low |
| 2. Error handling | L | 60 min | High |
| 3. Notify colors | S | 20 min | Low |
| 4. Modal ESC | S | 10 min | Low |
| 9. Table pagination | L | 90 min | High |
| 14. CSV export | M | 40 min | Medium |
| 15. Verification | M | 30 min | Low |

**Total Estimated Time**: 6.5 hours
**High-risk items**: 2 (Error handling), 9 (Pagination) — test thoroughly

---

## Ultrawork Execution Notes

### Session 1 (2 hours): Phase 1 + Phase 2
- Items 12, 13, 6 (CSS foundation)
- Items 1, 5, 7, 11 (HTML structure)
- Commit after Phase 2

### Session 2 (2.5 hours): Phase 3
- Items 2, 3, 4 (Error handling, notifications, modals)
- Items 9, 14 (Pagination, CSV export)
- Commit after Phase 3

### Session 3 (1 hour): Phase 4
- Run all verification tests
- Fix regressions
- Final commit

### Break Points
- After Phase 1: File has new CSS, no JS changes yet — safe to pause
- After Phase 2: HTML structure complete, some JS hooks added — test basic functionality
- After Phase 3: All features implemented — full testing required

---

## Questions for User

1. **Pagination threshold**: Is 50 rows per page appropriate, or would you prefer 25/100?
2. **CSV export scope**: Should all tables be exportable, or only specific ones (pipeline, alerts)?
3. **Error handling severity**: For auto-refresh failures, should we show a persistent banner instead of toasts after 3 consecutive errors?
4. **Skeleton detail level**: Should skeletons match the exact layout (e.g., table rows with columns) or just generic blocks?
5. **Mobile breakpoints**: The current breakpoint is 900px. Should we add a tablet-specific layout (768px-900px)?

---

## Appendix: Current CSS Variables Reference

```css
/* Lines 10-22 */
--bg: #0b0d11; --panel: #13161c; --border: #242a33; --text: #e8ecf1; --muted: #9aa5b8;
--accent: #4fc1ff; --up: #26a17b; --down: #d93a3a; --warn: #f5a623;
--primary: #1f6feb; --danger: #da3633;
--space-xs: 4px; --space-sm: 8px; --space-md: 14px; --space-lg: 20px; --space-xl: 32px;
--layer-1: #3b82f6; --layer-2: #8b5cf6; --layer-3: #10b981; --layer-4: #f59e0b; --layer-5: #ef4444; --layer-6: #6366f1;
--status-ok: #10b981; --status-warn: #f59e0b; --status-err: #ef4444; --status-unknown: #9ca3af;
```

## Appendix: Current notify() Usage

```javascript
// Line 898: getJSON error
notify(`載入失敗: ${url.split('/').pop()} - ${err.message}`, 'error');

// Line 909: postJSON error  
notify(`操作失敗: ${url.split('/').pop()} - ${err.message}`, 'error');

// Line 2535: Alert acknowledge success
notify('警報已確認', 'success');

// Line 2538: Alert acknowledge error
notify('確認失敗: ' + e.message, 'error');

// Line 2994: Industry detail placeholder
notify(`產業詳細分析功能開發中: ${industryId}`, 'info');
```

**Note**: All calls pass a `type` parameter, but the current implementation ignores it. This makes Item 3 straightforward — just implement the type handling.
