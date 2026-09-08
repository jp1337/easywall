package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jp1337/easywall/internal/shared"
)

// healthOK is a GET_HEALTH reply a fake core can hand back.
func healthReply(t *testing.T, res shared.HealthResult) shared.Response {
	t.Helper()
	data, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	return shared.Response{Success: true, Data: data}
}

// doRequestFrom is doRequest with the TCP peer chosen by the test, which is the
// whole subject of this file: /healthz decides on the peer and on nothing else.
func doRequestFrom(s *Server, peer, url string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", url, nil)
	req.RemoteAddr = peer
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	return rec
}

// The gate reads the peer address, never a forwarding header.
//
// docs-tech/threat-model.md refuses X-Forwarded-For for the routes that matter,
// and an address that decides whether an unauthenticated endpoint answers at
// all is one of them. 2.13's resolveClient walk exists so the audit log can
// record who logged in; it is not an authorisation mechanism, and reusing it
// here would let anything behind a trusted proxy claim to be loopback by
// writing one header.
//
// Both directions, because either alone passes for the wrong reason: the first
// case would go green on a gate that ignored the list entirely, and the second
// on a gate that refused everything.
func TestHealthzIgnoresForwardingHeaders(t *testing.T) {
	t.Run("a forged loopback header from a remote peer is still refused", func(t *testing.T) {
		fc := newFakeCore(t)
		fc.SetResponse(shared.CmdGetHealth, healthReply(t, shared.HealthResult{
			State: shared.HealthOK, Reason: shared.HealthReasonHealthy,
		}))
		s := newTestServer(t, fc)
		// The peer is named as a trusted proxy, which is the configuration that
		// makes resolveClient believe its header. The gate must not care.
		s.cfg.TrustedProxies = []string{"203.0.113.9"}

		rec := doRequestFrom(s, "203.0.113.9:44321", "/healthz",
			map[string]string{"X-Forwarded-For": "127.0.0.1"})
		if rec.Code != http.StatusNotFound {
			t.Fatalf("X-Forwarded-For: 127.0.0.1 from a remote peer answered %d, want 404 — "+
				"the gate believed a header instead of the peer, and anything behind a "+
				"trusted proxy can now read /healthz", rec.Code)
		}
	})

	t.Run("a real loopback peer is not talked out of it by a header", func(t *testing.T) {
		fc := newFakeCore(t)
		fc.SetResponse(shared.CmdGetHealth, healthReply(t, shared.HealthResult{
			State: shared.HealthOK, Reason: shared.HealthReasonHealthy,
		}))
		s := newTestServer(t, fc)
		// Loopback listed as a trusted proxy is the container's own shape, and
		// resolveClient would hand back 203.0.113.9 here.
		s.cfg.TrustedProxies = []string{"127.0.0.1"}

		rec := doRequestFrom(s, "127.0.0.1:44322", "/healthz",
			map[string]string{"X-Forwarded-For": "203.0.113.9"})
		if rec.Code != http.StatusOK {
			t.Fatalf("a loopback peer answered %d, want 200 — a header moved the gate's "+
				"idea of who was asking", rec.Code)
		}
	})
}

// The default. An unauthenticated endpoint on a firewall's administration
// interface starts closed to everything but the loopback the container's own
// health check and a local monitoring probe both come from.
func TestHealthzIsLoopbackOnlyByDefault(t *testing.T) {
	got := shared.WebDefault().HealthAllow
	if len(got) != 2 {
		t.Fatalf("config/web.toml ships health_allow = %v, want two loopback entries", got)
	}
	for _, want := range []string{"127.0.0.1/8", "::1/128"} {
		if !slices.Contains(got, want) {
			t.Errorf("health_allow does not carry %q", want)
		}
	}
}

