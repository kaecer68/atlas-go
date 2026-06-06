package narrative

import (
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func TestMarketNarrativeDataFromSnapshot_AllFields(t *testing.T) {
	snap := marketdata.MacroDataSnapshot{
		US10Y:       marketdata.MacroDataPoint{Symbol: "US10Y", Value: 4.5, ChangePct: 0.10},
		DXY:         marketdata.MacroDataPoint{Symbol: "DXY", ChangePct: 0.50},
		VIX:         marketdata.MacroDataPoint{Symbol: "VIX", Value: 22.0},
		USD_TWD:     marketdata.MacroDataPoint{Symbol: "USDTWD", ChangePct: -0.30},
		Oil:         marketdata.MacroDataPoint{Symbol: "CL=F", ChangePct: 1.20},
		Gold:        marketdata.MacroDataPoint{Symbol: "GC=F", ChangePct: 0.80},
		JPY:         marketdata.MacroDataPoint{Symbol: "USDJPY=X", ChangePct: -0.50},
		TSMCRevenue: marketdata.MacroDataPoint{Symbol: "TSMC", ChangePct: 45.0},
		// Extended fields
		CPIYoY:              marketdata.MacroDataPoint{Symbol: "CPI", Value: 3.5},
		Bdi:                 marketdata.MacroDataPoint{Symbol: "BDI", ChangePct: 12.0},
		Copper:              marketdata.MacroDataPoint{Symbol: "HG=F", ChangePct: -4.0},
		ExportElectronics:   marketdata.MacroDataPoint{Symbol: "TWEXPORT", ChangePct: 7.0},
		SOXIndex:            marketdata.MacroDataPoint{Symbol: "SOX", ChangePct: 6.0},
		DRAMSpotPrice:       marketdata.MacroDataPoint{Symbol: "DRAM", ChangePct: -8.0},
		ForeignInvestorNet:  marketdata.MacroDataPoint{Symbol: "TWFLOW", Value: -80.0},
		RetailMarginBalance: marketdata.MacroDataPoint{Symbol: "TWMARGIN", ChangePct: 5.0},
	}

	data := MarketNarrativeDataFromSnapshot(snap)

	// US10YChangeBps ≈ ChangePct * Value = 0.10 * 4.5 = 0.45
	if math.Abs(data.US10YChangeBps-0.45) > 1e-9 {
		t.Errorf("US10YChangeBps: expected 0.45, got %v", data.US10YChangeBps)
	}
	if data.DXYChangePct != 0.50 {
		t.Errorf("DXYChangePct: expected 0.50, got %v", data.DXYChangePct)
	}
	if data.VIXLevel != 22.0 {
		t.Errorf("VIXLevel: expected 22.0, got %v", data.VIXLevel)
	}
	if data.USD_TWD_ChangePct != -0.30 {
		t.Errorf("USD_TWD_ChangePct: expected -0.30, got %v", data.USD_TWD_ChangePct)
	}
	if data.OilChangePct != 1.20 {
		t.Errorf("OilChangePct: expected 1.20, got %v", data.OilChangePct)
	}
	if data.GoldChangePct != 0.80 {
		t.Errorf("GoldChangePct: expected 0.80, got %v", data.GoldChangePct)
	}
	if data.JPY_ChangePct != -0.50 {
		t.Errorf("JPY_ChangePct: expected -0.50, got %v", data.JPY_ChangePct)
	}
	// AICapexSentiment from TSMCRevenue.ChangePct=45.0
	expectedAISentiment := computeAICapexSentiment(45.0)
	if data.AICapexSentiment != expectedAISentiment {
		t.Errorf("AICapexSentiment: expected %v, got %v", expectedAISentiment, data.AICapexSentiment)
	}
	// Fields not available in snapshot default to 0
	if data.GeopoliticalGPR != 0 {
		t.Errorf("GeopoliticalGPR: expected 0, got %v", data.GeopoliticalGPR)
	}
	// RetailInstitutionalDivergence computed from ForeignInvestorNet + RetailMarginBalance
	// foreignSignal = -(-80)/50 = 1.6, retailSignal = 5/2.5 = 2.0, div = (2.0+1.6)/2 = 1.8
	if math.Abs(data.RetailInstitutionalDivergence-1.8) > 1e-9 {
		t.Errorf("RetailInstitutionalDivergence: expected 1.8, got %v", data.RetailInstitutionalDivergence)
	}
	// MarginZScore = 5.0/2.0 = 2.5
	if math.Abs(data.MarginZScore-2.5) > 1e-9 {
		t.Errorf("MarginZScore: expected 2.5, got %v", data.MarginZScore)
	}
	// Extended macro inputs
	if data.CPIYoY != 3.5 {
		t.Errorf("CPIYoY: expected 3.5, got %v", data.CPIYoY)
	}
	if data.BDIChangePct != 12.0 {
		t.Errorf("BDIChangePct: expected 12.0, got %v", data.BDIChangePct)
	}
	if data.CopperChangePct != -4.0 {
		t.Errorf("CopperChangePct: expected -4.0, got %v", data.CopperChangePct)
	}
	if data.ExportElectronicsChangePct != 7.0 {
		t.Errorf("ExportElectronicsChangePct: expected 7.0, got %v", data.ExportElectronicsChangePct)
	}
	if data.SOXIndexChangePct != 6.0 {
		t.Errorf("SOXIndexChangePct: expected 6.0, got %v", data.SOXIndexChangePct)
	}
	if data.DRAMSpotPriceChangePct != -8.0 {
		t.Errorf("DRAMSpotPriceChangePct: expected -8.0, got %v", data.DRAMSpotPriceChangePct)
	}
}

func TestMarketNarrativeDataFromSnapshot_EmptySnapshot(t *testing.T) {
	var snap marketdata.MacroDataSnapshot
	data := MarketNarrativeDataFromSnapshot(snap)

	if data.US10YChangeBps != 0 {
		t.Errorf("US10YChangeBps: expected 0 for empty snapshot, got %v", data.US10YChangeBps)
	}
	if data.AICapexSentiment != 0 {
		t.Errorf("AICapexSentiment: expected 0 for empty snapshot, got %v", data.AICapexSentiment)
	}
}

func TestMarketNarrativeDataFromSnapshot_NoTSMC(t *testing.T) {
	snap := marketdata.MacroDataSnapshot{
		DXY: marketdata.MacroDataPoint{Symbol: "DXY", ChangePct: 0.5},
	}
	data := MarketNarrativeDataFromSnapshot(snap)

	if data.AICapexSentiment != 0 {
		t.Errorf("AICapexSentiment: expected 0 when TSMCRevenue absent, got %v", data.AICapexSentiment)
	}
	if data.DXYChangePct != 0.5 {
		t.Errorf("DXYChangePct: expected 0.5, got %v", data.DXYChangePct)
	}
}
