package recommender

func devModeEnabled(h *Handler) bool {
	return h != nil && h.devMode
}
