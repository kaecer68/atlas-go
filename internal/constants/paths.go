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

	// State directory and subpaths under data/state/.
	// All paths are relative to cfg.WorkDir.
	StateDir = "data/state"

	// Top state subpaths by occurrence (sorted desc):
	StateMacro              = StateDir + "/macro"               // macro snapshot cache + TWSECapitalFlow
	StateGeopolitical       = StateDir + "/geopolitical"        // geopolitical event cache
	StateCapitalFlow        = StateDir + "/capital_flow"        // TWSE capital flow daily JSON
	StateExperiments        = StateDir + "/experiments"         // experiment run artifacts
	StateExport             = StateDir + "/export"              // export statistics cache
	StateChannels           = StateDir + "/channels"            // data quality channel health
	StateSessions           = StateDir + "/sessions"            // session snapshots (OOS outcomes)
	StateParameterSnapshots = StateDir + "/parameter-snapshots" // parameter calibration snapshots

	// Other configuration and data paths:
	AgentsConfigPath    = "configs/agents.json"                // agent registry
	ReplayCSVPath       = "data/replay/tw_extended_90days.csv" // default replay CSV universe
	FubonDMAScriptPath  = "cmd/fubon-dma/wrapper.py"           // fubon DMA wrapper script
	StateLive           = StateDir + "/live"                   // live trading state
	StateBaselinePolicy = StateDir + "/baseline_policy"        // baseline policy artifacts
	StateMargin         = StateDir + "/margin"                 // margin balance cache
	StateCalibration    = StateDir + "/calibration"            // calibration baseline artifacts
)