// An operator upgrading from 2.16 has a web.toml with no health_allow line in
// it, and the container's HEALTHCHECK asks this endpoint. Absent must therefore
// mean the loopback default and not "closed", or the upgrade itself reports the
// container unhealthy — the exact failure docker/entrypoint.sh:12-18 records,
// arriving from the other direction.
func TestHealthzAnswersOnLoopbackWhenTheConfigNeverMentionsIt(t *testing.T) {
	fc := newFakeCore(t)
	fc.SetResponse(shared.CmdGetHealth, healthReply(t, shared.HealthResult{
		State: shared.HealthOK, Reason: shared.HealthReasonHealthy,
	}))
	// newTestServer writes a web.toml with no health_allow key at all.
	s := newTestServer(t, fc)
	if s.cfg.fileConfig.HealthAllow != nil {
		t.Fatalf("this test needs a config file that never mentions health_allow, got %v",
			s.cfg.fileConfig.HealthAllow)
	}
	if rec := doRequestFrom(s, "127.0.0.1:44330", "/healthz", nil); rec.Code != http.StatusOK {
		t.Fatalf("a config with no health_allow answered %d on loopback, want 200", rec.Code)
	}
}

// A malformed entry stops startup and names its own setting.
//
// This is the whole reason ValidateAddressList takes the key rather than being
// copied per list: health_allow has trusted_proxies' grammar and a different
// meaning, and a message naming trusted_proxies for a health_allow entry sends
// an operator to the wrong line of their web.toml.
func TestAMalformedHealthAllowEntryNamesHealthAllow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "web.toml")
	body := `bind_addr    = "127.0.0.1:12227"
socket_path  = "` + dir + `/core.sock"
ssl_dir      = "` + dir + `/ssl"
data_dir     = "` + dir + `"
session_key  = "test-session-key-32bytes-padding!"
health_allow = ["127.0.0.1/8", "10.20.0.0.0/24"]
`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	err = cfg.Validate()
	if err == nil {
		t.Fatal("Validate accepted health_allow = [… \"10.20.0.0.0/24\"]; a typo would " +
			"silently leave the list one entry short, and the operator meets it as a " +
			"monitoring check that has been getting 404 since the upgrade")
	}
	if !strings.Contains(err.Error(), "health_allow") {
		t.Errorf("the error does not name health_allow: %v", err)
	}
	if strings.Contains(err.Error(), "trusted_proxies") {
		t.Errorf("the error blames trusted_proxies for a health_allow entry: %v", err)
	}
	if !strings.Contains(err.Error(), "10.20.0.0.0/24") {
		t.Errorf("the error does not quote the entry that is wrong: %v", err)
	}
}

// 404 rather than 403. Not confirming that an endpoint exists is the better
// default here, and it costs nothing: whoever is allowed gets the real answer.
func TestHealthzAnswers404FromAnAddressNotOnTheList(t *testing.T) {
	fc := newFakeCore(t)
	fc.SetResponse(shared.CmdGetHealth, healthReply(t, shared.HealthResult{
		State: shared.HealthOK, Reason: shared.HealthReasonHealthy,
	}))
	s := newTestServer(t, fc)

	rec := doRequestFrom(s, "198.51.100.7:5000", "/healthz", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("an address off the list answered %d, want 404", rec.Code)
	}
	// And it must not leak the answer in the body of whatever it did return.
	if body := rec.Body.String(); strings.Contains(body, `"state"`) {
		t.Errorf("the refusal carried a health body:\n%s", body)
	}
}

// A list explicitly emptied closes the endpoint entirely, which is the only way
// an operator has of saying so. Distinct from absent — see
// TestHealthzAnswersOnLoopbackWhenTheConfigNeverMentionsIt.
func TestHealthzAnEmptyListClosesItEvenToLoopback(t *testing.T) {
	fc := newFakeCore(t)
	fc.SetResponse(shared.CmdGetHealth, healthReply(t, shared.HealthResult{
		State: shared.HealthOK, Reason: shared.HealthReasonHealthy,
	}))
	s := newTestServer(t, fc)
	s.cfg.HealthAllow = []string{}

	if rec := doRequestFrom(s, "127.0.0.1:44331", "/healthz", nil); rec.Code != http.StatusNotFound {
		t.Errorf("health_allow = [] answered %d on loopback, want 404", rec.Code)
	}
}

