// internal/monitoring/api/system/health_aggregate.go
//
// /api/health/aggregate endpoint — Stage 6 frontend visibility.
//
// 用途：前端單一 HTTP 呼叫即可取得 atlas-go 整體健康狀態（4 層聚合）。
// 取代原本需要 fan-out 呼叫 `/health` + `/ready` + `/api/dashboard/channel-health` +
// `/api/llm/health` 的浪費，方便 admin_web / client_web 在啟動時一次性判斷。
//
// 設計：
//   - 不破壞既有端點契約：/health、/ready、/api/dashboard/channel-health、
//     /api/llm/health 行為完全不變。本檔只新增聚合視圖。
//   - 每個 Tier 獨立計算 latency_ms（前端可用於觀察 atlas 健康隨時間變化）。
//   - 任何 Tier 失敗不會讓整體 endpoint 500；改為回 200 + `tiers.<key>.ok: false`，
//     這樣前端 health banner 才能在 atlas 運作但有 partial failure 時正確顯示。
//   - Auth-free：需同步至 cmd/atlas/main.go isPublicPath
//     與 internal/monitoring/api/shared/handler.go authFreeExactPaths。

package system

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/portprobe"
)

// aggregateResponse is the JSON body for /api/health/aggregate.
type aggregateResponse struct {
	Tiers   map[string]tierReport `json:"tiers"`
	Overall tierReport            `json:"overall"`
}

// tierReport describes one health-check tier.
//
// OK is the canonical boolean for "this tier is healthy enough to render the UI".
// Reason is set when !OK and is human-readable (zh-TW).
// LatencyMS is the wall-clock cost of computing this tier (informational only).
type tierReport struct {
	OK        bool   `json:"ok"`
	Reason    string `json:"reason,omitempty"`
	LatencyMS int64  `json:"latency_ms"`
	Details   any    `json:"details,omitempty"`
}

// RegisterAggregateRoute mounts GET /api/health/aggregate on the given mux.
// Kept separate from HealthHandlers.RegisterRoutes so callers can opt in
// (admin only — staging should not expose this externally without explicit intent).
func (h *HealthHandlers) RegisterAggregateRoute(mux *http.ServeMux) {
	mux.Handle("GET /api/health/aggregate", newAggregateHandler(h))
}

func newAggregateHandler(h *HealthHandlers) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		status, data := h.handleHealthAggregate(r)
		w.Header().Set("Content-Type", "application/json")
		// 加 X-Aggregate-Duration 方便前端對齊 latency
		w.Header().Set("X-Aggregate-Duration-MS", time.Since(started).String())
		writeAggregateJSON(w, status, data)
	})
}

