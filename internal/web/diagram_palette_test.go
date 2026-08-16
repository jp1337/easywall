package web

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// cssTokens reads the `--name: value;` pairs of one block of web/src/docs.css.
// The dark palette is :root, the light one is the [data-theme="easywall-light"]
// block, which is how the stylesheet is organised.
func cssTokens(t *testing.T, css, opener string) map[string]string {
	t.Helper()
	start := strings.Index(css, opener)
	if start < 0 {
		t.Fatalf("docs.css has no %s block", opener)
	}
	open := strings.Index(css[start:], "{")
	end := strings.Index(css[start:], "\n}")
	if open < 0 || end < 0 || end < open {
		t.Fatalf("could not find the bounds of the %s block", opener)
	}
	body := css[start+open : start+end]

	out := map[string]string{}
	for _, m := range regexp.MustCompile(`(--[a-z0-9-]+)\s*:\s*([^;]+);`).FindAllStringSubmatch(body, -1) {
		out[m[1]] = strings.TrimSpace(m[2])
	}
	return out
}

// jsTheme reads one theme object out of scripts/render-diagrams.mjs.
func jsTheme(t *testing.T, src, name string) map[string]string {
	t.Helper()
	start := strings.Index(src, name+": {")
	if start < 0 {
		t.Fatalf("render-diagrams.mjs has no %s theme", name)
	}
	end := strings.Index(src[start:], "},")
	if end < 0 {
		t.Fatalf("could not find the end of the %s theme", name)
	}
	body := src[start : start+end]

	out := map[string]string{}
	for _, m := range regexp.MustCompile(`(\w+):\s*'([^']*)'`).FindAllStringSubmatch(body, -1) {
		out[m[1]] = m[2]
	}
	return out
}

// The committed diagrams are drawn in colours written out in
// scripts/render-diagrams.mjs, and every one of them is a design token that also
// exists in web/src/docs.css. Two copies, with "keep them in step with
// web/src/docs.css" above one of them and nothing enforcing it.
//
// It matters more here than a duplicated constant usually does, because the
// staleness check cannot see it: the digest stamped into each SVG covers the
// .mmd source, the mermaid version and the font file — not the colours. A token
// changed in the stylesheet would leave fourteen committed pictures drawn in the
// previous palette while `npm run check:diagrams` still reported them current.
// This test is what turns that into a failure, at the moment the token changes.
//
// Deliberately a comparison rather than making the script read the stylesheet.
// Both would work; this one fails immediately and names the token that moved,
// where the other needs a rebuild to reveal anything — and `build:diagrams` is
// not byte-reproducible (see the note on STAMP in render-diagrams.mjs), so a
// rebuild is not a free way to answer a question about colour.
func TestTheDiagramPaletteIsTheDocumentationPalette(t *testing.T) {
	root := filepath.Dir(localesDir(t))

	cssRaw, err := os.ReadFile(filepath.Join(root, "web", "src", "docs.css"))
	if err != nil {
		t.Fatalf("read docs.css: %v", err)
	}
	jsRaw, err := os.ReadFile(filepath.Join(root, "scripts", "render-diagrams.mjs"))
	if err != nil {
		t.Fatalf("read render-diagrams.mjs: %v", err)
	}
	css, js := string(cssRaw), string(jsRaw)

	// Which token each mermaid variable takes. lineColor is the one that is not
	// the same token in both themes: a connector has to carry against the page
	// behind it, and on light the quietest text token does not.
	tokens := map[string]string{
		"primaryColor":        "--surface-2",
		"primaryTextColor":    "--text",
		"primaryBorderColor":  "--control-edge",
		"secondaryColor":      "--surface",
		"tertiaryColor":       "--bg",
		"noteBkgColor":        "--surface-2",
		"noteTextColor":       "--text-muted",
		"noteBorderColor":     "--border",
		"actorBkg":            "--surface-2",
		"actorBorder":         "--control-edge",
		"actorTextColor":      "--text",
		"signalColor":         "--text-muted",
		"signalTextColor":     "--text-muted",
		"labelBoxBkgColor":    "--surface-2",
		"labelBoxBorderColor": "--control-edge",
		"labelTextColor":      "--text",
	}
	lineToken := map[string]string{"dark": "--text-subtle", "light": "--text-muted"}

	for _, tc := range []struct{ theme, opener string }{
		{"dark", ":root"},
		{"light", `[data-theme="easywall-light"]`},
	} {
		t.Run(tc.theme, func(t *testing.T) {
			want := cssTokens(t, css, tc.opener)
			got := jsTheme(t, js, tc.theme)
			if len(got) == 0 {
				t.Fatal("no colours parsed out of the theme; the file changed shape")
			}

			for mermaidVar, token := range tokens {
				check(t, tc.theme, mermaidVar, token, got, want)
			}
			check(t, tc.theme, "lineColor", lineToken[tc.theme], got, want)
		})
	}
}

func check(t *testing.T, theme, mermaidVar, token string, got, want map[string]string) {
	t.Helper()
	expected, ok := want[token]
	if !ok {
		t.Errorf("docs.css defines no %s, which %s %s is supposed to take", token, theme, mermaidVar)
		return
	}
	actual, ok := got[mermaidVar]
	if !ok {
		t.Errorf("render-diagrams.mjs %s theme has no %s", theme, mermaidVar)
		return
	}
	if !strings.EqualFold(actual, expected) {
		t.Errorf("%s %s is %s, but docs.css %s is %s\n"+
			"  the committed diagrams would be drawn in a colour the documentation site "+
			"no longer uses, and check:diagrams cannot see it — the digest covers the "+
			".mmd source and the mermaid version, not the palette",
			theme, mermaidVar, actual, token, expected)
	}
}
