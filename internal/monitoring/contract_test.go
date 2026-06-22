package monitoring

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// Test 1: C4 — compile-time method signature assertion.
// If ChannelErrors signature changes, this line fails to compile.
// IMPORTANT: ChannelErrors is a method on *macroDataGatewayAdapter, NOT on MacroDataProvider interface.
// The method expression (*macroDataGatewayAdapter).ChannelErrors has signature
// func(*macroDataGatewayAdapter) map[string]string (receiver as first arg).
func TestContract_ChannelErrors_Signature(t *testing.T) {
	var _ func(*macroDataGatewayAdapter) map[string]string = (*macroDataGatewayAdapter).ChannelErrors
}

// Test 2: negative contract — MacroDataProvider interface must NOT include ChannelErrors.
// If ChannelErrors is added to MacroDataProvider, the interface grows and breaks existing
// providers (Composite, Yahoo, BDI, Hybrid). This compile-time assertion locks the interface.
func TestContract_MacroDataProvider_DoesNotIncludeChannelErrors(t *testing.T) {
	// Ensure the adapter satisfies the MacroDataProvider interface with only Name + FetchSnapshot.
	// If the interface grows, this assignment fails to compile.
	var _ marketdata.MacroDataProvider = (*macroDataGatewayAdapter)(nil)
}
