package recommender

import "os"

// devModeEnabled reports whether the dev-mode header fallbacks are active.
// Only true when ATLAS_DEV_MODE=true (or "1"). Production defaults to false,
// so X-User-Email spoofing attempts return 401 instead of upgrading tier.
func devModeEnabled() bool {
	v := os.Getenv("ATLAS_DEV_MODE")
	return v == "true" || v == "1"
}