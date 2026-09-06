package core

import (
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// An installation upgrading from 2.14 has no [usage] section at all. Unset has
// to mean the shipped default, not zero — an int field would have switched the
// ticker off on every existing installation, silently, and the column would
// have read "never" for ever on all of them.
func TestUsageInterval_UnsetIsTheDefault(t *testing.T) {
	c := &Config{}
	if got, want := c.UsageInterval(), shared.UsageIntervalDefault*time.Second; got != want {
		t.Errorf("UsageInterval() with no [usage] section = %v, want %v", got, want)
	}
}

// Zero is a choice somebody made, and it stops the ticker. apply() still
// collects — that call exists to keep the flush from destroying a number rather
// than to sample one — which is why this only says "the ticker".
func TestUsageInterval_ZeroIsOff(t *testing.T) {
	zero := 0
	c := &Config{}
	c.Usage.Interval = &zero
	if got := c.UsageInterval(); got != 0 {
		t.Errorf("UsageInterval() with interval = 0 is %v, want 0", got)
	}
}

func TestUsageInterval_ReadsTheConfiguredValue(t *testing.T) {
	sixty := 60
	c := &Config{}
	c.Usage.Interval = &sixty
	if got := c.UsageInterval(); got != 60*time.Second {
		t.Errorf("UsageInterval() = %v, want 1m", got)
	}
}

// A negative interval is a typo, not an instruction. Clamped to off with a
// warning rather than refused, the way an out-of-range acceptance duration is:
// a daemon that will not start because of one number in one optional section is
// a worse outcome than a ticker that does not run.
func TestValidate_ClampsANegativeUsageInterval(t *testing.T) {
	neg := -5
	c := newTestConfig(t)
	c.Usage.Interval = &neg
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate rejected a negative usage interval instead of clamping it: %v", err)
	}
	if got := c.UsageInterval(); got != 0 {
		t.Errorf("after Validate, UsageInterval() = %v, want 0", got)
	}
}
