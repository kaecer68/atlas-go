package macroflow

// rule holds a single macro→weight adjustment rule and its justification.
type rule struct {
	Adjustment Adjustment
	Reason     string
}

// evaluateRules returns applicable rules for the given risk level + stress flag.
func evaluateRules(level RiskLevel, stress bool) []rule {
	switch level {
	case RiskYellow:
		if stress {
			return yellowStressRules()
		}
		return yellowRules()
	case RiskOrange:
		if stress {
			return orangeStressRules()
		}
		return orangeRules()
	case RiskRed:
		if stress {
			return redStressRules()
		}
		return redRules()
	default:
		return nil
	}
}

// combineAdjustments sums multiple rule adjustments into one.
func combineAdjustments(rules []rule) Adjustment {
	var total Adjustment
	for _, r := range rules {
		total.Defensive += r.Adjustment.Defensive
		total.Aggressive += r.Adjustment.Aggressive
		total.Cash += r.Adjustment.Cash
	}
	total, _ = clipAndDedupe(total, nil)
	return total
}

// --- Yellow tier (elevated uncertainty) ---

func yellowRules() []rule {
	return []rule{
		{
			Adjustment: Adjustment{Defensive: 5, Aggressive: 0, Cash: 0},
			Reason:     "yellow: defensiva +5% — increased uncertainty justifies moderate shift to defensive",
		},
	}
}

func yellowStressRules() []rule {
	return []rule{
		{
			Adjustment: Adjustment{Defensive: 20, Aggressive: -15, Cash: 0},
			Reason:     "yellow+stress: defensiva +20%, aggressive -15% — elevated uncertainty with market stress",
		},
	}
}

// --- Orange tier (high risk) ---

func orangeRules() []rule {
	return []rule{
		{
			Adjustment: Adjustment{Defensive: 15, Aggressive: -10, Cash: 0},
			Reason:     "orange: defensiva +15%, aggressive -10% — high risk regime",
		},
	}
}

func orangeStressRules() []rule {
	return []rule{
		{
			Adjustment: Adjustment{Defensive: 20, Aggressive: -20, Cash: 5},
			Reason:     "orange+stress: defensiva +20%, aggressive -20%, cash +5% — high risk with market stress",
		},
	}
}

// --- Red tier (severe/crisis) ---

func redRules() []rule {
	return []rule{
		{
			Adjustment: Adjustment{Defensive: 20, Aggressive: -25, Cash: 10},
			Reason:     "red: defensiva +20%, aggressive -25%, cash +10% — crisis regime",
		},
	}
}

func redStressRules() []rule {
	return []rule{
		{
			Adjustment: Adjustment{Defensive: 25, Aggressive: -30, Cash: 15},
			Reason:     "red+stress: defensiva +25%, aggressive -30%, cash +15% — crisis with extreme market stress",
		},
	}
}
