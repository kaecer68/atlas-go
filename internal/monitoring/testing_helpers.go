package monitoring

import "context"

// NoopFetcher returns a DataFetcher that always returns empty JSON.
// Use in tests to avoid live API calls when constructing DashboardAPI.
func NoopFetcher() DataFetcher {
	return func(_ context.Context, _ string) ([]byte, error) {
		return []byte("{}"), nil
	}
}
