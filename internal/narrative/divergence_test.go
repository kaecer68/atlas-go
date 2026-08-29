package narrative

import "testing"

func TestRetailDivergenceAndMarginZScore_SameDirectionUpReturnsZero(t *testing.T) {
	d := NewDivergenceDetector()
	seedDivergenceHistory(d, 60, 10, 100, 5, 50)

	divergence, marginZ := d.RetailDivergenceAndMarginZScore(6500, 3250)
	if divergence != 0 {
		t.Fatalf("expected divergence 0, got %v", divergence)
	}
	if marginZ == 0 {
		t.Fatalf("expected non-zero margin z-score with history, got %v", marginZ)
	}
}

func TestRetailDivergenceAndMarginZScore_MarginUpForeignDownPositive(t *testing.T) {
	d := NewDivergenceDetector()
	seedDivergenceHistory(d, 60, 10, 100, 5, 50)

	divergence, _ := d.RetailDivergenceAndMarginZScore(6500, -3250)
	if divergence <= 0 {
		t.Fatalf("expected positive divergence, got %v", divergence)
	}
}

func TestRetailDivergenceAndMarginZScore_MarginDownForeignUpNegative(t *testing.T) {
	d := NewDivergenceDetector()
	seedDivergenceHistory(d, 60, 10, 100, 5, 50)

	divergence, _ := d.RetailDivergenceAndMarginZScore(-6500, 3250)
	if divergence >= 0 {
		t.Fatalf("expected negative divergence, got %v", divergence)
	}
}

func TestRetailDivergenceAndMarginZScore_InsufficientHistoryReturnsZero(t *testing.T) {
	d := NewDivergenceDetector()
	seedDivergenceHistory(d, 9, 10, 100, 5, 50)

	divergence, marginZ := d.RetailDivergenceAndMarginZScore(6500, -3250)
	if divergence != 0 || marginZ != 0 {
		t.Fatalf("expected 0,0 for insufficient history, got %v,%v", divergence, marginZ)
	}
}

func TestRetailDivergenceAndMarginZScore_FlatMarketReturnsZero(t *testing.T) {
	d := NewDivergenceDetector()
	seedDivergenceHistory(d, 60, 10, 10, 10, 10)

	divergence, marginZ := d.RetailDivergenceAndMarginZScore(200, 200)
	if divergence != 0 || marginZ != 0 {
		t.Fatalf("expected 0,0 for flat market, got %v,%v", divergence, marginZ)
	}
}

func seedDivergenceHistory(d *DivergenceDetector, count int, marginStart, marginStep, foreignStart, foreignStep float64) {
	margin := marginStart
	foreign := foreignStart
	for i := range count {
		marginDelta := marginStep + float64(i%5)
		foreignDelta := foreignStep + float64((i+2)%5)
		if i%2 == 1 {
			marginDelta += 1
			foreignDelta += 2
		}
		prevMargin := margin
		prevForeign := foreign
		margin += marginDelta
		foreign += foreignDelta
		currMargin := margin
		currForeign := foreign
		d.Update(currMargin, prevMargin, currForeign, prevForeign)
	}
}
