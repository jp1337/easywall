package web

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/jp1337/easywall/internal/shared"
)

type optionsData struct {
	Options *shared.FirewallOptions
	CoreErr string
}

func (s *Server) handleOptionsGET(w http.ResponseWriter, r *http.Request) {
	opts, err := s.client.GetOptions()
	if err != nil {
		slog.Warn("get options error", "error", err)
		s.render(w, r, "options.html", "options", &optionsData{CoreErr: err.Error()})
		return
	}
	s.render(w, r, "options.html", "options", &optionsData{Options: opts})
}

func (s *Server) handleOptionsPOST(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.setFlash(w, r, "save_error")
		http.Redirect(w, r, "/options", http.StatusSeeOther)
		return
	}

	parseInt := func(name string, fallback int) int {
		v := r.FormValue(name)
		if v == "" {
			return fallback
		}
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err != nil || n <= 0 {
			return fallback
		}
		return n
	}

	opts := shared.FirewallOptions{
		SSHBruteForce:                r.FormValue("ssh_brute_force") != "",
		SSHBruteForceLog:             r.FormValue("ssh_brute_force_log") != "",
		SSHBruteForceConnectionLimit: parseInt("ssh_brute_force_connection_limit", 5),
		SSHBruteForceLogLimit:        parseInt("ssh_brute_force_log_limit", 60),
		ICMPFlood:                    r.FormValue("icmp_flood") != "",
		ICMPFloodLog:                 r.FormValue("icmp_flood_log") != "",
		ICMPFloodConnectionLimit:     parseInt("icmp_flood_connection_limit", 10),
		ICMPFloodLogLimit:            parseInt("icmp_flood_log_limit", 60),
		SYNFlood:                     r.FormValue("syn_flood") != "",
		SYNFloodLog:                  r.FormValue("syn_flood_log") != "",
		SYNFloodLimit:                parseInt("syn_flood_limit", 100),
		PortScan:                     r.FormValue("port_scan") != "",
		PortScanLog:                  r.FormValue("port_scan_log") != "",
		InvalidPackets:               r.FormValue("drop_invalid_packets") != "",
		InvalidPacketsLog:            r.FormValue("drop_invalid_packets_log") != "",
		Fragments:                    r.FormValue("drop_fragments") != "",
		FragmentsLog:                 r.FormValue("drop_fragments_log") != "",
		Bogons:                       r.FormValue("bogon_filter") != "",
		BogonsLog:                    r.FormValue("bogon_filter_log") != "",
		ConnectionLimit:              r.FormValue("connection_limit_per_ip") != "",
		ConnectionLimitMax:           parseInt("connection_limit_max", 100),
		TCPRSTFlood:                  r.FormValue("tcp_rst_flood") != "",
		TCPRSTFloodLog:               r.FormValue("tcp_rst_flood_log") != "",
		TCPRSTFloodLimit:             parseInt("tcp_rst_flood_limit", 100),
		DropBroadcast:                r.FormValue("drop_broadcast") != "",
		DropMulticast:                r.FormValue("drop_multicast") != "",
		DropAnycast:                  r.FormValue("drop_anycast") != "",
		LogBlocked:                   r.FormValue("log_blocked_connections") != "",
		LogBlockedLimit:              parseInt("log_blocked_connections_limit", 60),
		LogBlacklist:                 r.FormValue("log_blacklist_connections") != "",
		LogBlacklistLimit:            parseInt("log_blacklist_connections_limit", 60),
	}

	if err := s.client.SaveOptions(opts); err != nil {
		slog.Warn("save options error", "error", err)
		s.setFlash(w, r, "save_error")
		http.Redirect(w, r, "/options", http.StatusSeeOther)
		return
	}

	s.setFlash(w, r, "options_saved")
	http.Redirect(w, r, "/options", http.StatusSeeOther)
}
