package narrative

import "sort"

type AUCROCResult struct {
	AUC        float64
	ThresholdN int
}

func ComputeAUCROC(labels []bool, scores []float64) AUCROCResult {
	if len(labels) != len(scores) || len(labels) == 0 {
		return AUCROCResult{}
	}

	type pair struct {
		score float64
		label bool
	}
	pairs := make([]pair, len(labels))
	for i := range labels {
		pairs[i] = pair{scores[i], labels[i]}
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].score > pairs[j].score
	})

	var tp, fp, prevTP, prevFP float64
	var auc float64
	for i, p := range pairs {
		if p.label {
			tp++
		} else {
			fp++
		}
		if i == len(pairs)-1 || pairs[i+1].score != p.score {
			auc += (fp - prevFP) * (tp + prevTP) / 2
			prevTP, prevFP = tp, fp
		}
	}

	totalPos := tp
	totalNeg := fp
	if totalPos == 0 || totalNeg == 0 {
		return AUCROCResult{AUC: 0.5, ThresholdN: len(labels)}
	}
	return AUCROCResult{
		AUC:        auc / (totalPos * totalNeg),
		ThresholdN: len(labels),
	}
}
