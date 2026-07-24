package client_web

import (
	"embed"
	_ "embed"
)

// DistFS embeds the built frontend assets.
//
//go:embed all:dist
var DistFS embed.FS

// ValidFieldsJSON embeds the generated frontend/backend field contract
// (produced by cmd/gentags). Served at GET /api/field-contract.
//
//go:embed static/js/shared/valid_fields.json
var ValidFieldsJSON []byte
