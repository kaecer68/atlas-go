// Package channelsecrets implements persistent, encrypted storage and hot
// reload for data-channel API keys (issue #1776 Phase 1).
//
// Maturity: experimental
//
// Provides the admin channel-keys API backing (/api/admin/channel-keys)
// with AES-256-GCM encrypted persistence (Postgres prod / SQLite dev) and
// runtime hot reload into the shared marketdata clients. Master key:
// ATLAS_SECRET_STORE_KEY.
package channelsecrets
