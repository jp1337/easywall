package shared

import (
	"sort"
	"strings"
	"testing"
)

// The picker renders Catalogue in file order and relies on the doc comment's
// claim that it needs no sort. Assert it, case-insensitively by Name, so a
// misplaced entry (Redis once followed Remote Desktop) fails here instead of
// only being noticeable in the rendered list.
func TestCatalogueIsSortedByName(t *testing.T) {
	names := make([]string, len(Catalogue))
	for i, s := range Catalogue {
		names[i] = s.Name
	}
	if !sort.SliceIsSorted(names, func(i, j int) bool {
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	}) {
		t.Errorf("Catalogue is not ordered by name: %v", names)
	}
}

// An id is what a stored rule points at. Two entries sharing one, or an id
// changing, silently relabels somebody's rule.
func TestCatalogueIDsAreUniqueAndStable(t *testing.T) {
	seen := map[string]string{}
	for _, s := range Catalogue {
		if s.ID == "" {
			t.Errorf("service %q has no id", s.Name)
		}
		if prev, dup := seen[s.ID]; dup {
			t.Errorf("id %q is used by both %q and %q", s.ID, prev, s.Name)
		}
		seen[s.ID] = s.Name
	}
}

// Every port in the catalogue is a port the rest of the product will accept. An
// entry that cannot be saved is a button that produces an error message.
func TestEveryCataloguePortIsStorable(t *testing.T) {
	for _, s := range Catalogue {
		if len(s.Ports) == 0 {
			t.Errorf("%s has no ports", s.ID)
		}
		for _, p := range s.Ports {
			if p.Proto != "tcp" && p.Proto != "udp" {
				t.Errorf("%s: port %q has protocol %q", s.ID, p.Port, p.Proto)
			}
			if err := validatePortRule(PortRule{Port: p.Port}); err != nil {
				t.Errorf("%s: port %q: %v", s.ID, p.Port, err)
			}
		}
	}
}

// The suggestion is a constant, so the reasoning beside it can be translated and
// the private ranges live in one place. A free-form value would be neither.
func TestEverySuggestionIsKnownAndProducesSources(t *testing.T) {
	known := map[Suggestion]bool{}
	for _, s := range AllSuggestions {
		known[s] = true
	}
	for _, s := range Catalogue {
		if !known[s.Suggest] {
			t.Errorf("%s suggests %q, which is not in AllSuggestions", s.ID, s.Suggest)
		}
	}
	if got := SuggestedSources(SuggestAnywhere); len(got) != 0 {
		t.Errorf("SuggestedSources(anywhere) = %v, want empty — empty is what anywhere means", got)
	}
	if got := SuggestedSources(SuggestPrivate); len(got) == 0 {
		t.Error("SuggestedSources(private) is empty, which would open the port to the world")
	}
	// And what it suggests must be storable, or the picker fills a field the
	// server then refuses.
	if err := ValidateRules(Rules{TCP: []PortRule{
		{Port: "443", Sources: SuggestedSources(SuggestPrivate)}}}); err != nil {
		t.Errorf("the private suggestion does not validate: %v", err)
	}
	// The caller must not be able to edit the shared slice.
	SuggestedSources(SuggestPrivate)[0] = "0.0.0.0/0"
	if PrivateRanges[0] == "0.0.0.0/0" {
		t.Error("SuggestedSources returns the package slice; a caller can open every restriction")
	}
}

func TestServiceByID(t *testing.T) {
	if _, ok := ServiceByID("pihole"); !ok {
		t.Error("ServiceByID(pihole) not found")
	}
	if _, ok := ServiceByID("no-such-service"); ok {
		t.Error("ServiceByID invented a service")
	}
}
