package narrative

import "fmt"

type TaxonomyL1 string

const (
	TaxonomyL1Uncategorized   TaxonomyL1 = "uncategorized"
	TaxonomyL1MacroEconomic   TaxonomyL1 = "macro_economic"
	TaxonomyL1MarketStructure TaxonomyL1 = "market_structure"
	TaxonomyL1SectorDynamics  TaxonomyL1 = "sector_dynamics"
	TaxonomyL1CompanySpecific TaxonomyL1 = "company_specific"
)

type TaxonomyL2 string

const (
	TaxonomyL2Uncategorized       TaxonomyL2 = "uncategorized"
	TaxonomyL2CentralBankPolicy   TaxonomyL2 = "central_bank_policy"
	TaxonomyL2InflationData       TaxonomyL2 = "inflation_data"
	TaxonomyL2Geopolitical        TaxonomyL2 = "geopolitical"
	TaxonomyL2TradeBalance        TaxonomyL2 = "trade_balance"
	TaxonomyL2Employment          TaxonomyL2 = "employment"
	TaxonomyL2RegimeShift         TaxonomyL2 = "regime_shift"
	TaxonomyL2LiquidityEvent      TaxonomyL2 = "liquidity_event"
	TaxonomyL2InstitutionalFlow   TaxonomyL2 = "institutional_flow"
	TaxonomyL2MarginCall          TaxonomyL2 = "margin_call"
	TaxonomyL2VolatilitySpike     TaxonomyL2 = "volatility_spike"
	TaxonomyL2SectorRotation      TaxonomyL2 = "sector_rotation"
	TaxonomyL2PolicyIndustry      TaxonomyL2 = "policy_industry_specific"
	TaxonomyL2GlobalSupplyChain   TaxonomyL2 = "global_supply_chain"
	TaxonomyL2CommodityPrice      TaxonomyL2 = "commodity_price"
	TaxonomyL2CompetitiveShock    TaxonomyL2 = "competitive_shock"
	TaxonomyL2EarningsRelease     TaxonomyL2 = "earnings_release"
	TaxonomyL2GuidanceUpdate      TaxonomyL2 = "guidance_update"
	TaxonomyL2CapitalAction       TaxonomyL2 = "capital_action"
	TaxonomyL2CorporateGovernance TaxonomyL2 = "corporate_governance"
	TaxonomyL2Incident            TaxonomyL2 = "incident"
)

var taxonomyL2Valid = map[TaxonomyL1]map[TaxonomyL2]bool{
	TaxonomyL1MacroEconomic: {
		TaxonomyL2CentralBankPolicy: true, TaxonomyL2InflationData: true,
		TaxonomyL2Geopolitical: true, TaxonomyL2TradeBalance: true, TaxonomyL2Employment: true,
	},
	TaxonomyL1MarketStructure: {
		TaxonomyL2RegimeShift: true, TaxonomyL2LiquidityEvent: true,
		TaxonomyL2InstitutionalFlow: true, TaxonomyL2MarginCall: true, TaxonomyL2VolatilitySpike: true,
	},
	TaxonomyL1SectorDynamics: {
		TaxonomyL2SectorRotation: true, TaxonomyL2PolicyIndustry: true,
		TaxonomyL2GlobalSupplyChain: true, TaxonomyL2CommodityPrice: true, TaxonomyL2CompetitiveShock: true,
	},
	TaxonomyL1CompanySpecific: {
		TaxonomyL2EarningsRelease: true, TaxonomyL2GuidanceUpdate: true,
		TaxonomyL2CapitalAction: true, TaxonomyL2CorporateGovernance: true, TaxonomyL2Incident: true,
	},
}

func IsValidL1(l1 TaxonomyL1) bool {
	_, ok := taxonomyL2Valid[l1]
	return ok
}

func ValidateTaxonomy(l1 TaxonomyL1, l2 TaxonomyL2) error {
	if l1 == TaxonomyL1Uncategorized {
		return nil
	}
	if !IsValidL1(l1) {
		return fmt.Errorf("narrative taxonomy: unknown L1 %q", l1)
	}
	if l2 == TaxonomyL2Uncategorized {
		return nil
	}
	if !taxonomyL2Valid[l1][l2] {
		return fmt.Errorf("narrative taxonomy: L2 %q does not belong to L1 %q", l2, l1)
	}
	return nil
}
