package demo

import "github.com/kaecer68/atlas-go/tests/fixtures/inert-closure/internal/config"

// Consume 是 production consumer：它讀 read_flag，並呼叫有接線的 writer。
func Consume(cfg *config.ParametersConfig) float64 {
	SetWiredWriter(1)
	return cfg.Demo.ReadFlag.Value + float64(DemoState())
}
