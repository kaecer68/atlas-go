// Package sectorallocation provides the single authoritative source for
// industry/sector weight computation in atlas-go.
//
// This module unifies weight calculation that was previously scattered across
// three independent modules (industry, portfolio, monitoring) into one
// multi-factor engine.
//
// Weight Formula:
//
//	adjusted = baseWeight × cycleMultiplier × seasonalMultiplier ×
//	          linkageMultiplier × narrativeMultiplier × macroTilt × factorTilt
//
// Maturity: evolving
package sectorallocation
