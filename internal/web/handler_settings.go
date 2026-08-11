package web

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/jp1337/easywall/internal/shared"
)

type settingsData struct {
	Settings        *shared.NetworkSettings
	CustomNetworks  string // pre-joined for textarea display
	RoutingNetworks string // likewise
	CoreErr         string
}

func (s *Server) handleSettingsGET(w http.ResponseWriter, r *http.Request) {
	ns, err := s.client.GetSettings()
	if err != nil {
		slog.Warn("get settings error", "error", err)
		s.render(w, r, "settings.html", "settings", &settingsData{CoreErr: err.Error()})
		return
	}
	s.render(w, r, "settings.html", "settings", &settingsData{
		Settings:        ns,
		CustomNetworks:  strings.Join(ns.Docker.CustomNetworks, "\n"),
		RoutingNetworks: strings.Join(ns.Routing.Networks, "\n"),
	})
}

func (s *Server) handleSettingsPOST(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.respondPartialError(w, r, "/settings", "save_error")
		return
	}

	// The core refuses an unparseable network, but a redirect with a generic
	// "save failed" leaves the operator hunting. Name the lines here.
	for _, field := range []string{"custom_networks", "routing_networks"} {
		if errs := validateIPListEntries(r.FormValue(field)); len(errs) > 0 {
			s.respondPartialError(w, r, "/settings", "save_invalid_entries")
			return
		}
	}

	ns := shared.NetworkSettings{
		IPv6: shared.IPv6Config{
			// A three-way choice, not a toggle: the old boolean claimed "off
			// means IPv6 is not filtered at all" and did the opposite.
			Mode:                           ipv6ModeFromForm(r.FormValue("ipv6_mode")),
			ICMPAllowRouterAdvertisement:   r.FormValue("icmp_allow_router_advertisement") != "",
			ICMPAllowNeighborAdvertisement: r.FormValue("icmp_allow_neighbor_advertisement") != "",
		},
		Docker: shared.DockerConfig{
			Enabled:             r.FormValue("docker_enabled") != "",
			AllowBridgeNetworks: r.FormValue("allow_bridge_networks") != "",
			CustomNetworks:      parseIPList(r.FormValue("custom_networks")),
		},
		Routing: shared.RoutingConfig{
			Mode:     routingModeFromForm(r.FormValue("routing_mode")),
			Networks: parseIPList(r.FormValue("routing_networks")),
		},
	}

	if err := s.client.SaveSettings(ns); err != nil {
		slog.Warn("save settings error", "error", err)
		s.respondPartialError(w, r, "/settings", "save_error")
		return
	}

	s.respondPartialSave(w, r, "/settings", "settings_saved")
}

// routingModeFromForm maps the submitted value to a mode, falling back to
// closed. Same reasoning as ipv6ModeFromForm below, and the same direction: a
// value nobody recognises must not turn into "open", which is the one setting
// that stops easywall having an opinion about routed traffic at all.
func routingModeFromForm(v string) shared.RoutingMode {
	m := shared.RoutingMode(v)
	if m.Valid() {
		return m
	}
	return shared.RoutingClosed
}

// ipv6ModeFromForm maps the submitted value to a mode, falling back to the
// filtering default. An unrecognised value must not silently become
// passthrough or block — those open or close the host.
func ipv6ModeFromForm(v string) shared.IPv6Mode {
	m := shared.IPv6Mode(v)
	if m.Valid() {
		return m
	}
	return shared.IPv6Filter
}