func writeAggregateJSON(w http.ResponseWriter, status int, data any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// HandleHealthAggregate fans out to N tiers in-process. Each tier is wrapped
// in a recovery + latency timer so one failing tier cannot crash the endpoint.
//
//nolint:unparam // r is required by shared.Handler interface
func (h *HealthHandlers) handleHealthAggregate(r *http.Request) (int, any) {
	resp := aggregateResponse{
		Tiers: map[string]tierReport{
			"liveness":       h.runTier(h.checkLiveness),
			"channel_health": h.runTier(h.checkChannelHealth),
			"llm_ready":      h.runTier(h.checkLLMReady),
			"auth_posture":   h.runTier(h.checkAuthPosture),
		},
	}
	resp.Overall = summarizeOverall(resp.Tiers)
	return http.StatusOK, resp
}

// runTier measures wall-clock latency and catches panic per tier.
func (h *HealthHandlers) runTier(check func() (ok bool, reason string, details any)) tierReport {
	start := time.Now()
	defer func() {
		// 若 check panic 也不影響 caller；Recover 後 OK 為 false。
		_ = recover()
	}()
	ok, reason, details := check()
	return tierReport{
		OK:        ok,
		Reason:    reason,
		LatencyMS: time.Since(start).Milliseconds(),
		Details:   details,
	}
}

// ---- Tier 1: liveness（port probe of atlas_http + fubon_proxy）----

type portProbeDetail struct {
	AtlasHTTP  string `json:"atlas_http"`
	FubonProxy string `json:"fubon_proxy"`
}

func (h *HealthHandlers) checkLiveness() (bool, string, any) {
	apiAddr := h.APIAddr
	if apiAddr == "" {
		apiAddr = constants.AdminHTTPAddr
	}
	detail := portProbeDetail{AtlasHTTP: "healthy", FubonProxy: "free"}

	state, _, err := portprobe.Probe(apiAddr)
	if err != nil || state != portprobe.StateHealthy {
		detail.AtlasHTTP = "unhealthy"
		return false, "atlas_http port not healthy", detail
	}

	fubonAddr := h.FubonAddr
	if fubonAddr == "" {
		fubonAddr = constants.FubonProxyAddr
	}
	if state, occ, err := portprobe.Probe(fubonAddr); err != nil || state == portprobe.StateForeign {
		detail.FubonProxy = "foreign_or_error"
		// fubon-proxy 在大多數 staging 不啟動；不算 critical，只回 warning。
		_ = occ
	}
	return true, "", detail
}

// ---- Tier 2: channel_health (uses ChannelHealthStore) ----

type channelHealthDetail struct {
	Total    int `json:"total"`
	OK       int `json:"ok"`
	Warn     int `json:"warn"`
	Error    int `json:"error"`
	Degraded int `json:"degraded"`
	Inactive int `json:"inactive"`
	Other    int `json:"other"`
}

func (h *HealthHandlers) checkChannelHealth() (bool, string, any) {
	if h.ChannelHealth == nil {
		return true, "", channelHealthDetail{}
	}
	all := h.ChannelHealth.All()
	detail := channelHealthDetail{Total: len(all)}
	for _, rec := range all {
		switch rec.Status {
		case "ok":
			detail.OK++
		case "warn":
			detail.Warn++
		case "error":
			detail.Error++
		case "degraded":
			detail.Degraded++
		case "inactive":
			detail.Inactive++
		default:
			detail.Other++
		}
	}
	if detail.Error > 0 {
		return false, "some channels in error state", detail
	}
	return true, "", detail
}

// ---- Tier 3: llm_ready（檔案存在檢查，因為 router 在 cmd/atlas 注入；這層
//            只回報 LLM provider 是否配置，非實際健康呼叫 — 避免聚合 endpoint
//            依賴深層 wiring；真實健康請打 /api/llm/health）----

type llmReadyDetail struct {
	Note string `json:"note"`
}

func (h *HealthHandlers) checkLLMReady() (bool, string, any) {
	// /api/health/aggregate 不主動打 /api/llm/health（會在 production 拖慢首屏）。
	// 退化為「配置存在性」檢查。實際 provider 狀態請讀 /api/llm/health。
	apiKey := config.GetSecret("LLM_DEEPSEEK_API_KEY")
	if apiKey == "" {
		return false, "LLM_DEEPSEEK_API_KEY 未設定", llmReadyDetail{Note: "no_live_check"}
	}
	return true, "", llmReadyDetail{Note: "configured; call /api/llm/health for live status"}
}

// ---- Tier 4: auth_posture（顯示當前環境的 auth 模式，讓前端 banner
//            可以在 dev mode 顯示警告）----

type authPostureDetail struct {
	Status string `json:"status"`
}

func (h *HealthHandlers) checkAuthPosture() (bool, string, any) {
	apiKey := os.Getenv("ATLAS_API_KEY")
	env := os.Getenv("ATLAS_ENV")
	status := "dev_no_auth"
	if env != "" && apiKey != "" {
		if env == "production" {
			status = "production"
		} else {
			status = "authenticated"
		}
	}
	return status == "dev_no_auth" || status == "authenticated" || status == "production", "", authPostureDetail{Status: status}
}

// summarizeOverall 計算整體狀態：liveness 不 OK → 整體 fail；其他 tier warning 仍算 OK。
func summarizeOverall(tiers map[string]tierReport) tierReport {
	if t, ok := tiers["liveness"]; ok && !t.OK {
		return tierReport{OK: false, Reason: "liveness: " + t.Reason}
	}
	return tierReport{OK: true}
}
