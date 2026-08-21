package web

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// appStylesheet returns the built application stylesheet.
func appStylesheet(t *testing.T) string {
	t.Helper()
	root := filepath.Dir(localesDir(t))
	raw, err := os.ReadFile(filepath.Join(root, "web", "static", "style.css"))
	if err != nil {
		t.Fatalf("read built stylesheet: %v", err)
	}
	return string(raw)
}

// statTileRule returns the body of the built `.stat-tile{…}` rule.
//
// Anchored on the class followed by `{` so the many `.stat-tile-*` rules — label,
// value, note, cta — are not matched instead.
func statTileRule(t *testing.T) string {
	t.Helper()
	m := regexp.MustCompile(`\.stat-tile\{([^}]*)\}`).FindStringSubmatch(appStylesheet(t))
	if m == nil {
		t.Fatal("web/static/style.css has no .stat-tile rule — renamed, or the build dropped it")
	}
	return m[1]
}

// The dashboard's six tiles take their four rows — label, value, note, call to
// action — from the grid above them, so the four line up across a row however
// long any one label is. A label that wraps otherwise pushes only its own number
// down: French says "Règles personnalisées" where English says "Custom rules",
// and two numbers of six sat 20px below the other four.
//
// Reserving a second label line would have been the other fix, and it costs
// every language 18px of empty tile whether or not anything wraps. This one
// costs nothing, which is exactly why it is worth a guard: a Tailwind rebuild
// that drops it leaves a stylesheet that builds green and a dashboard that goes
// ragged in one language nobody on this repository reads first.
func TestStatTileRowsComeFromTheGrid(t *testing.T) {
	rule := statTileRule(t)

	for _, want := range []struct{ decl, breaks string }{
		{"display:grid", "the tile stacks its own children and the rows stop being shared"},
		{"grid-template-rows:subgrid", "each tile sizes its rows alone, so a wrapped label shifts only its own number"},
	} {
		if !strings.Contains(rule, want.decl) {
			t.Errorf(".stat-tile has no %q — %s", want.decl, want.breaks)
		}
	}
}

// A subgrid spans a fixed number of rows, so the count in the stylesheet has to
// equal the number of children the markup actually gives it. Get them out of
// step and the tile either leaves an empty row at the bottom or overflows the
// rows it claimed — and nothing else in the suite would notice, because both
// halves are individually valid.
func TestStatTileSpanMatchesItsChildren(t *testing.T) {
	m := regexp.MustCompile(`grid-row:span (\d+)`).FindStringSubmatch(statTileRule(t))
	if m == nil {
		t.Fatal(".stat-tile has no grid-row:span — without it the tile occupies one row and subgrid has nothing to inherit")
	}
	span, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("unreadable span %q: %v", m[1], err)
	}

	raw, err := os.ReadFile(filepath.Join(templatesDir(t), "dashboard.html"))
	if err != nil {
		t.Fatal(err)
	}

	tiles := statTileBlocks(string(raw))
	if len(tiles) == 0 {
		t.Fatal(`dashboard.html has no class="stat-tile" — the markup moved`)
	}
	for i, block := range tiles {
		if got := directChildren(block); got != span {
			t.Errorf("stat tile %d has %d children but the stylesheet spans %d rows", i+1, got, span)
		}
	}
}

// statTileBlocks returns the inner markup of every `<a class="stat-tile">…</a>`.
func statTileBlocks(src string) []string {
	const marker = `class="stat-tile">`
	var out []string
	for rest := src; ; {
		i := strings.Index(rest, marker)
		if i < 0 {
			return out
		}
		rest = rest[i+len(marker):]
		end := strings.Index(rest, "</a>")
		if end < 0 {
			return out
		}
		out = append(out, rest[:end])
		rest = rest[end:]
	}
}

// directChildren counts the elements at the top level of a block of markup.
//
// A depth counter rather than a pattern over the child classes: the point is to
// count what the grid will place, which is every direct child whatever it is
// called, so a fifth one added later fails the test above instead of matching
// nothing. `<path … />` inside the icon is self-closing and changes no depth.
func directChildren(block string) int {
	depth, count := 0, 0
	for i := 0; i < len(block); i++ {
		if block[i] != '<' {
			continue
		}
		end := strings.IndexByte(block[i:], '>')
		if end < 0 {
			break
		}
		tag := block[i : i+end+1]
		switch {
		case strings.HasPrefix(tag, "</"):
			depth--
		case strings.HasSuffix(tag, "/>"):
			if depth == 0 {
				count++
			}
		default:
			if depth == 0 {
				count++
			}
			depth++
		}
		i += end
	}
	return count
}
