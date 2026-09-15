package core

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// recordingLogHandler keeps every line logged while it is installed, because
// what is under test here is a sentence: the port, the address and the protocol
// an operator has to read to know which service just went dark.
//
// The attributes are rendered into the line rather than discarded. 53/udp
// against 53/tcp is the distinction that killed the reporting host — its
// resolver answers both — and an attribute nothing asserts can be deleted with
// the whole suite staying green, which is how the protocol came to be the one
// output field with no test.
type recordingLogHandler struct{ lines *[]string }

func (h recordingLogHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h recordingLogHandler) Handle(_ context.Context, r slog.Record) error {
	line := r.Message
	r.Attrs(func(a slog.Attr) bool {
		line += " " + a.Key + "=" + a.Value.String()
		return true
	})
	*h.lines = append(*h.lines, line)
	return nil
}
func (h recordingLogHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h recordingLogHandler) WithGroup(string) slog.Handler      { return h }

// applyWithPublishedPorts renders the forward chain over a fixture of published
// ports and returns what the daemon said while doing it.
func applyWithPublishedPorts(t *testing.T, rules shared.Rules,
	cidrs []string, published []publishedPort) []string {
	t.Helper()

	var lines []string
	prevLog := slog.Default()
	slog.SetDefault(slog.New(recordingLogHandler{lines: &lines}))
	t.Cleanup(func() { slog.SetDefault(prevLog) })

	prevDetect := detectPublishedPortsFn
	detectPublishedPortsFn = func([]string, int) []publishedPort { return published }
	t.Cleanup(func() { detectPublishedPortsFn = prevDetect })

	buildForward(t, filteredDocker(), shared.RoutingConfig{Mode: shared.RoutingClosed},
		rules, cidrs)
	return lines
}

// Two bridges, and a resolver published on the first one's gateway with no
// forwarded rule for it. That is the host this release was written for: the
// deny renders once per bridge, the container in the second bridge reaches the
// resolver with its destination in the first and its source outside it, and
// every lookup dies while every external probe stays green.
//
// The assertion is the sentence, not a count: an operator who greps the journal
// for a port number has to find the address beside it, or the line names a
// service they cannot locate.
func TestAPublishedPortWithNoForwardedRuleIsNamed(t *testing.T) {
	lines := applyWithPublishedPorts(t,
		shared.Rules{TCP: []shared.PortRule{
			{Port: "443", Description: "a rule for a different port",
				Scope: shared.ScopeForwarded},
		}},
		[]string{"172.17.0.0/16", "172.18.0.0/16"},
		[]publishedPort{{addr: "172.17.0.1", port: 53, containerPort: 53, proto: "udp"}})

	const want = "53 published on 172.17.0.1 with no forwarded rule"
	for _, line := range lines {
		if !strings.Contains(line, want) {
			continue
		}
		// The protocol is the rest of the answer, not decoration. The reporting
		// host's resolver answered on both, and a line naming the wrong one
		// sends an operator to write a rule that opens the wrong port.
		if !strings.Contains(line, "protocol=udp") {
			t.Fatalf("the line names the port but not the protocol it was published "+
				"on, so the rule it asks for could be the wrong one: %q", line)
		}
		// The remedy, asserted because it is advice: an operator whose rule
		// legitimately restricts the port to the bridge networks must not read
		// this line as an instruction to drop that restriction and open the
		// port to the world. The documentation describes this same sentence.
		if !strings.Contains(line, "sources naming the other bridge networks") {
			t.Fatalf("the remedy offers only a rule with no sources, so the only way "+
				"out it names is opening the port to the world: %q", line)
		}
		return
	}
	t.Fatalf("nothing named the published port the deny just closed.\n"+
		"  want a line containing: %q\n  got: %q", want, lines)
}

// `-p 8080:80`, uncovered. The line has to carry both numbers: the published one
// so the operator recognises the service, and the container one because that is
// the only number a forwarded rule can be written for — the accepts render in
// the forward chain, which runs after Docker's DNAT. A remedy naming 8080 there
// matches nothing, and the port stays dropped with the warning repeating.
func TestARemappedPublishNamesBothPortsAndAsksForTheContainerOne(t *testing.T) {
	lines := applyWithPublishedPorts(t, shared.Rules{},
		[]string{"172.18.0.0/16"},
		[]publishedPort{{addr: "0.0.0.0", port: 8080, containerPort: 80, proto: "tcp"}})

	const want = "8080 published on 0.0.0.0 reaches the container on 80: " +
		"no forwarded rule for 80"
	for _, line := range lines {
		if strings.Contains(line, want) {
			return
		}
	}
	t.Fatalf("the remapped publish was not named with both of its ports.\n"+
		"  want a line containing: %q\n  got: %q", want, lines)
}

