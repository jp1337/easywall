package core

import (
	"log/slog"
	"slices"
	"time"
)

// reconcileDockerBridges restores once more if the Docker bridges turn up after
// the boot restore has already run.
//
// easywall-core.service is After=network.target. Docker is not part of
// network.target, so on a container host the daemon can restore its rules
// before docker0 and any br-* exist. The coexistence rules are built from the
// addresses those interfaces carry, so they come out without them, and container
// traffic is dropped until somebody applies again — on a machine that looks,
// from the dashboard, entirely healthy.
//
// The obvious fix is a systemd drop-in with After=docker.service. It was not
// taken: that is a hard ordering on a unit which is usually not installed, and it
// would have to be conditional on docker.enabled, which lives in a TOML file
// systemd does not read. A unit that refuses to start because Docker is absent
// would be a worse bug than the one being fixed.
//
// So the daemon waits for what it expected instead. It does nothing at all
// unless Docker coexistence is switched on, it restores at most once, and it
// gives up after reconcileWait rather than polling for ever.
func (f *Firewall) reconcileDockerBridges(quit <-chan struct{}) {
	docker := f.cfg.NetworkSettings().Docker
	if !docker.Enabled || !docker.AllowBridgeNetworks {
		return
	}

	// Bridges that were already there at boot are already in the rules. Only
	// their appearance afterwards is worth a second apply.
	if len(f.getBootBridges()) > 0 {
		return
	}

	deadline := time.NewTimer(f.reconcileWait)
	defer deadline.Stop()
	tick := time.NewTicker(f.reconcilePoll)
	defer tick.Stop()

	for {
		select {
		case <-quit:
			return
		case <-deadline.C:
			slog.Info("docker coexistence is on and no bridge appeared; the rules "+
				"in force do not name one. If containers cannot be reached, apply "+
				"again once Docker is up",
				"waited", f.reconcileWait)
			return
		case <-tick.C:
			found := detectDockerBridges()
			if len(found) == 0 || slices.Equal(found, f.getBootBridges()) {
				continue
			}
			slog.Info("docker bridges appeared after startup; putting the rules back "+
				"so they name them", "bridges", found)
			f.setBootBridges(found)
			if err := f.RestoreCurrent(RestoreReasonBoot); err != nil {
				slog.Error("could not restore after the docker bridges appeared", "error", err)
			}
			return
		}
	}
}
