package marketdata

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// This file pins the guarantee issue #2014 is about: the daily ceiling bounds
// the SUM over every process that shares the state file, not each process on
// its own.
//
// Production shape being reproduced: one long-lived atlas-go container plus
// seven atlas-cron-* containers, all bind-mounting the same data/state
// directory, each with its own DailyQuotaTracker. Before #2014 each read the
// counter once at construction and then overwrote the file with its own count,
// so N processes could each spend the full ceiling while the file showed only
// whoever wrote last. Measured on the production host with three throwaway
// containers on the real share: 3 x 200 calls actually went out against a
// ceiling of 200.

const quotaHelperEnvVar = "ATLAS_QUOTA_HELPER_PROCESS"

// TestDailyQuotaHelperProcess is not a test: it is the body that
// TestDailyQuotaTracker_CrossProcess_SharedCeiling re-executes as a SEPARATE OS
// PROCESS (the test binary calls itself). Everything it needs arrives through
// the environment and the result is printed on stdout.
func TestDailyQuotaHelperProcess(t *testing.T) {
	if os.Getenv(quotaHelperEnvVar) != "1" {
		t.Skip("not a helper invocation")
	}

	dir := os.Getenv("ATLAS_QUOTA_HELPER_DIR")
	provider := os.Getenv("ATLAS_QUOTA_HELPER_PROVIDER")
	limit, err := strconv.Atoi(os.Getenv("ATLAS_QUOTA_HELPER_LIMIT"))
	if err != nil {
		fmt.Printf("HELPER_ERROR=limit: %v\n", err)
		return
	}
	attempts, err := strconv.Atoi(os.Getenv("ATLAS_QUOTA_HELPER_ATTEMPTS"))
	if err != nil {
		fmt.Printf("HELPER_ERROR=attempts: %v\n", err)
		return
	}

	// Barrier so every helper constructs its tracker at the same instant. This
	// is the production shape: several processes boot with their own view of one
	// shared quota day, minutes or hours apart, and then keep spending.
	goFile := os.Getenv("ATLAS_QUOTA_HELPER_GO")
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, statErr := os.Stat(goFile); statErr == nil {
			break
		}
		if time.Now().After(deadline) {
			fmt.Printf("HELPER_ERROR=barrier timeout\n")
			return
		}
		time.Sleep(2 * time.Millisecond)
	}

	tracker := NewDailyQuotaTracker(provider, dir, limit)
	granted := 0
	for range attempts {
		if tracker.AllowCall() {
			granted++
		}
	}
	fmt.Printf("HELPER_GRANTED=%d\n", granted)
}

// TestDailyQuotaTracker_CrossProcess_SharedCeiling runs the ceiling against
// real concurrent processes. With the pre-#2014 tracker it fails with
// processes x limit granted; with the locked read-modify-write it grants
// exactly the limit.
func TestDailyQuotaTracker_CrossProcess_SharedCeiling(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("advisory file locking (flock) is unavailable on windows; there the tracker fails closed")
	}
	const (
		limit    = 40
		procs    = 4
		attempts = 40
	)
	dir := t.TempDir()
	goFile := filepath.Join(dir, "go")
	provider := "crossproc"

	type child struct {
		cmd *exec.Cmd
		out *bytes.Buffer
	}
	children := make([]child, 0, procs)
	for i := range procs {
		var buf bytes.Buffer
		cmd := exec.Command(os.Args[0], "-test.run=^TestDailyQuotaHelperProcess$")
		cmd.Env = append(os.Environ(),
			quotaHelperEnvVar+"=1",
			"ATLAS_QUOTA_HELPER_DIR="+dir,
			"ATLAS_QUOTA_HELPER_PROVIDER="+provider,
			"ATLAS_QUOTA_HELPER_LIMIT="+strconv.Itoa(limit),
			"ATLAS_QUOTA_HELPER_ATTEMPTS="+strconv.Itoa(attempts),
			"ATLAS_QUOTA_HELPER_GO="+goFile,
		)
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		if err := cmd.Start(); err != nil {
			t.Fatalf("start helper %d: %v", i, err)
		}
		children = append(children, child{cmd: cmd, out: &buf})
	}

	if err := os.WriteFile(goFile, []byte("go"), 0o644); err != nil {
		t.Fatalf("release barrier: %v", err)
	}

	total := 0
	for i, c := range children {
		if err := c.cmd.Wait(); err != nil {
			t.Fatalf("helper %d failed: %v\n%s", i, err, c.out.String())
		}
		granted, err := helperGranted(c.out.String())
		if err != nil {
			t.Fatalf("helper %d: %v\noutput:\n%s", i, err, c.out.String())
		}
		total += granted
	}

	if total > limit {
		t.Fatalf("cross-process total granted = %d, want <= %d: %d processes each spent up to the per-process ceiling, so the shared ceiling is not enforced",
			total, limit, procs)
	}
	if total != limit {
		t.Fatalf("cross-process total granted = %d, want exactly %d (every attempt must be accounted for)", total, limit)
	}

	// The file must agree with reality: the whole point of the shared counter.
	final := NewDailyQuotaTracker(provider, dir, limit)
	if got := final.CallsToday(); got != limit {
		t.Errorf("CallsToday() after the run = %d, want %d", got, limit)
	}
	if got := final.Remaining(); got != 0 {
		t.Errorf("Remaining() after the run = %d, want 0", got)
	}
}

func helperGranted(out string) (int, error) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "HELPER_GRANTED="); ok {
			return strconv.Atoi(v)
		}
		if strings.HasPrefix(line, "HELPER_ERROR=") {
			return 0, fmt.Errorf("helper reported %s", line)
		}
	}
	return 0, fmt.Errorf("helper printed no HELPER_GRANTED line")
}