// The other half, and the one a mutation lands on: a published port that *has*
// a forwarded rule is working as configured, and a warning about it would be
// the line an operator learns to ignore. Ranges count — the ports editor writes
// "8000:9000" and that rule opens 8080.
func TestAPublishedPortWithARuleIsNotNamed(t *testing.T) {
	for _, tc := range []struct {
		name string
		rule shared.PortRule
		port publishedPort
	}{
		{"exact", shared.PortRule{Port: "53", Scope: shared.ScopeForwarded},
			publishedPort{addr: "172.17.0.1", port: 53, containerPort: 53, proto: "udp"}},
		{"a range", shared.PortRule{Port: "8000:9000", Scope: shared.ScopeForwarded},
			publishedPort{addr: "0.0.0.0", port: 8080, containerPort: 8080, proto: "tcp"}},
		{"scope both", shared.PortRule{Port: "53", Scope: shared.ScopeBoth},
			publishedPort{addr: "172.17.0.1", port: 53, containerPort: 53, proto: "udp"}},
		// `-p 8080:80`. The forward chain sees 80, so 80 is the rule that
		// covers it — and comparing the published number instead warns an
		// operator who has done exactly the right thing, at every apply.
		{"a remapped publish, covered by the container port",
			shared.PortRule{Port: "80", Scope: shared.ScopeForwarded},
			publishedPort{addr: "0.0.0.0", port: 8080, containerPort: 80, proto: "tcp"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rules := shared.Rules{TCP: []shared.PortRule{tc.rule}}
			if tc.port.proto == "udp" {
				rules = shared.Rules{UDP: []shared.PortRule{tc.rule}}
			}
			for _, line := range applyWithPublishedPorts(t, rules,
				[]string{"172.17.0.0/16", "172.18.0.0/16"}, []publishedPort{tc.port}) {
				if strings.Contains(line, "no forwarded rule") {
					t.Fatalf("a published port that is open was reported as closed: %q", line)
				}
			}
		})
	}
}

// A host-scoped rule for the same number opens nothing in the forward chain —
// it renders in the input chain — so the published port is still dropped and
// still has to be named. The protocols are separate for the same reason: the
// reporting host's resolver answered on UDP.
func TestARuleInTheWrongChainOrProtocolStillLeavesThePortNamed(t *testing.T) {
	for _, tc := range []struct {
		name  string
		rules shared.Rules
	}{
		{"host scope", shared.Rules{UDP: []shared.PortRule{{Port: "53"}}}},
		{"the other protocol", shared.Rules{TCP: []shared.PortRule{
			{Port: "53", Scope: shared.ScopeForwarded}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lines := applyWithPublishedPorts(t, tc.rules, []string{"172.17.0.0/16"},
				[]publishedPort{{addr: "172.17.0.1", port: 53, containerPort: 53, proto: "udp"}})
			for _, line := range lines {
				if strings.Contains(line, "53 published on 172.17.0.1") {
					return
				}
			}
			t.Fatalf("a rule that opens nothing in the forward chain silenced the "+
				"warning; got: %q", lines)
		})
	}
}

// A forwarded rule that names sources opens the port to those sources and to
// nobody else: portAcceptRules renders `ip saddr <sources> … accept`, so a
// container in another bridge falls past it into the deny. That is the
// reporting host's failure with a rule in place — the operator has written one
// and believes the port is covered — so it is the case the warning must not be
// silent on.
func TestARuleThatNamesSourcesDoesNotSilenceTheWarning(t *testing.T) {
	lines := applyWithPublishedPorts(t,
		shared.Rules{UDP: []shared.PortRule{{
			Port: "53", Scope: shared.ScopeForwarded, Sources: []string{"10.0.0.0/8"},
		}}},
		[]string{"172.17.0.0/16", "172.18.0.0/16"},
		[]publishedPort{{addr: "172.17.0.1", port: 53, containerPort: 53, proto: "udp"}})

	for _, line := range lines {
		if strings.Contains(line, "53 published on 172.17.0.1") {
			return
		}
	}
	t.Fatalf("a rule that opens the port to 10.0.0.0/8 only silenced the warning, "+
		"while the kernel drops the 172.18.x container that has no rule at all; "+
		"got: %q", lines)
}

// Under published_ports = "open" nothing in the forward chain denies a published
// port, so there is nothing to warn about — and a warning there would be the
// daemon reporting a consequence of a setting nobody has switched on.
func TestNothingIsNamedWhileFilteringIsOff(t *testing.T) {
	var lines []string
	prevLog := slog.Default()
	slog.SetDefault(slog.New(recordingLogHandler{lines: &lines}))
	t.Cleanup(func() { slog.SetDefault(prevLog) })

	prevDetect := detectPublishedPortsFn
	detectPublishedPortsFn = func([]string, int) []publishedPort {
		return []publishedPort{{addr: "172.17.0.1", port: 53, containerPort: 53, proto: "udp"}}
	}
	t.Cleanup(func() { detectPublishedPortsFn = prevDetect })

	buildForward(t, openDocker(), shared.RoutingConfig{Mode: shared.RoutingClosed},
		shared.Rules{}, []string{"172.17.0.0/16"})

	for _, line := range lines {
		if strings.Contains(line, "no forwarded rule") {
			t.Fatalf("published_ports = \"open\" drops nothing, and the daemon "+
				"warned about a port it is not closing: %q", line)
		}
	}
}
