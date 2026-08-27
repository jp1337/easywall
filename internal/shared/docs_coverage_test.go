package shared

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// Every page an operator can open has a page in the documentation that describes
// it.
//
// This is the same idea as TestEveryConfigKeyIsDocumented, applied to the other
// half of the surface: that test derives its list from the config structs, this
// one from the router. A page nobody documented is a feature an operator can only
// understand by clicking around it and guessing.
//
// It was three pages when this was written, and one of them mattered: /firstrun
// is the first thing every single installation shows, it decides the SSH port,
// IPv6 and whether the host is counted, and it deliberately stages everything but
// the account — behaviour that reads as a bug when nothing explains it. /apply is
// the feature easywall exists for, and its screenshots had been sitting in
// docs/assets/img/screens/ referenced by nothing at all.
//
// A new route therefore has to be answered here: name the page that documents it,
// or say why it is not a page. Both are cheap; neither happens by itself.
func TestEveryPageIsDocumented(t *testing.T) {
	// route → the page that describes it, relative to docs/. The Jekyll
	// collection holds the actual content under _docs/; the old paths outside
	// it are redirect stubs that would satisfy os.Stat without proving anything
	// is documented.
	documented := map[string]string{
		"/login":        "_docs/installation/first-run.md",
		"/login/verify": "_docs/features/two-factor.md",
		"/firstrun":     "_docs/installation/first-run.md",
		"/password":     "_docs/installation/first-run.md",
		"/dashboard":    "_docs/features/dashboard.md",
		"/apply":        "_docs/features/apply.md",
		"/ports":        "_docs/features/ports.md",
		"/blacklist":    "_docs/features/blacklist.md",
		"/whitelist":    "_docs/features/blacklist.md",
		"/forwarding":   "_docs/features/forwarding.md",
		"/custom":       "_docs/features/custom-rules.md",
		"/options":      "_docs/features/filters.md",
		"/settings":     "_docs/features/system-settings.md",
		"/system":       "_docs/features/system-settings.md",
		"/log":          "_docs/features/audit-log.md",
		"/export":       "_docs/features/export-import.md",
	}

	// Not pages: redirects, polling endpoints, fragments answered into a page
	// that is already documented, and the asset handler. Each one is listed by
	// name rather than matched by a pattern, so a new route cannot slip in by
	// happening to look like one of these.
	notAPage := map[string]string{
		"/":              "redirects to /dashboard",
		"/logout":        "an action, not a page — the behaviour is in security.md",
		"/apply/status":  "polled by the apply page for the countdown",
		"/log/filter":    "an htmx fragment of the audit log page",
		"/static/*":      "the asset handler",
		"/schemas/*":     "the JSON schemas, served as files",
		"/healthz":       "a probe endpoint, deliberately not in the operator docs",
		"/favicon.ico":   "an asset",
		"/robots.txt":    "an asset",
		"/.well-known/*": "an asset",
		// All four are POST, answered in place on the password page — see the
		// comment on the regex below for why they are inert here regardless.
		"/password/2fa/begin":    "a form on the password page, answered in place",
		"/password/2fa/confirm":  "a form on the password page, answered in place",
		"/password/2fa/disable":  "a form on the password page, answered in place",
		"/password/2fa/recovery": "a form on the password page, answered in place",
	}

	root := repoRootDir(t)
	server := repoFile(t, "internal", "web", "server.go")

	// Only GET: a POST route is the same page answering its own form, and the
	// pages that accept one all have a GET beside it.
	//
	// The word boundary is load-bearing: without it `r.Header.Get("HX-Request")`
	// matches, because "Header." ends in the same two characters the router does.
	routes := regexp.MustCompile(`\br\.(?:Get|Handle)\("([^"]+)"`).FindAllStringSubmatch(server, -1)
	if len(routes) < 10 {
		t.Fatalf("found %d routes in server.go; the pattern no longer matches how they "+
			"are registered, so this test would pass by finding nothing", len(routes))
	}

	for _, m := range routes {
		route := m[1]
		if reason, ok := notAPage[route]; ok {
			_ = reason
			continue
		}
		page, ok := documented[route]
		if !ok {
			t.Errorf("%s is served to operators and this test does not know a page for it\n"+
				"  add it to `documented` with the page that describes it, or to `notAPage` with the reason",
				route)
			continue
		}
		if _, err := os.Stat(filepath.Join(root, "docs", page)); err != nil {
			t.Errorf("%s is documented in docs/%s, which does not exist", route, page)
		}
	}
}

