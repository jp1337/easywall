package shared

import (
	"slices"
	"strings"
	"time"
)

// Countries (2.26): one feed per country, id "country-<cc>", switched on in
// Rules.Feeds beside the catalogue (spec D3, D4). The source is
// ipverse/country-ip-blocks (CC0 1.0), built daily from the five RIRs'
// delegation files. Like the catalogue, the core knows only which ids exist.

// FeedFormatIPverse is ipverse's country/<cc>/aggregated.json:
// {"prefixes":{"ipv4":[…],"ipv6":[…]}}. One file for both families, so a
// country is a one-URL feed and its daily refresh is a conditional GET — a
// 304 when nothing changed (measured 2026-10-13). The two .txt files the spec
// named would make it a two-URL feed, which refreshFrom never asks
// conditionally, and six countries publish no ipv6 file at all.
const FeedFormatIPverse FeedFormat = "ipverse-json"

const countryFeedPrefix = "country-"

// MaxCountries is how many countries may be switched on (spec D5). Every
// refresh reads GET_FEEDS for every id and rewrites feeds.json whole, and
// every country adds two sets and two to four rules.
// TestIntegration_ARealisticRuleSetWithFeedsApplies loads the thirty largest
// at their measured sizes; this is not raised past what that test passes.
const MaxCountries = 30

// CountryInterval: ipverse rebuilds once a day, between 01:23 and 01:25 UTC
// on the five days measured (spec D9).
const CountryInterval = 24 * time.Hour

// FeedMaxBitsV4Country and FeedMaxBitsV6Country are a country's breadth
// limits. The lists' /8 refuses the United States: its aggregated IPv4 holds
// 16.0.0.0/6 and 32.0.0.0/6 (2026-10-13), so v4 widens to /4, two bits beyond
// the broadest. IPv6 stays at the lists' /16: the broadest across all 238
// countries is /16 (the US's 2630::/16), which the lists' limit already passes.
const (
	FeedMaxBitsV4Country = 4
	FeedMaxBitsV6Country = FeedMaxBitsV6
)

// countryURLStem + cc + "/aggregated.json" is where a country's list is
// fetched. raw.githubusercontent.com answers it with an ETag and honours
// If-None-Match.
const countryURLStem = "https://raw.githubusercontent.com/ipverse/country-ip-blocks/master/country/"

// CountryCodes are the ISO 3166-1 alpha-2 codes ipverse publishes, lower
// case: its 238 country directories on 2026-10-13. Never shortened (the
// catalogue's rule, invariants.md): a code removed here makes ValidateRules
// refuse every rule set that switched it on, at boot included.
// TestCountryCodesAreNeverRemoved holds that. A code ipverse later drops
// fails its fetch with a 404 and keeps its last copy.
var CountryCodes = []string{
	"ad", "ae", "af", "ag", "ai", "al", "am", "ao", "aq", "ar", "as", "at", "au", "aw", "ax", "az",
	"ba", "bb", "bd", "be", "bf", "bg", "bh", "bi", "bj", "bl", "bm", "bn", "bo", "bq", "br", "bs",
	"bt", "bw", "by", "bz", "ca", "cd", "cf", "cg", "ch", "ci", "ck", "cl", "cm", "cn", "co", "cr",
	"cu", "cv", "cw", "cy", "cz", "de", "dj", "dk", "dm", "do", "dz", "ec", "ee", "eg", "er", "es",
	"et", "fi", "fj", "fk", "fm", "fo", "fr", "ga", "gb", "gd", "ge", "gf", "gg", "gh", "gi", "gl",
	"gm", "gn", "gp", "gq", "gr", "gt", "gu", "gw", "gy", "hk", "hn", "hr", "ht", "hu", "id", "ie",
	"il", "im", "in", "io", "iq", "ir", "is", "it", "je", "jm", "jo", "jp", "ke", "kg", "kh", "ki",
	"km", "kn", "kp", "kr", "kw", "ky", "kz", "la", "lb", "lc", "li", "lk", "lr", "ls", "lt", "lu",
	"lv", "ly", "ma", "mc", "md", "me", "mf", "mg", "mh", "mk", "ml", "mm", "mn", "mo", "mp", "mq",
	"mr", "ms", "mt", "mu", "mv", "mw", "mx", "my", "mz", "na", "nc", "ne", "nf", "ng", "ni", "nl",
	"no", "np", "nr", "nu", "nz", "om", "pa", "pe", "pf", "pg", "ph", "pk", "pl", "pm", "pr", "ps",
	"pt", "pw", "py", "qa", "re", "ro", "rs", "ru", "rw", "sa", "sb", "sc", "sd", "se", "sg", "si",
	"sk", "sl", "sm", "sn", "so", "sr", "ss", "st", "sv", "sx", "sy", "sz", "tc", "td", "tg", "th",
	"tj", "tk", "tl", "tm", "tn", "to", "tr", "tt", "tv", "tw", "tz", "ua", "ug", "us", "uy", "uz",
	"va", "vc", "ve", "vg", "vi", "vn", "vu", "wf", "ws", "ye", "yt", "za", "zm", "zw",
}

// CountryFeedID is the feed id of country cc: "de" → "country-de".
func CountryFeedID(cc string) string { return countryFeedPrefix + cc }

// CountryCode is the code a country id names, and whether it is one: true
// only for "country-" and a code in CountryCodes, spelled exactly so.
func CountryCode(id string) (string, bool) {
	cc, ok := strings.CutPrefix(id, countryFeedPrefix)
	return cc, ok && slices.Contains(CountryCodes, cc)
}

// IsCountryFeedID reports whether id is a country's.
func IsCountryFeedID(id string) bool {
	_, ok := CountryCode(id)
	return ok
}

// countryFeed is the catalogue row of country cc. Name is the code in
// capitals; the web process renders the localised name instead.
func countryFeed(cc string) CatalogueFeed {
	return CatalogueFeed{
		ID: CountryFeedID(cc), Name: strings.ToUpper(cc),
		URLs:     []string{countryURLStem + cc + "/aggregated.json"},
		Format:   FeedFormatIPverse,
		Interval: CountryInterval,
		Homepage: "https://github.com/ipverse/country-ip-blocks",
		Terms:    "https://github.com/ipverse/country-ip-blocks/blob/master/LICENSE",
	}
}

// FeedMaxBits is the broadest prefix a feed may carry, per family: a
// country's limits for a country id, the lists' for every other.
func FeedMaxBits(id string) (v4, v6 int) {
	if IsCountryFeedID(id) {
		return FeedMaxBitsV4Country, FeedMaxBitsV6Country
	}
	return FeedMaxBitsV4, FeedMaxBitsV6
}

// FeedRuleOrder is ids in the order the kernel's rules check them (spec D8):
// the lists in the order they were switched on, then the countries in theirs.
// An address on both is counted and logged by the list, which says more about
// why it is refused. The core builds the rules in this order and the lockout
// verdict names the first feed in it.
func FeedRuleOrder(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if !IsCountryFeedID(id) {
			out = append(out, id)
		}
	}
	for _, id := range ids {
		if IsCountryFeedID(id) {
			out = append(out, id)
		}
	}
	return out
}
