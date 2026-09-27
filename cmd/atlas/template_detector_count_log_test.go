package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/narrative"
)

// TestTemplateDetectorRouteLog_FollowsRegistryCount pins the fix for the stale
// "[TemplateDetector] registered ... (N detectors + scan store=...)" startup
// line: the message must report the size of the registry it was given, not a
// number written into the source. The old literal kept a fixed count after the
// registry had grown and leaked a false number into downstream audit output
// (atlas-wiki CI audit).
func TestTemplateDetectorRouteLog_FollowsRegistryCount(t *testing.T) {
	reg := narrative.NewDefaultDetectorRegistry()
	want := len(reg.List())

	msg := templateDetectorRouteLog(reg, true)
	if !strings.Contains(msg, fmt.Sprintf("(%d detectors", want)) {
		t.Fatalf("startup log %q does not report the live registry size %d", msg, want)
	}

	// An empty registry must move the message: proof the count is computed, not
	// interpolated from a constant.
	if empty := templateDetectorRouteLog(narrative.NewDetectorRegistry(), false); !strings.Contains(empty, "(0 detectors") {
		t.Fatalf("startup log with an empty registry = %q, want it to report 0 detectors", empty)
	}

	// The route summary must distinguish "scan store present" from absent, since
	// the store decides whether the scan endpoints are reachable.
	if !strings.Contains(templateDetectorRouteLog(reg, false), "scan store=false") {
		t.Fatalf("startup log %q does not report a missing scan store", templateDetectorRouteLog(reg, false))
	}

	// Emit the operator-visible line through the real logger so the test output
	// shows exactly what production prints.
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	log.Print(templateDetectorRouteLog(reg, true))
	t.Logf("startup log line: %s", strings.TrimSpace(buf.String()))
}
