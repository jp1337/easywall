package web

import "fmt"

// filtersDocsURL is the published page every option's disclosure links into.
// Absolute, as password.html's is: buildRouter serves no /docs, so a relative
// link would 404 on a live instance.
const filtersDocsURL = "https://easywall-project.org/docs/features/filters/"

// optionHelp is what /options says about one protection module beyond its name:
// the one-liner, and the three lines of its disclosure. Key is the module's toml
// key — the form field, the key in [firewall], and the card's id="opt-<key>".
// Anchor is a heading id in docs/_docs/features/filters.md.
//
// The sentences live in the locale files; this table only says which ones
// belong to which card, so the fourteen cards cannot drift into fourteen
// hand-copied blocks. TestOptionHelpsAreComplete reads it.
type optionHelp struct {
	Key, Anchor string
}

func (o optionHelp) DescKey() string     { return "opt_" + o.Key + "_desc" }
func (o optionHelp) ProtectsKey() string { return "opt_" + o.Key + "_protects" }
func (o optionHelp) BreaksKey() string   { return "opt_" + o.Key + "_breaks" }
func (o optionHelp) WhenKey() string     { return "opt_" + o.Key + "_when" }
func (o optionHelp) DocsURL() string     { return filtersDocsURL + "#" + o.Anchor }

// optionHelps is in the order options.html renders the cards.
var optionHelps = []optionHelp{
	{"ssh_brute_force", "attack-protection"},
	{"icmp_flood", "attack-protection"},
	{"syn_flood", "attack-protection"},
	{"port_scan", "attack-protection"},
	{"drop_invalid_packets", "attack-protection"},
	{"drop_fragments", "what-fragment-drop-breaks"},
	{"bogon_filter", "what-the-bogon-filter-drops"},
	{"connection_limit_per_ip", "attack-protection"},
	{"tcp_rst_flood", "attack-protection"},
	{"drop_broadcast", "traffic-filtering"},
	{"drop_multicast", "traffic-filtering"},
	{"drop_anycast", "traffic-filtering"},
	{"log_blocked_connections", "logging"},
	{"log_blocklist_connections", "logging"},
}

// optionHelpFor is the template's lookup. An unknown key is an error, not an
// empty disclosure: render() buffers, so the page fails whole and
// TestOptionsPageExplainsEveryModule sees it, instead of a card that
// silently says nothing.
func optionHelpFor(key string) (optionHelp, error) {
	for _, h := range optionHelps {
		if h.Key == key {
			return h, nil
		}
	}
	return optionHelp{}, fmt.Errorf("optHelp: no entry for option %q in optionHelps", key)
}
