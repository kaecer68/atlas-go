// Package startup provides one-shot startup-time checks for atlas components.
//
// Maturity: stable
package startup

import (
	"fmt"

	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/portprobe"
)

// PortClaim names a component and the TCP address it intends to bind.
//
// AllowZombieKill opts the claim into automatic cleanup of suspected
// fubon-proxy zombies (per Oracle 4th round verdict F10 / F10a, 2026-06-29):
//   - false: any foreign occupant triggers actionable error,
//     matches historic preflight semantics — caller's port (e.g., atlas-http)
//     must not be auto-killed under any circumstance.
//   - true:  if foreign occupant is recognised as a fubon-proxy zombie
//     (portprobe.IsFubonZombie matches command name) we automatically
//     KillOccupant + re-probe; non-zombie occupants still error out.
//
// Mirrors fubonproxy.Start() lines 187-242 so a SIGKILL / orphan scenario
// no longer wedges atlas startup behind a stuck :8081.
type PortClaim struct {
	Component       string
	Addr            string
	AllowZombieKill bool
}

// probeFn / killFn / isFubonZombieFn are package-level seams used by
// checkClaim so unit tests can drive deterministic behavior without a
// real subprocess listening on a TCP port. They default to the
// portprobe package implementations.
var (
	probeFn         = portprobe.Probe
	killFn          = portprobe.KillOccupant
	isFubonZombieFn = portprobe.IsFubonZombie
)

// Preflight checks that every claimed address is free or healthy. If any
// address is held by a foreign process, it returns an actionable error
// identifying the occupant and a kill command; for claims with
// AllowZombieKill=true a recognised fubon-proxy zombie is auto-killed
// before producing an error. Probe failures are logged as warnings and
// do not stop the startup sequence.
func Preflight(claims []PortClaim) error {
	for _, claim := range claims {
		if err := checkClaim(claim); err != nil {
			return err
		}
	}
	return nil
}

func checkClaim(claim PortClaim) error {
	state, occupant, err := probeFn(claim.Addr)
	if err != nil {
		logging.Warn("startup", "preflight_probe_failed",
			"component", claim.Component,
			"addr", claim.Addr,
			logging.Err(err))
		return nil
	}
	if state != portprobe.StateForeign {
		return nil
	}
	if !claim.AllowZombieKill {
		return actionableForeignError(claim, occupant)
	}
	if occupant.PID <= 0 || !isFubonZombieFn(occupant) {
		// AllowZombieKill only authorises reclaiming fubon-proxy zombies;
		// never auto-kill an unrecognised foreign process even when caller
		// opted in (mirrors fubonproxy.Start() lines 233-242).
		return actionableForeignError(claim, occupant)
	}
	logging.Warn("startup", "preflight_zombie_detected",
		"component", claim.Component,
		"pid", occupant.PID,
		"cmd", occupant.Command,
		"message", "auto-killing zombie subprocess holding "+claim.Addr,
	)
	if killErr := killFn(occupant.PID); killErr != nil {
		return fmt.Errorf("%s address %s held by zombie process (pid=%d cmd=%q); auto-kill failed: %v; stop it manually with `kill %d`",
			claim.Component, claim.Addr, occupant.PID, occupant.Command, killErr, occupant.PID)
	}
	logging.Info("startup", "preflight_zombie_killed",
		"component", claim.Component,
		"pid", occupant.PID,
		"message", "re-probing "+claim.Addr+" after zombie kill",
	)
	newState, _, probeErr := probeFn(claim.Addr)
	if probeErr != nil {
		// Re-probe unavailable (e.g., lsof missing). Fall through: downstream
		// fubonproxy.Start() still has its own zombie kill + bind-attempt
		// path so the supervisor chain is preserved.
		logging.Warn("startup", "preflight_reprobe_failed",
			"component", claim.Component,
			"addr", claim.Addr,
			logging.Err(probeErr))
		return nil
	}
	if newState == portprobe.StateFree {
		return nil
	}
	return fmt.Errorf("%s address %s still held after auto-kill; identify the current occupant with `lsof -nP -iTCP:%s -sTCP:LISTEN` and stop it",
		claim.Component, claim.Addr, claim.Addr)
}

// actionableForeignError formats the user-facing message when no auto-kill
// is applicable (no AllowZombieKill, or occupant is not a recognised zombie).
func actionableForeignError(claim PortClaim, occupant portprobe.Occupant) error {
	if occupant.PID > 0 {
		return fmt.Errorf("%s address %s is held by a foreign process (pid=%d cmd=%q); stop it with `kill %d` or change fubon-proxy port",
			claim.Component, claim.Addr, occupant.PID, occupant.Command, occupant.PID)
	}
	return fmt.Errorf("%s address %s is held by an unknown process; identify it with `lsof -nP -iTCP:%s -sTCP:LISTEN` and stop it",
		claim.Component, claim.Addr, claim.Addr)
}
