package server

// autoDescOr returns the auto-generated description for tool name, or fallback
// if the tool is not in the auto-desc map (manual override or unknown).
func autoDescOr(name, fallback string) string {
	if d := autoDesc(name); d != nil && d.Description != "" {
		return d.Description
	}
	return fallback
}