func TestHealthzAnswers200AndOkFromLoopback(t *testing.T) {
	fc := newFakeCore(t)
	fc.SetResponse(shared.CmdGetHealth, healthReply(t, shared.HealthResult{
		State:  shared.HealthOK,
		Reason: shared.HealthReasonHealthy,
		Selftest: shared.HealthSelftest{
			Version: "2.17.0", Kernel: "6.1.0-test", Result: shared.SelftestPassed,
		},
	}))
	s := newTestServer(t, fc)

	rec := doRequestFrom(s, "127.0.0.1:44323", "/healthz", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("loopback answered %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json — this answers a machine", ct)
	}
	var got shared.HealthResult
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("the body does not parse as a HealthResult: %v\n%s", err, rec.Body.String())
	}
	if got.State != shared.HealthOK || got.Reason != shared.HealthReasonHealthy {
		t.Errorf("body = %+v, want state ok and reason healthy", got)
	}
	if got.Selftest.Kernel != "6.1.0-test" || got.Selftest.Result != shared.SelftestPassed {
		t.Errorf("the self-test stamp did not survive: %+v", got.Selftest)
	}
	// The command really went to the core, rather than the handler inventing an
	// answer that happens to match.
	if last := fc.LastCommand(); last == nil || last.Type != shared.CmdGetHealth {
		t.Errorf("the core was asked %v, want GET_HEALTH", last)
	}
}

// degraded answers 200 on purpose. A degraded firewall is still filtering, and
// a Docker HEALTHCHECK that marked the container unhealthy for it would restart
// a working firewall. The state is in the body for whoever wants to alert on it.
func TestHealthzAnswers200ForDegradedWithTheStateInTheBody(t *testing.T) {
	fc := newFakeCore(t)
	fc.SetResponse(shared.CmdGetHealth, healthReply(t, shared.HealthResult{
		State: shared.HealthDegraded, Reason: shared.HealthReasonBuildFindings,
	}))
	s := newTestServer(t, fc)

	rec := doRequestFrom(s, "127.0.0.1:44324", "/healthz", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("degraded answered %d, want 200 — a degraded firewall is filtering, and "+
			"restarting the container for it restarts a working firewall", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"state":"degraded"`) {
		t.Errorf(`the body does not carry "state":"degraded", so nothing can alert on it:`+"\n%s", body)
	}
}

func TestHealthzAnswers503ForFail(t *testing.T) {
	fc := newFakeCore(t)
	fc.SetResponse(shared.CmdGetHealth, healthReply(t, shared.HealthResult{
		State: shared.HealthFail, Reason: shared.HealthReasonNotEnforcing,
	}))
	s := newTestServer(t, fc)

	rec := doRequestFrom(s, "127.0.0.1:44325", "/healthz", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("fail answered %d, want 503", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"state":"fail"`) {
		t.Errorf(`the body does not carry "state":"fail":`+"\n%s", body)
	}
}

// The case the endpoint exists for. docker/entrypoint.sh:12-18 records it: the
// core was alive and the web process was dead, and a check that only looked at
// the socket reported the container Up. Here it is the other half — the web
// process answers, and says so, when the core is the one that is gone.
func TestHealthzAnswers503WhenTheCoreDoesNotAnswer(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc)
	s.client = NewCoreClient(t.TempDir() + "/there-is-no-core.sock")

	rec := doRequestFrom(s, "127.0.0.1:44326", "/healthz", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("an unreachable core answered %d, want 503: %s", rec.Code, rec.Body.String())
	}
	var got shared.HealthResult
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("the body does not parse as a HealthResult: %v\n%s", err, rec.Body.String())
	}
	if got.State != shared.HealthFail {
		t.Errorf("state = %q, want fail: a core that cannot be reached cannot be shown to "+
			"be enforcing anything", got.State)
	}
}

