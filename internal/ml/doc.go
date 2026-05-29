// Maturity: evolving
// Package ml provides supervised learning models for financial factor analysis.
//
// This package implements models from the Fin-Skills specification:
//   - SK-05: OLS (Ordinary Least Squares) baseline linear model
//   - SK-06: ElasticNet regularized model with coordinate descent
//   - SK-08: PCR (Principal Component Regression) via SVD
//   - SK-09: PLS (Partial Least Squares) via NIPALS algorithm
//
// All models use gonum.org/v1/gonum/mat for matrix operations with no
// external ML/AI library dependencies.
package ml
