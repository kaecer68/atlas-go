package swagger

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
)

// Handlers provides Swagger UI and spec endpoints.
type Handlers struct {
	WorkDir string
}

// NewHandlers creates a new swagger Handlers.
func NewHandlers(workDir string) *Handlers {
	return &Handlers{WorkDir: workDir}
}

// RegisterRoutes mounts swagger endpoints on the given mux.
func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/docs", h.HandleSwaggerUI)
	mux.HandleFunc("/api/docs/swagger.json", h.HandleSwaggerJSON)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// HandleSwaggerUI serves the Swagger UI HTML page.
func (h *Handlers) HandleSwaggerUI(w http.ResponseWriter, r *http.Request) {
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
}

// HandleSwaggerJSON serves the OpenAPI spec JSON file.
func (h *Handlers) HandleSwaggerJSON(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(filepath.Join(h.WorkDir, "docs/swagger.json"))
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "swagger spec not found")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}
