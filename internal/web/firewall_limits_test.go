package web

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// Three places had an opinion about how large a firewall limit may be, and no
// two of them agreed:
//
//	                                   options.html   schema      daemon
//	ssh_brute_force_connection_limit   1..9999        1..100      any n > 0
//	icmp_flood_connection_limit        1..9999        1..1000     any n > 0
//	connection_limit_max               1..9999        1..100000   any n > 0
//	log_blocked_connections_limit      1..9999        1..∞        any n > 0
//
// The daemon is the one that decides, and it had no upper bound at all. `max` on
// an <input type=number> is a hint to the browser; the schema is a hint to an
// editor. Neither stops a curl, a hand-edited easywall.toml, or a browser with
// the attribute removed — and the values reach 32-bit nftables fields, so a large
// one wraps rather than failing. Measured against a real kernel:
//
//	connection_limit_max = 4294967296  ->  ct count over 0   ← drops every connection
//
// This derives both artefacts from shared.FirewallLimits, which is the table the
// daemon enforces, so the next limit added has to be described in one place.
func TestTheAdvertisedLimitsAreTheOnesTheDaemonEnforces(t *testing.T) {
	root := filepath.Dir(localesDir(t))

	// ── The options page ────────────────────────────────────────────────
	tmpl, err := os.ReadFile(filepath.Join(root, "web", "templates", "options.html"))
	if err != nil {
		t.Fatalf("read options.html: %v", err)
	}
	inputRe := regexp.MustCompile(`name="([a-z_]+)"[^>]*?min="(\d+)"\s+max="(\d+)"`)
	inputs := make(map[string][2]int)
	for _, m := range inputRe.FindAllStringSubmatch(string(tmpl), -1) {
		lo, _ := strconv.Atoi(m[2])
		hi, _ := strconv.Atoi(m[3])
		inputs[m[1]] = [2]int{lo, hi}
	}
	if len(inputs) == 0 {
		t.Fatal("no numeric inputs found in options.html — the regex or the template changed shape")
	}

	// ── The schema an operator's editor validates easywall.toml against ──
	raw, err := os.ReadFile(filepath.Join(root, "docs", "schemas", "easywall.schema.json"))
	if err != nil {
		t.Fatalf("read easywall.schema.json: %v", err)
	}
	var schema struct {
		Properties struct {
			Firewall struct {
				Properties map[string]struct {
					Minimum *int `json:"minimum"`
					Maximum *int `json:"maximum"`
					// The firewall section mixes numbers and switches, so the
					// default of a neighbouring key is a bool.
					Default any `json:"default"`
				} `json:"properties"`
			} `json:"firewall"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("easywall.schema.json does not parse: %v", err)
	}
	props := schema.Properties.Firewall.Properties

	for _, l := range shared.FirewallLimits {
		t.Run(l.Key, func(t *testing.T) {
			got, ok := inputs[l.Key]
			if !ok {
				t.Errorf("options.html has no numeric input for %q, so the page cannot set it", l.Key)
			} else if got != [2]int{l.Min, l.Max} {
				t.Errorf("options.html offers %d..%d, the daemon enforces %d..%d — "+
					"the page invites a value that will be refused, or refuses one that is fine",
					got[0], got[1], l.Min, l.Max)
			}

			p, ok := props[l.Key]
			if !ok {
				t.Fatalf("the schema has no %q, so an editor reports a valid file as invalid", l.Key)
			}
			switch {
			case p.Minimum == nil || p.Maximum == nil:
				t.Errorf("the schema states no range for %q; the daemon enforces %d..%d",
					l.Key, l.Min, l.Max)
			case *p.Minimum != l.Min || *p.Maximum != l.Max:
				t.Errorf("the schema says %d..%d, the daemon enforces %d..%d",
					*p.Minimum, *p.Maximum, l.Min, l.Max)
			}
			if n, ok := p.Default.(float64); ok && int(n) != l.Default {
				t.Errorf("the schema's default is %d, the daemon substitutes %d", int(n), l.Default)
			}
			if !l.InRange(l.Default) {
				t.Errorf("the default %d is outside the range %d..%d it belongs to",
					l.Default, l.Min, l.Max)
			}
		})
	}

	// Every numeric input on the page must be a limit the daemon knows, or it is
	// a field that sets nothing.
	known := make(map[string]bool, len(shared.FirewallLimits))
	for _, l := range shared.FirewallLimits {
		known[l.Key] = true
	}
	for name := range inputs {
		if !known[name] {
			t.Errorf("options.html offers %q, which shared.FirewallLimits does not describe", name)
		}
	}
}
