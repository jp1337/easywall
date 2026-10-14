package web

import (
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// The demo shows the country column from its own table and fetches nothing
// (2.27): no runner, no client. The packet Antarctica's feed refused reads
// AQ, and at least one row offers Block for a country not switched on.
func TestTheDemoNamesCountriesFromItsOwnTable(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	s.cfg.DemoMode = true
	for _, on := range []bool{false, true} {
		s.cfg.CountryLookup = on
		demo, err := NewServer(s.cfg)
		if err != nil {
			t.Fatalf("NewServer: %v", err)
		}
		defer demo.Stop()
		if got := demo.lookup != nil; got != on {
			t.Fatalf("country_lookup = %v: the demo built a lookup = %v", on, got)
		}
		if !on {
			continue
		}
		if demo.feeds != nil || demo.lookup.http != nil {
			t.Fatal("the demo's lookup could fetch")
		}
		named, offered := 0, false
		for _, e := range demoShapes {
			cc := demo.lookup.country(e.Src)
			if want, ok := shared.CountryCode(e.Feed); ok && cc != want {
				t.Errorf("%s was refused by %s and reads %q", e.Src, e.Feed, cc)
			}
			if cc != "" {
				named++
				offered = offered || cc != "aq"
			}
		}
		if named < 3 || !offered || !demo.lookup.status().Loaded {
			t.Errorf("%d demo sources named, a Block offered = %v; want the column to have something to show", named, offered)
		}
	}
}