// The technical documentation is written for whoever maintains this repository,
// and it is not published.
//
// It says which incident produced which rule, what the release actually does, and
// where the packaging has bitten before. That belongs in the repository and not on
// easywall-project.org — and the way to be sure of it is structural rather than a
// list: Jekyll builds from docs/ and only from docs/, so a directory outside docs/
// cannot be published even if someone forgets an exclude entry.
//
// This test holds both halves of that: the files are outside docs/, and the
// workflow still builds from docs/. Moving the technical docs under docs/, or
// pointing the build at the repository root, fails here.
func TestTheTechnicalDocsAreNotPublished(t *testing.T) {
	root := repoRootDir(t)

	entries, err := os.ReadDir(filepath.Join(root, "docs-tech"))
	if err != nil {
		t.Fatalf("docs-tech/ is missing: %v\n"+
			"  it is where the maintainer-facing documentation lives, deliberately "+
			"outside the Jekyll source", err)
	}
	if len(entries) == 0 {
		t.Fatal("docs-tech/ is empty")
	}

	// Nothing of it may sit inside the published tree, under any name.
	for _, name := range []string{"docs-tech", "technical", "internal"} {
		if _, err := os.Stat(filepath.Join(root, "docs", name)); err == nil {
			t.Errorf("docs/%s exists — anything under docs/ is built and published by "+
				"docs.yml, which is what this directory is not for", name)
		}
	}

	// And the build still reads only docs/. Two jobs, both with the same
	// working directory; a third one, or a changed path, is a change of scope
	// that has to be noticed here.
	wf := repoFile(t, ".github", "workflows", "docs.yml")
	builds := regexp.MustCompile(`(?m)^\s+run:\s+bundle exec jekyll build`).FindAllString(wf, -1)
	dirs := regexp.MustCompile(`(?m)^\s+working-directory:\s+(\S+)`).FindAllStringSubmatch(wf, -1)
	if len(builds) == 0 {
		t.Fatal("docs.yml no longer runs `bundle exec jekyll build`; this test cannot " +
			"tell what gets published any more")
	}
	for _, d := range dirs {
		if strings.Trim(d[1], `"'`) != "docs" {
			t.Errorf("docs.yml builds from %q; only docs/ may be published, or the "+
				"technical documentation goes online with it", d[1])
		}
	}
	if len(dirs) < len(builds) {
		t.Errorf("%d jekyll build steps but only %d working-directory entries — a build "+
			"without one runs from the repository root and would publish everything",
			len(builds), len(dirs))
	}
}

