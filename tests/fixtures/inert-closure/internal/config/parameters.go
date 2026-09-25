package config

// DemoParameters 是 fixture 用的 config 區塊（比照 internal/config/parameters.go 的形狀）。
type DemoParameters struct {
	ReadFlag  DemoParam `json:"read_flag"`
	InertFlag DemoParam `json:"inert_flag"`
}

type DemoParam struct {
	Value float64 `json:"value"`
}

type ParametersConfig struct {
	Demo DemoParameters `json:"demo"`
}
