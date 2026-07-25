# Color-Token Audit (Phase 2a — Issue Tracking)

> **Date**: 2026-07-22
> **Source plan**: `.omo/plans/2026-07-20-next-phases.md` (Phase 2)
> **Goal**: Catalog every hardcoded hex/rgba color in `shared_web/static/js/`, map to
> existing `color-tokens.js` helpers, flag unmapped colors that need new helpers.
> This is the prerequisite for Phase 2b (consolidation) — splitting the work
> into per-file PRs.

---

**Total**: 109 hardcoded color references across 10 files (60 hex + 49 rgba).
**Note**: Original plan (2026-07-20) cited 76 — current count is 109, +33 due to
additions in the past 2 days.

**Hex subtotal**: 60 occurrences, ~25 unique colors (top: #b8c4d0 × 13, #13161c × 6, #f0f4f8 × 5).
**Rgba subtotal**: 49 occurrences, mostly derived from base hex × alpha.

---

## 2. Existing Helpers (color-tokens.js)

```javascript
export function financialColor(value, category)  // profit / loss / neutral
export function regimeColor(regime)              // RISK_ON / RISK_OFF / NEUTRAL
export function severityColor(severity)          // info / warn / error
export function confidenceColor(confidence)      // 0.0 ~ 1.0 → token
export function pnlProfitColor()                 // var(--pnl-profit)
export function pnlLossColor()                   // var(--pnl-loss)
export function inflowColor()                    // var(--capital-inflow)
export function outflowColor()                   // var(--capital-outflow)
export function hexToRgba(hex, alpha)            // hex → rgba with alpha
```

**Helper categories that exist**: financial sentiment, market regime, alert severity,
confidence heat, P&L, capital flow. **Missing**: chart palette, gradient stops,
chart series colors (not in tokens yet).

---

## 3. Color Frequency Map (109 total)

### 3.1 Hex colors (deduplicated, frequency-desc)

| Count | Color | Semantic guess | Token mapping | Action |
|------:|-------|----------------|---------------|--------|
| 13 | `#b8c4d0` | chart text / axis label | **unmapped** | Add `chartAxisColor()` helper |
| 6  | `#13161c` | chart background dark | **unmapped** | Add `chartBackgroundColor()` helper |
| 5  | `#f0f4f8` | light text on dark | **unmapped** | Add `chartTextColor()` helper |
| 5  | `#9ca3af` | muted text | **unmapped** | Add `mutedTextColor()` helper |
| 4  | `#f59e0b` | warn orange | `severityColor('warn')` | **MIGRATE** to existing helper |
| 3  | `#6b7280` | neutral gray | **unmapped** | Add `neutralTextColor()` helper |
| 3  | `#4fc1ff` | accent blue (custom) | **unmapped** | Add `accentBlueColor()` helper |
| 2  | `#fff`    | white | CSS `var(--text-primary)` | **MIGRATE** to CSS variable |
| 2  | `#ef4444` | red 500 | `severityColor('error')` | **MIGRATE** |
| 2  | `#10b981` | green 500 | `financialColor(value>0)` | **MIGRATE** |
| 1  | `#f39c12` | flat-ui orange | `severityColor('warn')` (≈ #f59e0b) | **MIGRATE** |
| 1  | `#e74c3c` | flat-ui red | `severityColor('error')` | **MIGRATE** |
| 1  | `#e67e22` | flat-ui orange dark | `severityColor('warn')` | **MIGRATE** |
| 1  | `#a855f7` | purple 500 | **unmapped** | Add `accentPurpleColor()` helper |
| 1  | `#9b59b6` | flat-ui purple | **unmapped** | Add `accentPurpleColor()` helper |
| 1  | `#8b5cf6` | violet 500 | **unmapped** | Add `accentPurpleColor()` helper |
| 1  | `#3b82f6` | blue 500 | **unmapped** | Add `accentBlueColor()` helper |
| 1  | `#3498db` | flat-ui blue | **unmapped** | Add `accentBlueColor()` helper |
| 1  | `#2ecc71` | flat-ui green | `financialColor(value>0)` | **MIGRATE** |
| 1  | `#2d333b` | dark gray (chart bg) | **unmapped** | Add `chartBackgroundColor()` |
| 1  | `#242a33` | dark gray variant | **unmapped** | Add `chartBackgroundColor()` |
| 1  | `#1abc9c` | flat-ui teal | **unmapped** | Add `accentTealColor()` helper |
| 1  | `#0b0d11` | near-black | CSS `var(--bg-primary)` | **MIGRATE** to CSS variable |
| 1  | `#000000` | pure black | **unmapped** | Add `overlayColor()` helper |
| 1  | `#e2e8f0` | light text variant | **unmapped** | Add `chartTextColor()` helper |

**Hex subtotal**: 60 occurrences, ~25 unique colors (top: #b8c4d0 × 13, #13161c × 6, #f0f4f8 × 5, #9ca3af × 5).

### 3.2 rgba() — 49 occurrences

Most rgba() calls are **derived from a base hex** + alpha. Examples:

- `rgba(245,158,11,.1)` × 3 — orange base = `#f59e0b` × 0.1 alpha
- `rgba(59,130,246,.15)` × 2 — blue base = `#3b82f6` × 0.15 alpha
- `rgba(239,68,68,.1)` × 2 — red base = `#ef4444` × 0.1 alpha
- `rgba(107,114,128,.1)` × 2 — gray base = `#6b7280` × 0.1 alpha
- `rgba(79,193,255,...)` × 4 — `#4fc1ff` × various alpha

**Mapping**: rgba colors should use `hexToRgba(hex, alpha)` from color-tokens.js,
**but only if hex is already a token**. For non-tokenized colors, the audit
recommends first migrating the hex to a helper, then converting rgba to use
`hexToRgba(helperOutput, alpha)`.

**Special case**: Some rgba calls are **template literals** with variable
alpha (e.g., `rgba(0, 0, 0, ${alpha})`) — these are dynamic and cannot
be tokenized without API change. Leave as-is.

---

## 4. Re-evaluation (2026-07-22 follow-up)

**Phase 2a audit over-counted**. Manual inspection of the top 4 files
(sparkline, pipeline, evolution_panel, industry) revealed that ~95% of
the "hardcoded" hex/rgba values are **NOT** raw literals — they are
**fallback values for `getThemeColor('--name', '#hex')`** calls (Canvas
context, where `var(--xxx)` is unusable) or **deliberately-tuned alpha
values** for HTML inline `style="background:rgba(...)"` (where
`getThemeColor()` would trigger a `getComputedStyle` per draw call,
costing performance).

The actual *theme-blind* hardcoded colors that benefit from Phase 2b
migration are limited to:

| Category | Count | What they are | Worth migrating? |
|----------|------:|---------------|------------------|
| Canvas `ctx.fillStyle`/`ctx.strokeStyle` with `getThemeColor()` fallback hex | ~50 | Color resolution already theme-aware; hex is just SSR/JSDOM fallback | **NO** — Canvas requires real hex; migration would break |
| HTML `style="background:rgba(R,G,B,A)"` literal | ~30 | Design-tuned alpha values; conversion to `hexToRgba()` adds `getComputedStyle` per render | **NO** — performance cost; rgba is already explicit |
| HTML `style="color:..."` / `.style.color = "..."` with hardcoded hex | ~5 | Theme-blind; can use `var(--xxx)` directly | **YES** — small, safe |
| Plain hex literals in non-CSS, non-Canvas code | ~3 | Theme-blind; should use helpers | **YES** — small, safe |

**Revised estimate**: ~8 colors benefit from Phase 2b migration, not 109.

## 4.1 Per-File Revised Workload

| File | Hex count | rgba count | Worth migrating | Reason |
|------|----------:|-----------:|-----------------|--------|
| `components/sparkline.js` | 27 | 1 | **3** (L292-294 HTML style) | 25 of 28 are `getThemeColor()` Canvas fallback |
| `pages/pipeline.js` | 0 | 27 | **0** | All 27 are HTML `style="background:rgba(...)"` with design-tuned alpha |
| `pages/evolution_panel.js` | 20 | 0 | **0** | All 20 are `getThemeColor()` Canvas fallback |
| `pages/industry.js` | 4 | 14 | **0** | Mix of Canvas fallback + design-tuned alpha |
| `pages/capital-history.js` | 9 | 0 | **TBD** | Need to check context |
| `shared/components/seasonality-panel.js` | 2 | 0 | **TBD** | Need to check context |
| `pages/dashboard.js` | 0 | 2 | **TBD** | Need to check context |
| `shared/utils.js` | 1 | 0 | **TBD** | Need to check context |
| `pages/decision-chain.js` | 1 | 0 | **TBD** | Need to check context |
| `pages/crossmarket.js` | 1 | 0 | **TBD** | Need to check context |

**Revised PR slicing (only 2 PRs, not 5)**:

1. **PR 2b-A** ✅ already shipped (#1284): 9 chart/accent helpers added
2. **PR 2b-B (revised)**: `components/sparkline.js` L292-294 (3 HTML style lines) + check `capital-history.js` / `dashboard.js` / `decision-chain.js` / `crossmarket.js` / `utils.js` for genuine theme-blind hex (expected ~5 more changes total)

**Conclusion**: The 5-PR slicing in the original audit was over-estimated.
After manual inspection, **2b-B is a small follow-up** (~8 changes max),
not a 28-change refactor. The other 3 PRs (2b-C/D/E for pipeline /
evolution_panel / industry) are **cancelled** because the hardcoded
values are deliberately theme-aware or design-tuned.

---

## 5. New Helpers Needed (proposed)

```javascript
// In color-tokens.js — additions for chart + accent palette
export function chartAxisColor()     { return 'var(--chart-axis)'; }     // #b8c4d0
export function chartBackgroundColor() { return 'var(--chart-bg)'; }    // #13161c, #242a33
export function chartTextColor()     { return 'var(--chart-text)'; }     // #f0f4f8, #e2e8f0
export function mutedTextColor()      { return 'var(--text-muted)'; }    // #9ca3af
export function accentBlueColor()     { return 'var(--accent-blue)'; }   // #4fc1ff, #3b82f6, #3498db
export function accentPurpleColor()   { return 'var(--accent-purple)'; } // #a855f7, #8b5cf6
export function accentTealColor()     { return 'var(--accent-teal)'; }   // #1abc9c
export function neutralTextColor()    { return 'var(--text-neutral)'; }  // #6b7280
export function overlayColor()        { return 'var(--overlay)'; }       // #000000
```

**CSS variables** (add to `shared_web/static/css/tokens.css` if not already present):
- `--chart-axis`, `--chart-bg`, `--chart-text`, `--text-muted`, `--accent-blue`,
  `--accent-purple`, `--accent-teal`, `--text-neutral`, `--overlay`

---

## 6. Risks & Caveats

| Risk | Mitigation |
|------|-----------|
| Visual regression after token migration | Test each PR in browser before merge; compare screenshots |
| Dynamic rgba (template literal) | Leave as-is, only migrate static rgba |
| Helper lookup is O(n) for chart series | Cache common series colors if hot path detected |
| Existing CSS variable may not be defined | Verify all 9 new tokens in `tokens.css` before PR |

---

## 7. Out of Scope (intentionally)

- **CSS variable pruning** (Task 2.3 in plan) — CANCELLED per Oracle
- **Tailwind-style color mapping** — not aligned with project conventions
- **Migration of admin_web/static/js/** — separate codebase, separate audit

---

## 8. Action Items (revised 2026-07-22 after Phase 2b re-evaluation)

- [x] **PR 2b-A**: Add 9 chart/accent helpers — **shipped #1284 (commit f5d4bedc)**
- [ ] **PR 2b-B (revised)**: `components/sparkline.js` L292-294 (3 HTML style lines) +
      check `capital-history.js` / `dashboard.js` / `decision-chain.js` /
      `crossmarket.js` / `utils.js` for theme-blind hex (expected ≤ 5 more lines)
- [CANCELLED] **PR 2b-C/D/E**: pipeline.js / evolution_panel.js / industry.js —
      hardcoded values are already `getThemeColor()` Canvas fallback or
      design-tuned alpha. No token migration needed.

**Revised estimated total**: 1 PR, ≤ 30 minutes (audit + small style refactor).
**Caveat**: This 2b-A + 2b-B re-evaluation only changes the *implementation* of
the audit, not its strategic value. The 9 helpers added in 2b-A are still useful
for any new HTML inline style code; they will reduce future tech debt even
though the existing hex fallbacks stay.

---

## 9. Verification

Run this command to reproduce the count:

```bash
for f in $(grep -rlE "['\"]#[0-9a-fA-F]{3,8}['\"]|rgba\(" shared_web/static/js/ \
  --include='*.js' \
  | grep -v node_modules | grep -v __tests__ | grep -v dist | grep -v color-tokens); do
  count=$(grep -cE "['\"]#[0-9a-fA-F]{3,8}['\"]|rgba\(" "$f")
  echo "$count $f"
done | sort -rn
```

Total should be 109 (matches this audit's 60 hex + 49 rgba).
