package web

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The forwarding editor must read its port fields as numbers, not re-parse the
// displayed text.
//
// They are <input type="number">, and `parseInt(el.value, 10)` does not agree
// with the field about what it holds. A spreadsheet writes 10000 as "1E+04"; the
// field accepts that as a valid number and parseInt stops at the "1". Measured
// in Chrome against the running interface, before the fix: typed 1E+04 into the
// incoming port, the hidden payload carried source_port 1, the page said
// "Changes saved.", and the stored rule was tcp 1 → 9999 — a privileged port
// nobody asked for, with nothing on screen to say so. After the fix the same
// input stores 10000.
//
// This is a source assertion, and it is worth saying why: the behaviour lives in
// a browser's number parsing, so proving it needs a browser, and there is no
// browser in this suite. What this guards is the one line that made the two
// disagree — enough that reintroducing parseInt here fails before it ships.
func TestForwardingEditorReadsPortsAsNumbers(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join(root, "web", "static", "app.js"))
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}

	// Comments stripped first: the fix carries a comment that names parseInt as
	// the thing it replaced, and a check that reads comments fails on its own
	// explanation.
	editor := stripLineComments(forwardingEditorSource(t, string(src)))

	if strings.Contains(editor, "parseInt") {
		t.Error("the forwarding editor parses a port with parseInt again: " +
			"parseInt(\"1E+04\", 10) is 1, and <input type=number> reads the same text as " +
			"10000, so a pasted port list is stored as port 1 and the page reports success. " +
			"Use valueAsNumber, which is the field's own value.")
	}
	if !strings.Contains(editor, "valueAsNumber") {
		t.Error("the forwarding editor no longer reads valueAsNumber; whatever replaced it " +
			"has to agree with the number field about what the field holds")
	}
}

// stripLineComments removes // comments so the assertions read code only. Good
// enough for this file: app.js has no string literal containing "//".
func stripLineComments(src string) string {
	var b strings.Builder
	for _, line := range strings.Split(src, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// forwardingEditorSource returns the body of initForwardingEditor, so the
// assertions above cannot be satisfied or broken by an unrelated part of the
// file. The port editor next door legitimately handles text — its field accepts
// "8000:9000" — and must not be caught by this.
func forwardingEditorSource(t *testing.T, src string) string {
	t.Helper()
	start := strings.Index(src, "function initForwardingEditor()")
	if start < 0 {
		t.Fatal("initForwardingEditor not found in app.js — this test needs updating with it")
	}
	rest := src[start:]
	// The next top-level function declaration ends it.
	if m := regexp.MustCompile(`(?m)^function \w+\(`).FindStringIndex(rest[1:]); m != nil {
		return rest[:m[0]+1]
	}
	return rest
}
