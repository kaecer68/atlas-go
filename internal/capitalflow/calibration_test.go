package capitalflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
)

// ===========================================================================
// Issue #1941 — CalibrationStatus must have a reachable, human-gated path.
//
// Before the fix ComputeCapitalFlowAssessment hardcoded
// CalibrationStatus="calibrating", so EligibleForAutomation() could never
// open and /api/recommendations warned on every response forever.
// ===========================================================================

// calibrationForces builds one ForceScore per dimension with the requested
// sample count / availability.
func calibrationForces(samples int, available bool) []ForceScore {
	dims := []ForceName{
		ForceForeign, ForceFutures, ForceTSMADR,
		ForceInstitutional, ForceDealer, ForceGovernment, ForceRetail,
	}
	out := make([]ForceScore, 0, len(dims))
	for _, d := range dims {
		status := CalibrationCalibrating
		if samples >= CalibrationEligibleMinSamples {
			status = CalibrationEligible
		}
		out = append(out, ForceScore{
			Force:             d,
			ZScore:            1.5,
			Trend:             "bullish",
			DataAvailable:     available,
			SampleCount:       samples,
			CalibrationStatus: status,
			AsOfTradingDate:   "2026-09-24",
		})
	}
	return out
}

// TestDeriveCalibrationStatus_DefaultOverrideStaysCalibrating locks the
// default (pre-#1941) behavior: with the human gate closed the assessment is
// calibrating no matter how complete the rolling history is.
func TestDeriveCalibrationStatus_DefaultOverrideStaysCalibrating(t *testing.T) {
	got := DeriveCalibrationStatus(calibrationForces(60, true), false)
	if got != CalibrationCalibrating {
		t.Errorf("DeriveCalibrationStatus(override=false) = %q, want %q", got, CalibrationCalibrating)
	}
	if reason := CalibrationStatusReason(calibrationForces(60, true), got); reason != "" {
		t.Errorf("calibrating must not carry a degradation reason; got %q", reason)
	}
}

// TestDeriveCalibrationStatus_OverrideReachesEligible proves the path exists:
// gate open + every available dimension at the sample floor → eligible, which
// is the only status that opens EligibleForAutomation().
//
// The fixture marks each dimension eligible the way ForceExtractor.Score does,
// so the derivation sees the same shape production produces.
func TestDeriveCalibrationStatus_OverrideReachesEligible(t *testing.T) {
	forces := calibrationForces(CalibrationEligibleMinSamples, true)
	status := DeriveCalibrationStatus(forces, true)
	if status != CalibrationEligible {
		t.Fatalf("DeriveCalibrationStatus(override=true, samples=%d) = %q, want %q",
			CalibrationEligibleMinSamples, status, CalibrationEligible)
	}
	assessment := CapitalFlowAssessment{CalibrationStatus: status}
	if !assessment.EligibleForAutomation() {
		t.Error("EligibleForAutomation() must open once the derived status is eligible")
	}
	if reason := CalibrationStatusReason(forces, status); reason != "" {
		t.Errorf("eligible must not carry a degradation reason; got %q", reason)
	}
}

// TestDeriveCalibrationStatus_OverrideBelowFloorIsDegraded: gate open but a
// dimension's samples collapsed → degraded (gate stays closed), with a reason
// naming the dimension.
func TestDeriveCalibrationStatus_OverrideBelowFloorIsDegraded(t *testing.T) {
	forces := calibrationForces(CalibrationEligibleMinSamples, true)
	for i := range forces {
		if forces[i].Force == ForceGovernment {
			forces[i].SampleCount = 3
		}
	}
	status := DeriveCalibrationStatus(forces, true)
	if status != CalibrationDegraded {
		t.Fatalf("status = %q, want %q", status, CalibrationDegraded)
	}
	if (CapitalFlowAssessment{CalibrationStatus: status}).EligibleForAutomation() {
		t.Error("degraded must keep the automation gate closed (CF-INV-13)")
	}
	reason := CalibrationStatusReason(forces, status)
	if !strings.Contains(reason, string(ForceGovernment)) || !strings.Contains(reason, "30") {
		t.Errorf("degradation reason must name the below-floor dimension and the floor; got %q", reason)
	}
}

// TestDeriveCalibrationStatus_OverrideWithoutAvailableDataIsDegraded: nothing
// to calibrate at all is degraded, not eligible.
func TestDeriveCalibrationStatus_OverrideWithoutAvailableDataIsDegraded(t *testing.T) {
	status := DeriveCalibrationStatus(calibrationForces(0, false), true)
	if status != CalibrationDegraded {
		t.Fatalf("status = %q, want %q", status, CalibrationDegraded)
	}
	if reason := CalibrationStatusReason(calibrationForces(0, false), status); reason == "" {
		t.Error("degraded without data must explain itself")
	}
}

// TestDeriveCalibrationStatus_MissingDimensionDoesNotBlockEligible: a source
// that is simply unavailable is reported by its own layer (Available=false) and
// must not gate the assessment forever — only available dimensions count.
func TestDeriveCalibrationStatus_MissingDimensionDoesNotBlockEligible(t *testing.T) {
	forces := calibrationForces(CalibrationEligibleMinSamples, true)
	for i := range forces {
		if forces[i].Force == ForceTSMADR {
			forces[i].DataAvailable = false
			forces[i].SampleCount = 0
		}
	}
	if status := DeriveCalibrationStatus(forces, true); status != CalibrationEligible {
		t.Fatalf("status = %q, want %q (an unavailable dimension has its own layer gate)", status, CalibrationEligible)
	}
}

