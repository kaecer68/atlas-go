// Package reflexivity provides reflexive price dynamics engine — models
// the feedback loop where agent trading actions themselves move prices,
// which in turn affects future agent decisions.
//
// Reflexivity is a concept from George Soros: prices don't just reflect
// fundamentals, they actively shape them through participant behavior.
// This package provides the mathematical scaffolding for modeling this
// feedback loop in simulation.
//
// Maturity: experimental — the underlying model is under active
// development. Do not depend on this from stable modules.
package reflexivity
