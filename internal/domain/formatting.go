package domain

import (
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// FormatNTD formats a floating-point amount as a Taiwan Dollar string.
// Examples: FormatNTD(1234.56) → "NT$1,234.56", FormatNTD(-100) → "NT$-100.00", FormatNTD(0) → "NT$0.00"
func FormatNTD(amount float64) string {
	p := message.NewPrinter(language.English)
	return "NT$" + p.Sprintf("%.2f", amount)
}
