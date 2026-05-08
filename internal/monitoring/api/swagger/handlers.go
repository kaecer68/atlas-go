package swagger

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
)

type Handlers struct {
	WorkDir string
}

func NewHandlers(workDir string) *Handlers {
	return &Handlers{WorkDir: workDir}
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/docs", shared.GetRaw(h.HandleSwaggerUI))
	mux.Handle("GET /api/docs/swagger.json", shared.GetRaw(h.HandleSwaggerJSON))
}

func (h *Handlers) HandleSwaggerUI(w http.ResponseWriter, r *http.Request) (int, any) {
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

func (h *Handlers) HandleSwaggerJSON(w http.ResponseWriter, r *http.Request) (int, any) {
	data, err := os.ReadFile(filepath.Join(h.WorkDir, "docs/swagger.json"))
	if err != nil {
		return http.StatusNotFound, map[string]string{"error": "swagger spec not found"}
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
	return 0, nil
}
