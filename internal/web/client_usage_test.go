package web

import (
	"testing"
	"time"

	"github.com/jp1337/easywall/internal/shared"
)

// The public demo is where most people ever see this product. A feature that
// answers an empty map there does not exist as far as any visitor is concerned.
func TestDemoClient_GetUsageShowsAPortNobodyHasUsed(t *testing.T) {
	c := NewDemoClient()
	res, err := c.GetUsage()
	if err != nil {
		t.Fatalf("GetUsage: %v", err)
	}
	if len(res.Usage) == 0 {
		t.Fatal("the demo answers GET_USAGE with an empty map; the Last used column would " +
			"read the same on every row and the feature is invisible to every visitor")
	}
	if res.CollectedAt.IsZero() {
		t.Error("the demo reports no collection time")
	}

	state, err := c.GetRules()
	if err != nil {
		t.Fatal(err)
	}

	var stale, fresh int
	cutoff := time.Now().AddDate(0, 0, -30)
	for _, r := range state.Staged.TCP {
		u, known := res.Usage[r.ID]
		if !known || u.LastSeen.IsZero() {
			continue
		}
		if u.LastSeen.Before(cutoff) {
			stale++
		} else {
			fresh++
		}
	}
	if stale == 0 {
		t.Error("no demo port is unused for 30+ days; the dashboard's second line never appears")
	}
	if fresh == 0 {
		t.Error("every demo port is stale; the column shows one state and reads as broken")
	}
}

// Every rule the demo answers with has an id, or the column reads em dash on
// every row of the public demo.
func TestDemoClient_EveryPortRuleHasAnID(t *testing.T) {
	c := NewDemoClient()
	state, err := c.GetRules()
	if err != nil {
		t.Fatal(err)
	}
	for _, set := range []shared.Rules{state.Current, state.Staged} {
		for _, list := range [][]shared.PortRule{set.TCP, set.UDP} {
			for _, r := range list {
				if r.ID == "" {
					t.Errorf("demo rule %q has no id", r.Port)
				}
			}
		}
	}
}

// And a save through the demo assigns one, the way the core's SaveStaged does.
// The demo represents the product to everyone who has not installed it.
func TestDemoClient_SaveRulesAssignsAnID(t *testing.T) {
	c := NewDemoClient()
	if err := c.SaveRules("tcp", []shared.PortRule{{Port: "9000", Description: "new"}}); err != nil {
		t.Fatal(err)
	}
	state, _ := c.GetRules()
	if state.Staged.TCP[0].ID == "" {
		t.Error("a rule saved through the demo has no id")
	}
}
