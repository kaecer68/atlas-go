// Package bootstrap provides atlas-go system initialization: HTTP routing
// registration and dashboard endpoint wiring.
//
// bootstrap is called once during cmd/atlas startup after configuration load
// and apigateway initialization. It wires:
//   - HTTP routes for the monitoring DashboardAPI
//   - All /api/* handler registrations (dashboard, industry, narrative,
//     control, experiment, backtest)
//   - Static file serving for the embedded admin_web/ and client_web/ dist
//
// The HTTP server uses net/http (no third-party framework) to keep the
// dependency footprint minimal. Routes are registered in a single
// RegisterRoutes() call to avoid scattered registration.
//
// bootstrap does NOT start the server — cmd/atlas main() owns the
// http.Server lifecycle and graceful shutdown.
//
// Maturity: stable
package bootstrap
