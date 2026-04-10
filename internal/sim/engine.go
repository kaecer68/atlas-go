package sim

import (
	"math"
	"sort"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type Engine struct {
	constraints domain.SimulationConstraints
}

func NewEngine(constraints domain.SimulationConstraints) *Engine {
	return &Engine{constraints: constraints}
}

func (e *Engine) Run(regime domain.Regime, quotes []domain.Quote, recs []domain.Recommendation) domain.SimulationResult {
	cash := e.constraints.StartingCash
	orders := make([]domain.Order, 0)
	positions := make([]domain.Position, 0)
	quoteBySymbol := make(map[string]domain.Quote, len(quotes))

	for _, quote := range quotes {
		quoteBySymbol[quote.Symbol] = quote
	}

	sort.Slice(recs, func(i, j int) bool {
		if recs[i].Conviction != recs[j].Conviction {
			return recs[i].Conviction > recs[j].Conviction
		}
		if recs[i].Symbol != recs[j].Symbol {
			return recs[i].Symbol < recs[j].Symbol
		}
		if recs[i].Agent != recs[j].Agent {
			return recs[i].Agent < recs[j].Agent
		}
		return recs[i].Reason < recs[j].Reason
	})

	maxDeployableCash := cash * (1 - e.constraints.ReserveCashFraction)
	maxPerPosition := maxDeployableCash * e.constraints.MaxPositionWeight

	for _, rec := range recs {
		if len(positions) >= e.constraints.MaxOpenPositions {
			break
		}
		if rec.Side != domain.SideBuy {
			continue
		}
		if rec.Conviction < e.constraints.MinRecommendationConviction {
			continue
		}

		quote, ok := quoteBySymbol[rec.Symbol]
		if !ok || !quote.IsTradable || quote.Volume < e.constraints.MinTradableVolume {
			continue
		}

		price := applyBPS(quote.Last, e.constraints.SlippageBPS+e.constraints.TransactionCostBPS)
		quantity := int(math.Floor(maxPerPosition/price/100.0) * 100)
		if quantity <= 0 {
			continue
		}

		cost := float64(quantity) * price
		if cost > cash {
			continue
		}

		cash -= cost
		orders = append(orders, domain.Order{
			Symbol:   rec.Symbol,
			Side:     domain.SideBuy,
			Quantity: quantity,
			Price:    price,
			Reason:   rec.Reason,
		})
		positions = append(positions, domain.Position{
			Symbol:        rec.Symbol,
			Quantity:      quantity,
			AverageCost:   price,
			CurrentPrice:  quote.Last,
			MarketValue:   float64(quantity) * quote.Last,
			UnrealizedPnL: float64(quantity) * (quote.Last - price),
		})
	}

	return domain.SimulationResult{
		Regime:     regime,
		Orders:     orders,
		Positions:  positions,
		EndingCash: cash,
	}
}

func applyBPS(price, bps float64) float64 {
	return price * (1 + bps/10000.0)
}
