package constants

// Paths used by atlas-go binaries and internal modules to locate configuration
// and data files relative to the repository root. When changing a value here,
// update any scripts, documentation, or environment variable defaults that
// reference the same path.

const (
	// ParametersFile is the canonical filename for the JSON parameter
	// configuration tree (used by config.LoadParametersConfig, calibration
	// commands, monitoring dashboards, and experiment executors).
	// The path is relative to the repository root; callers that need an
	// absolute path should prepend cfg.WorkDir or use filepath.Join.
	ParametersFile = "configs/parameters.json"
)
