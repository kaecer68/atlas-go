package portfolio

import (
	"math"
	"sync"
)

type NormalizationMethod int

const (
	ZScore NormalizationMethod = iota
	Percentile
	MinMax
)

type agentStats struct {
	count  int
	mean   float64
	M      float64
	minVal float64
	maxVal float64
}

func (s *agentStats) variance() float64 {
	if s.count < 2 {
		return 0
	}
	return s.M / float64(s.count)
}

func (s *agentStats) stdDev() float64 {
	v := s.variance()
	if v <= 0 {
		return 0
	}
	return math.Sqrt(v)
}

type ConvictionNormalizer struct {
	mu     sync.RWMutex
	agents map[string]*agentStats
}

func NewConvictionNormalizer() *ConvictionNormalizer {
	return &ConvictionNormalizer{
		agents: make(map[string]*agentStats),
	}
}

func (cn *ConvictionNormalizer) RecordConviction(agentID string, conviction int) {
	cn.mu.Lock()
	defer cn.mu.Unlock()

	stats, ok := cn.agents[agentID]
	if !ok {
		stats = &agentStats{
			minVal: float64(conviction),
			maxVal: float64(conviction),
		}
		cn.agents[agentID] = stats
	}

	x := float64(conviction)
	stats.count++
	delta := x - stats.mean
	stats.mean += delta / float64(stats.count)
	delta2 := x - stats.mean
	stats.M += delta * delta2

	if x < stats.minVal {
		stats.minVal = x
	}
	if x > stats.maxVal {
		stats.maxVal = x
	}
}

func (cn *ConvictionNormalizer) Normalize(agentID string, conviction int, method NormalizationMethod) float64 {
	cn.mu.RLock()
	stats, ok := cn.agents[agentID]
	cn.mu.RUnlock()

	if !ok || stats.count < 2 {
		return float64(conviction)
	}

	x := float64(conviction)

	switch method {
	case ZScore:
		sd := stats.stdDev()
		if sd == 0 {
			return float64(conviction)
		}
		return (x - stats.mean) / sd
	case Percentile:
		z := stats.zScore(x)
		return cn.standardNormalCDF(z) * 100
	case MinMax:
		rangeVal := stats.maxVal - stats.minVal
		if rangeVal == 0 {
			return float64(conviction)
		}
		return (x - stats.minVal) / rangeVal
	default:
		return float64(conviction)
	}
}

func (s *agentStats) zScore(x float64) float64 {
	sd := s.stdDev()
	if sd == 0 {
		return 0
	}
	return (x - s.mean) / sd
}

func (cn *ConvictionNormalizer) standardNormalCDF(z float64) float64 {
	if z < -6 {
		return 0
	}
	if z > 6 {
		return 1
	}

	absZ := math.Abs(z)
	sqrt2pi := math.Sqrt2 * math.Sqrt(math.Pi)
	t := 1 / (1 + 0.2316419*absZ)
	poly := t * (0.319381530 + t*(-0.356563782+t*(1.781477937+t*(-1.821255978+t*1.330274429))))
	gaussian := math.Exp(-0.5*absZ*absZ) / sqrt2pi
	cdf := 1 - gaussian*poly

	if z < 0 {
		cdf = 1 - cdf
	}
	return cdf
}

func (cn *ConvictionNormalizer) GetStats(agentID string) (count int, mean, stdDev, min, max float64) {
	cn.mu.RLock()
	defer cn.mu.RUnlock()

	stats, ok := cn.agents[agentID]
	if !ok {
		return 0, 0, 0, 0, 0
	}

	return stats.count, stats.mean, stats.stdDev(), stats.minVal, stats.maxVal
}
