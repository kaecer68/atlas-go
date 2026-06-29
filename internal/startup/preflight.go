// Package startup provides one-shot startup-time checks for atlas components.
package startup

import (
	"fmt"

	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/portprobe"
)

// PortClaim names a component and the TCP address it intends to bind.
type PortClaim struct {
	Component string
	Addr      string
}

// Preflight checks that every claimed address is free or healthy. If any
// address is held by a foreign process, it returns an actionable error
// identifying the occupant and a kill command. Probe failures are logged as
// warnings and do not stop the startup sequence.
func Preflight(claims []PortClaim) error {
	for _, claim := range claims {
		if err := checkClaim(claim); err != nil {
			return err
		}
	}
	return nil
}

func checkClaim(claim PortClaim) error {
	state, occupant, err := portprobe.Probe(claim.Addr)
	if err != nil {
		logging.Warn("startup", "preflight_probe_failed",
			"component", claim.Component,
			"addr", claim.Addr,
			logging.Err(err))
		return nil
	}
	if state == portprobe.StateForeign {
		return fmt.Errorf("%s address %s is held by a foreign process (pid=%d cmd=%q); stop it with `kill %d`",
			claim.Component, claim.Addr, occupant.PID, occupant.Command, occupant.PID)
	}
	return nil
}
