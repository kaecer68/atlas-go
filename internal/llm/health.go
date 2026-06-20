package llm

// Health returns a map of health status for every registered provider,
// keyed by the provider's identity. Returns a non-nil empty map when no
// providers are registered.
func (r *DefaultRouter) Health() map[Provider]HealthStatus {
	result := make(map[Provider]HealthStatus, len(r.providers))
	for _, impl := range r.providers {
		h := impl.Health()
		result[h.Provider] = h
	}
	return result
}
