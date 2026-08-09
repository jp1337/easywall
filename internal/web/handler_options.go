package web

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

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
		s.respondPartialError(w, r, "/options", "save_error")
		return
	}

	// strconv, not fmt.Sscanf: Sscanf stops at the first character it cannot
	// read and reports success for what it got, so "5abc" arrived as 5 and
	// "5 10" as 5. The same loose parse was taken out of the port validation and
	// the rule builders; this was the third copy of it.
	parseInt := func(name string, fallback int) int {
		v := strings.TrimSpace(r.FormValue(name))
		if v == "" {
			return fallback
		}
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			slog.Info("ignoring an unusable value on the options page; using the default",
				"field", name, "got", v, "using", fallback)
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
		s.respondPartialError(w, r, "/options", "save_error")
		return
	}

	s.respondPartialSave(w, r, "/options", "options_saved")
}
