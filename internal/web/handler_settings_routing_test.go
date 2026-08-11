package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// sentSettings returns the NetworkSettings the web process actually put on the
// socket. What the page said it saved is not the question here.
func sentSettings(t *testing.T, fc *fakeCore, form string) shared.NetworkSettings {
	t.Helper()
	var got shared.NetworkSettings
	fc.OnCommand(shared.CmdSaveSettings, func(cmd shared.Command) {
		if err := json.Unmarshal(cmd.Payload, &got); err != nil {
			t.Errorf("payload does not decode: %v", err)
		}
	})
	fc.SetResponse(shared.CmdSaveSettings, shared.Response{Success: true})

	s := newTestServer(t, fc)
	rec := doAuthFormRequest(t, s, "/settings", form)
	assertRedirect(t, rec, "/settings")
	return got
}

func TestSettingsPOST_CarriesTheRoutingMode(t *testing.T) {
	for _, mode := range []shared.RoutingMode{shared.RoutingClosed, shared.RoutingNetworks, shared.RoutingOpen} {
		got := sentSettings(t, newFakeCore(t),
			"routing_mode="+string(mode)+"&routing_networks=10.8.0.0%2F24")
		if got.Routing.Mode != mode {
			t.Errorf("routing.mode reached the core as %q, want %q", got.Routing.Mode, mode)
		}
		if len(got.Routing.Networks) != 1 || got.Routing.Networks[0] != "10.8.0.0/24" {
			t.Errorf("routing.networks reached the core as %v", got.Routing.Networks)
		}
	}
}

// An unrecognised value has to land on closed, never on open.
//
// "open" is the one position that stops easywall having an opinion about routed
// traffic, and a form value nobody sent — a stale page, a hand-made request —
// must not be able to reach it.
func TestSettingsPOST_UnknownRoutingModeBecomesClosed(t *testing.T) {
	got := sentSettings(t, newFakeCore(t), "routing_mode=sometimes")
	if got.Routing.Mode != shared.RoutingClosed {
		t.Errorf("an unknown routing mode became %q, want %q", got.Routing.Mode, shared.RoutingClosed)
	}
}

// The routing list is checked in the browser's own request, like the Docker one.
// The core would refuse it too, but a generic "save failed" leaves the operator
// hunting for which line.
func TestSettingsPOST_RefusesARoutingNetworkThatIsNotOne(t *testing.T) {
	fc := newFakeCore(t)
	reached := false
	fc.OnCommand(shared.CmdSaveSettings, func(shared.Command) { reached = true })
	s := newTestServer(t, fc)

	rec := doAuthFormRequest(t, s, "/settings",
		"routing_mode=networks&routing_networks=10.8.0.0%2F24%0Anot-a-network")
	assertRedirect(t, rec, "/settings")
	if reached {
		t.Error("an unparseable routing network was sent to the core instead of being named on the page")
	}
}

// The card renders with the stored mode selected, in the page's own language.
func TestSettingsGET_RendersTheRoutingCard(t *testing.T) {
	fc := newFakeCore(t)
	fc.SetResponse(shared.CmdGetSettings, successResp(shared.NetworkSettings{
		IPv6:    shared.IPv6Config{Mode: shared.IPv6Filter},
		Routing: shared.RoutingConfig{Mode: shared.RoutingNetworks, Networks: []string{"10.8.0.0/24"}},
	}))
	s := newTestServer(t, fc)

	rec := doAuthRequest(t, s, "GET", "/settings", nil)
	assertStatus(t, rec, http.StatusOK)
	body := rec.Body.String()

	if !strings.Contains(body, `name="routing_mode" value="networks" class="radio"`) {
		t.Error("the routing radio group is not on the page")
	}
	// The stored mode is the one that comes back checked.
	idx := strings.Index(body, `value="networks" class="radio"`)
	if idx < 0 || !strings.Contains(body[idx:min(idx+120, len(body))], "checked") {
		t.Error("the stored routing mode is not the one selected")
	}
	if !strings.Contains(body, "10.8.0.0/24") {
		t.Error("the stored routing networks are not in the textarea")
	}
}