// It is outside the session middleware, and it creates nothing.
//
// No cookie, no redirect to /login, and a 200 without any credential — an
// orchestrator holds no session, which is the reason this route exists at all.
func TestHealthzNeedsNoSessionAndSetsNoCookie(t *testing.T) {
	fc := newFakeCore(t)
	fc.SetResponse(shared.CmdGetHealth, healthReply(t, shared.HealthResult{
		State: shared.HealthOK, Reason: shared.HealthReasonHealthy,
	}))
	s := newTestServer(t, fc)

	rec := doRequestFrom(s, "127.0.0.1:44327", "/healthz", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("an unauthenticated request answered %d, want 200 — the route is behind "+
			"authentication, and no orchestrator can get past it: %s",
			rec.Code, rec.Body.String())
	}
	if got := rec.Result().Cookies(); len(got) != 0 {
		t.Errorf("/healthz set %d cookie(s): %v", len(got), got)
	}
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Errorf("/healthz redirected to %q", loc)
	}
}

// An empty kernel is rendered as nothing rather than as an empty label.
//
// The demo answers with an unprovable stamp whose Kernel is the zero value:
// internal/web has no path to the privileged core.KernelRelease(), by design.
// On a real host an unprovable stamp does carry a kernel, so the demo differs
// from production, and the JSON must not offer an empty field where a kernel
// belongs — a dashboard reading it would print "Kernel:" followed by nothing.
func TestHealthzOmitsAnEmptyKernel(t *testing.T) {
	fc := newFakeCore(t)
	fc.SetResponse(shared.CmdGetHealth, healthReply(t, shared.HealthResult{
		State:  shared.HealthOK,
		Reason: shared.HealthReasonHealthy,
		Selftest: shared.HealthSelftest{
			Version: shared.CurrentVersion, Result: shared.SelftestUnprovable,
		},
	}))
	s := newTestServer(t, fc)

	rec := doRequestFrom(s, "127.0.0.1:44328", "/healthz", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("answered %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); strings.Contains(body, `"kernel"`) {
		t.Errorf("the body carries an empty kernel field:\n%s", body)
	}
	// And the fields that do have a value are still there, so the assertion
	// above is not passing because the stamp went missing altogether.
	if body := rec.Body.String(); !strings.Contains(body, `"result":"unprovable"`) {
		t.Errorf("the stamp itself is gone, so the kernel check above proves nothing:\n%s", body)
	}
}

// The detail string stays out. SelftestStamp.Detail names a port number and the
// claim that failed — "an open port accepts a connection — port 12227 was
// dropped" — and this endpoint is unauthenticated by necessity. Asserted
// against the reply the core's own type produces rather than against a
// hand-built one, so a field added back to HealthSelftest turns this red here
// as well as in shared.
func TestHealthzPublishesNoRuleDetail(t *testing.T) {
	fc := newFakeCore(t)
	fc.SetResponse(shared.CmdGetHealth, healthReply(t, shared.HealthResult{
		State:  shared.HealthDegraded,
		Reason: shared.HealthReasonSelftestFailed,
		Selftest: shared.HealthSelftest{
			Version: "2.17.0", Kernel: "6.1.0-test", Result: shared.SelftestFailed,
		},
	}))
	s := newTestServer(t, fc)

	rec := doRequestFrom(s, "127.0.0.1:44329", "/healthz", nil)
	var generic map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &generic); err != nil {
		t.Fatalf("the body does not parse: %v\n%s", err, rec.Body.String())
	}
	st, _ := generic["selftest"].(map[string]any)
	if st == nil {
		t.Fatalf("no selftest object in the body:\n%s", rec.Body.String())
	}
	if _, present := st["detail"]; present {
		t.Error("/healthz publishes selftest.detail to anyone who can reach it")
	}
}
