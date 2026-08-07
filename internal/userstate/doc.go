// Package userstate defines the per-user behavioral state entities that
// back product positioning §9 「追蹤 → 紀律」:
//
//	觀測 → 解讀 → 追蹤 → 紀律
//
// The investment dashboard today stops at 顯示 (§9 audit: client_web has no
// watchlist / read-marking / journal mechanisms — see
// .omo/audit/2026-08-06-gap3-capital-flow-to-action.md). This package is the
// data-model skeleton for the missing 追蹤/紀律 layers. It deliberately
// contains ONLY entity types — storage and HTTP APIs are follow-up PRs
// (Gap 3 R2-R4).
//
// Design principles:
//   - UserID is the subscription.User.ID (int64) — this package does NOT
//     redefine the account entity, only behavior state keyed by user.
//   - Entities are immutable DTOs; mutation lives in the future store layer.
//   - Signal keys follow the strategy_techniques signal naming (e.g.
//     "foreign-3day-inflow") so read-state can join with signal metadata.
//
// Maturity: evolving (skeleton — no production consumers yet; API may adjust
// once R2 storage lands)
package userstate
