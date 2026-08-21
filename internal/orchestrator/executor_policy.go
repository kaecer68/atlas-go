package orchestrator

import (
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
)

func DefaultExecutionPolicy() domain.ExecutionPolicy {
	cfg := config.GetParametersConfig().Engine.Executors
	return domain.ExecutionPolicy{
		ConvictionFloor:               cfg.ConvictionFloorDefault.Value,
		RequireCROPass:                true,
		MomentumCrashProtection:       true,
		EnableConvictionNormalization: true,
	}
}