// AllCommandTypes is the root of three guards — it is what the dispatch test
// iterates to verify handlers exist, and what the docs guard iterates to verify
// documentation. If an entry is deleted from this list, all three guards stop
// checking it. If a command is added to the const block without being listed,
// it ships both unhandled and undocumented.
//
// This test reads protocol.go's own source and verifies the list matches what
// is declared there. Go has no runtime enumeration of constants, so reading the
// source is the only way to make the list checkable. The cost — a test that
// fails loudly if protocol.go is renamed or the const syntax changes — is the
// acceptable price of having three other guards that can rely on this list
// being complete and accurate.
//
// The test compares on the constant *values* (what CommandType holds: "PANIC",
// "GET_RULES", etc), not their names (CmdPanic, CmdGetRules), because that is
// what appears in the list and what matters to the protocol.
func TestAllCommandTypesMatchesTheProtocolSource(t *testing.T) {
	protocolSource := repoFile(t, "internal", "shared", "protocol.go")

	// Extract all CommandType constant declarations: CmdSomething = "VALUE"
	// This pattern handles extra whitespace, trailing comments, but NOT grouped
	// declarations like "CmdFoo, CmdBar CommandType = "FOO", "BAR"".
	constPattern := regexp.MustCompile(`(Cmd\w+)\s+CommandType\s*=\s*"([^"]+)"`)
	matches := constPattern.FindAllStringSubmatch(protocolSource, -1)

	if len(matches) == 0 {
		t.Fatal("could not find any CommandType constants in protocol.go; " +
			"the pattern no longer matches or the file is missing declarations")
	}

	// Check for unparsed declarations: a line that contains multiple Cmd* names
	// but the regex only matches one is a grouped declaration the pattern cannot
	// handle. For example: CmdFoo, CmdBar CommandType = "FOO", "BAR"
	// The regex matches CmdBar but misses CmdFoo, so we catch it by counting
	// how many Cmd* names appear on the line vs. how many the pattern captured.
	lines := strings.Split(protocolSource, "\n")
	cmdNamePattern := regexp.MustCompile(`Cmd\w+`)
	for _, line := range lines {
		// Find all Cmd* names on this line (could be 0, 1, or multiple)
		cmdNames := cmdNamePattern.FindAllString(line, -1)
		if len(cmdNames) == 0 {
			continue
		}

		// Find all regex matches on this line (one match = one captured Cmd* name)
		lineMatches := constPattern.FindAllStringSubmatch(line, -1)

		// If the line has Cmd* names but the regex didn't match any on it, or
		// if the line has multiple Cmd* names but the regex only got one, that
		// is a declaration shape we don't understand.
		if len(cmdNames) > 0 && len(lineMatches) == 0 {
			// Line has Cmd* names but no regex match
			if strings.Contains(line, "CommandType") && strings.Contains(line, "=") {
				t.Errorf("line contains CommandType declaration but was not parsed by the regex: %s",
					strings.TrimSpace(line))
			}
		} else if len(cmdNames) > len(lineMatches) {
			// Line has more Cmd* names than regex matches (grouped declaration)
			if strings.Contains(line, "CommandType") && strings.Contains(line, "=") {
				t.Errorf("line contains %d constant names but regex only matched %d; "+
					"this looks like a grouped declaration the pattern cannot handle: %s",
					len(cmdNames), len(lineMatches), strings.TrimSpace(line))
			}
		}
	}

	// Build a map of declared values for comparison.
	// Store the raw count of matches separately to catch deduplication bugs.
	declaredValues := make(map[string]string) // value -> name, for error reporting
	for _, m := range matches {
		name := m[1]
		value := m[2]
		declaredValues[value] = name
	}
	rawMatchCount := len(matches) // Count of all matches, before deduplication

	// Every declared constant's value must appear in AllCommandTypes.
	for value, name := range declaredValues {
		found := false
		for _, cmd := range AllCommandTypes {
			if string(cmd) == value {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("constant %s (value %q) is declared but not listed in AllCommandTypes", name, value)
		}
	}

	// Every entry in AllCommandTypes must correspond to a declared constant.
	for _, cmd := range AllCommandTypes {
		if _, ok := declaredValues[string(cmd)]; !ok {
			t.Errorf("AllCommandTypes lists %q but no constant with that value is declared in protocol.go",
				string(cmd))
		}
	}

	// Sanity check: the list and source must have the same count.
	// This is layered on the two loops above, not a replacement: the loops catch
	// drift, the count check catches both sides being wrong by the same amount.
	// Compare against raw match count, not deduplicated values, so two constants
	// sharing a value are caught.
	if len(AllCommandTypes) != rawMatchCount {
		t.Errorf("AllCommandTypes has %d entries but protocol.go declares %d constant declarations",
			len(AllCommandTypes), rawMatchCount)
	}
}

// Every command the protocol declares must be documented in both the operator
// documentation and the technical documentation. This catches drifts like the
// one where PANIC was added to the constants and the architecture table but
// nothing told the next person to do that: now, adding a command and forgetting
// a table is caught immediately.
//
// The list is derived from AllCommandTypes, which is published by the protocol
// itself, so this test catches failures at the source.
func TestEveryCommandIsDocumentedInBothPublishedAndTechnicalDocs(t *testing.T) {
	// The published content lives under docs/_docs/ since the Jekyll collection
	// restructure; docs/architecture.md is now a redirect stub and would never
	// contain a command table again.
	archDocs := repoFile(t, "docs", "_docs", "architecture.md")
	techDocs := repoFile(t, "docs-tech", "protocol.md")

	if len(AllCommandTypes) == 0 {
		t.Fatal("AllCommandTypes is empty; the list has not been populated or has been broken")
	}

	for _, cmd := range AllCommandTypes {
		cmdStr := "`" + string(cmd) + "`"
		if !strings.Contains(archDocs, cmdStr) {
			t.Errorf("docs/_docs/architecture.md does not document command %s", cmdStr)
		}
		if !strings.Contains(techDocs, cmdStr) {
			t.Errorf("docs-tech/protocol.md does not document command %s", cmdStr)
		}
	}
}

// AllLoginEvents is the root of the same shape of guard AllCommandTypes has
// carried since 2.7: dispatch knows each, audit-log.md documents each, both
// locales label each. If an entry is deleted from this list, all three stop
// checking it; if an event is added to the const block without being listed, it
// ships unhandled, uncoloured and untranslated.
func TestAllLoginEventsMatchesTheProtocolSource(t *testing.T) {
	src := repoFile(t, "internal", "shared", "protocol.go")

	constPattern := regexp.MustCompile(`(Ev\w+)\s+LoginEvent\s*=\s*"([^"]+)"`)
	matches := constPattern.FindAllStringSubmatch(src, -1)
	if len(matches) == 0 {
		t.Fatal("could not find any LoginEvent constants in protocol.go; the pattern no " +
			"longer matches or the declarations are gone")
	}

	declared := make(map[string]string, len(matches))
	for _, m := range matches {
		declared[m[2]] = m[1]
	}
	for value, name := range declared {
		found := false
		for _, ev := range AllLoginEvents {
			if string(ev) == value {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("constant %s (value %q) is declared but not listed in AllLoginEvents", name, value)
		}
	}
	for _, ev := range AllLoginEvents {
		if _, ok := declared[string(ev)]; !ok {
			t.Errorf("AllLoginEvents lists %q but no constant with that value is declared", string(ev))
		}
	}
	if len(AllLoginEvents) != len(matches) {
		t.Errorf("AllLoginEvents has %d entries but protocol.go declares %d constants",
			len(AllLoginEvents), len(matches))
	}
}

// The submitted username is never echoed into the audit record, and there is no
// free-text field for one to arrive in. The payload carries an event from a
// fixed enum, an address the core parses itself, and an integer.
func TestLogEventPayloadCarriesNoFreeText(t *testing.T) {
	typ := reflect.TypeOf(LogEventPayload{})
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		switch f.Name {
		case "Event":
			if f.Type.Name() != "LoginEvent" {
				t.Errorf("Event is a %s; it must be the LoginEvent enum, or the web process can "+
					"write any sentence it likes into the root process's record", f.Type)
			}
		case "Addr", "Left":
			// Addr goes through netip.ParseAddr in the core; Left is an integer
			// and can smuggle nothing.
		case "Proxied":
			if f.Type.Kind() != reflect.Bool {
				t.Errorf("Proxied is a %s; it must stay a bool derived from a header's presence, "+
					"or the web process can smuggle text through it", f.Type)
			}
		default:
			t.Errorf("LogEventPayload has gained a %s field (%s). If it carries text from the "+
				"web process, it is a way to write arbitrary lines into the audit log",
				f.Type, f.Name)
		}
	}
}
