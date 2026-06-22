package monitoring

import (
	"testing"
)

// Test 1: C4 — compile-time method signature assertion.
// If ChannelErrors signature changes, this line fails to compile.
// IMPORTANT: ChannelErrors is a method on *macroDataGatewayAdapter, NOT on MacroDataProvider interface.
// The method expression (*macroDataGatewayAdapter).ChannelErrors has signature
// func(*macroDataGatewayAdapter) map[string]string (receiver as first arg).
func TestContract_ChannelErrors_Signature(t *testing.T) {
	var _ func(*macroDataGatewayAdapter) map[string]string = (*macroDataGatewayAdapter).ChannelErrors
}
