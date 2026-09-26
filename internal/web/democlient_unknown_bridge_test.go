package web

import (
	"encoding/json"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

func TestTheDemoNamesAnUnknownBridgeUntilItApplies(t *testing.T) {
	c := NewDemoClient()
	status := func() shared.FirewallStatus {
		t.Helper()
		resp, err := c.Send(shared.Command{Type: shared.CmdGetStatus})
		if err != nil || !resp.Success {
			t.Fatalf("status: %v %+v", err, resp)
		}
		var s shared.FirewallStatus
		if err := json.Unmarshal(resp.Data, &s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	if got := status().UnknownBridges; len(got) != 1 || got[0].Interface != "br-4f2a" {
		t.Fatalf("a fresh demo reports %+v, want br-4f2a", got)
	}
	if resp, err := c.Send(shared.Command{Type: shared.CmdApplyRules}); err != nil || !resp.Success {
		t.Fatalf("the demo refused the apply: %v %+v", err, resp)
	}
	if got := status().UnknownBridges; len(got) != 0 {
		t.Errorf("after an apply the demo still reports %+v", got)
	}
}
