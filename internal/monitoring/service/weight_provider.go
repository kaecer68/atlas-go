package service

type WeightProvider interface {
	GetWeights(regime string) map[string]float64
}
