package config

type NegParameters struct {
	UnreadFlag NegParam `json:"unread_flag"`
}

type NegParam struct {
	Value int `json:"value"`
}

type ParametersConfig struct {
	Neg NegParameters `json:"neg"`
}