// TestComputeCapitalFlowAssessment_StatusFollowsConfigOverride exercises the
// full production wiring: ComputeCapitalFlowAssessment reads
// capitalflow.calibration_eligible_override through the config singleton
// (same pattern as the trend thresholds).
func TestComputeCapitalFlowAssessment_StatusFollowsConfigOverride(t *testing.T) {
	prevPath := config.GetParametersConfigPath()
	t.Cleanup(func() {
		config.SetParametersConfigPath(prevPath)
		config.ResetParametersConfig()
	})

	cfg := config.DefaultParametersConfig()
	cfg.Capitalflow.CalibrationEligibleOverride.Value = true
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	path := filepath.Join(t.TempDir(), "parameters.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	config.SetParametersConfigPath(path)
	config.ResetParametersConfig()
	defer config.ResetParametersConfig()

	if !config.GetCapitalflowCalibrationEligibleOverride() {
		t.Fatal("GetCapitalflowCalibrationEligibleOverride() = false; config did not load")
	}

	forces := calibrationForces(CalibrationEligibleMinSamples, true)
	got := ComputeCapitalFlowAssessment(forces)
	if got.CalibrationStatus != CalibrationEligible {
		t.Fatalf("CalibrationStatus = %q, want %q with the override config on", got.CalibrationStatus, CalibrationEligible)
	}
	if !got.EligibleForAutomation() {
		t.Error("EligibleForAutomation() must open once the human gate is set and samples are sufficient")
	}
	if len(got.Reasons) != 0 {
		t.Errorf("eligible assessment must not carry degradation reasons; got %v", got.Reasons)
	}
}

// TestComputeCapitalFlowAssessment_DefaultConfigStaysCalibrating is the
// bit-identical gate for the default configuration: the assessment status and
// reasons must match the pre-#1941 hardcoded output exactly.
func TestComputeCapitalFlowAssessment_DefaultConfigStaysCalibrating(t *testing.T) {
	prevPath := config.GetParametersConfigPath()
	t.Cleanup(func() {
		config.SetParametersConfigPath(prevPath)
		config.ResetParametersConfig()
	})
	config.SetParametersConfigPath(filepath.Join(t.TempDir(), "does-not-exist.json"))
	config.ResetParametersConfig()

	if config.GetCapitalflowCalibrationEligibleOverride() {
		t.Fatal("unloaded config must default the override to false")
	}

	got := ComputeCapitalFlowAssessment(calibrationForces(calibrationEligibleSamplesProbe(), true))
	if got.CalibrationStatus != CalibrationCalibrating {
		t.Errorf("CalibrationStatus = %q, want calibrating by default", got.CalibrationStatus)
	}
	if got.Reasons != nil {
		t.Errorf("default assessment must carry no extra reasons (bit-identical); got %v", got.Reasons)
	}
	if got.EligibleForAutomation() {
		t.Error("default assessment must keep the automation gate closed (CF-INV-13)")
	}
}

// calibrationEligibleSamplesProbe keeps the default-path assertion honest by
// using a sample count far above the floor, so "calibrating" can only come from
// the closed human gate.
func calibrationEligibleSamplesProbe() int { return CalibrationEligibleMinSamples * 2 }

// TestDeriveCalibrationStatus_DimensionDegradedBlocksEligible is the issue #1940
// R3 interaction: a dimension whose reference window cannot standardize the
// value reports "degraded" whatever its sample count, and the overall
// assessment must not claim "eligible" on top of it.
func TestDeriveCalibrationStatus_DimensionDegradedBlocksEligible(t *testing.T) {
	forces := calibrationForces(CalibrationEligibleMinSamples*2, true)
	for i := range forces {
		if forces[i].Force == ForceForeign {
			forces[i].CalibrationStatus = CalibrationDegraded
		}
	}
	status := DeriveCalibrationStatus(forces, true)
	if status != CalibrationDegraded {
		t.Fatalf("status = %q, want %q when one available dimension is degraded", status, CalibrationDegraded)
	}
	if (CapitalFlowAssessment{CalibrationStatus: status}).EligibleForAutomation() {
		t.Error("a degraded dimension must keep the automation gate closed")
	}
	reason := CalibrationStatusReason(forces, status)
	if !strings.Contains(reason, string(ForceForeign)) || !strings.Contains(reason, CalibrationDegraded) {
		t.Errorf("reason must name the degraded dimension and its status; got %q", reason)
	}
}

// TestDeriveCalibrationStatus_DimensionBelowFloorWithStatusEmptyStillDegraded
// keeps the derivation robust for callers that build ForceScore by hand and
// leave CalibrationStatus empty (e.g. replay/validation paths): the sample
// floor alone still vetoes "eligible".
func TestDeriveCalibrationStatus_DimensionBelowFloorWithStatusEmptyStillDegraded(t *testing.T) {
	forces := calibrationForces(CalibrationEligibleMinSamples, true)
	for i := range forces {
		forces[i].CalibrationStatus = ""
		if forces[i].Force == ForceRetail {
			forces[i].SampleCount = CalibrationEligibleMinSamples - 1
		}
	}
	if status := DeriveCalibrationStatus(forces, true); status != CalibrationDegraded {
		t.Fatalf("status = %q, want %q", status, CalibrationDegraded)
	}
	if reason := CalibrationStatusReason(forces, CalibrationDegraded); !strings.Contains(reason, "retail=29") {
		t.Errorf("reason must include the dimension/sample pair; got %q", reason)
	}
}
