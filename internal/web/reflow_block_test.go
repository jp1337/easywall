package web

import (
	"strings"
	"testing"
)

// containerBlock returns the body of the first @container rule whose header
// starts with prefix, brace-matched out of the built stylesheet.
func containerBlock(t *testing.T, css, prefix string) string {
	t.Helper()
	start := strings.Index(css, prefix)
	if start < 0 {
		t.Fatalf("no %q in the built stylesheet", prefix)
	}
	open := strings.Index(css[start:], "{")
	if open < 0 {
		t.Fatalf("%q has no body", prefix)
	}
	open += start
	depth := 0
	for i := open; i < len(css); i++ {
		switch css[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return css[open+1 : i]
			}
		}
	}
	t.Fatalf("%q is never closed", prefix)
	return ""
}

// /ports reflows to cards at its own width, and it has to be the same cards.
//
// The card layout exists once, at `@container (max-width: 940px)`, for every
// .table-wrap in the app. /ports carries a seventh column from 2.19 and runs
// out of room 283px earlier, so it has its own container name and its own
// threshold — and, because a container query is an at-rule and two conditions
// cannot be OR'd into one, its own copy of the block.
//
// A copy is a thing that drifts. A declaration added to the shared block and
// not to this one would give /ports a card layout subtly unlike every other
// page's — different padding, a label that no longer travels with its value —
// at exactly the widths a phone uses, where nobody is looking. The two bodies
// are therefore compared byte for byte in the built stylesheet, which is what
// ships and what has already dropped a rule silently once.
func TestThePortsReflowIsTheSharedReflow(t *testing.T) {
	css := appStylesheet(t)

	shared := containerBlock(t, css, "@container (max-width:940px)")
	ports := containerBlock(t, css, "@container ports-table (max-width:")

	if shared == "" {
		t.Fatal("the shared reflow block is empty; this test can no longer tell the two apart")
	}
	if shared != ports {
		t.Errorf("the /ports reflow block has drifted from the shared one.\n"+
			"  shared (%d bytes): %s\n  ports  (%d bytes): %s",
			len(shared), shared, len(ports), ports)
	}
}

// And the page that needs the earlier threshold is the one that gets it.
//
// The name is the whole mechanism: without `container-name: ports-table` on
// /ports' own .table-wrap, the second block matches nothing and the page goes
// back to clipping five descriptions at 1440px with every test green — which
// is how this shipped to review in the first place.
func TestThePortsTableWrapCarriesTheContainerName(t *testing.T) {
	css := appStylesheet(t)
	if !strings.Contains(css, ".page-grid-ports .table-wrap{container-name:ports-table}") {
		t.Error("the /ports .table-wrap does not name its container, so the ports-table " +
			"query below it addresses nothing")
	}
	if !strings.Contains(css, "container-type:inline-size") {
		t.Error("no container-type in the built stylesheet: a named container that is not " +
			"a container answers no query at all")
	}
}
