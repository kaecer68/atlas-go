// Package eval provides model evaluation metrics and interpretability tools.
//
// Key symbols:
//   - OOSR2, SharpeRatio, CumulativeReturn, MaxDrawdown — out-of-sample metrics
//   - PermutationImportance — feature importance via permutation (SK-13)
//   - PartialDependence — partial dependence plots (SK-14)
//   - FriedmanH — pairwise factor interaction detection via H-statistic (SK-15)
//   - CheckSLRLAlignment — detects supervised/reinforcement learning reward mismatch (SK-28)
//
// Pitfall: new metrics MUST align with Fin-Skills specification numbering (SK-12~15, SK-28).
//
// Maturity: evolving
package eval
