package system

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
)

// SwaggerHandlers serves the Swagger UI and swagger.json spec.
type SwaggerHandlers struct {
	WorkDir string
}

// NewSwaggerHandlers creates a SwaggerHandlers with the given working directory.
func NewSwaggerHandlers(workDir string) *SwaggerHandlers {
	return &SwaggerHandlers{WorkDir: workDir}
}

func (h *SwaggerHandlers) RegisterRoutes(mux *http.ServeMux) {
	// Deprecated: dev-only Swagger UI; not for production traffic. See docs/operations/tier-boundary.md.
	mux.Handle("GET /api/docs", shared.GetRaw(h.HandleSwaggerUI))
	// Deprecated: dev-only OpenAPI spec; not for production traffic. See docs/operations/tier-boundary.md.
	mux.Handle("GET /api/docs/swagger.json", shared.GetRaw(h.HandleSwaggerJSON))
}

func (h *SwaggerHandlers) HandleSwaggerUI(w http.ResponseWriter, r *http.Request) (int, any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>Atlas-Go API Docs</title>
<link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
<div id="swagger-ui"></div>
<script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
<script>
SwaggerUIBundle({
  url: './docs/swagger.json',
  dom_id: '#swagger-ui',
  presets: [SwaggerUIBundle.presets.apis, SwaggerUIBundle.presets.standalone]
});
</script>
</body>
</html>`))
	return 0, nil
}

func (h *SwaggerHandlers) HandleSwaggerJSON(w http.ResponseWriter, r *http.Request) (int, any) {
	data, err := os.ReadFile(filepath.Join(h.WorkDir, "docs/swagger.json"))
	if err != nil {
		return http.StatusNotFound, map[string]string{"error": "swagger spec not found"}
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
	return 0, nil
}
