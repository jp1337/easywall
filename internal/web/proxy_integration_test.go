//go:build integration

package web

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// The trusted-proxy list, measured with a peer address the kernel assigned.
//
// A unit test sets r.RemoteAddr on a request it built, and that field is
// exactly the one thing a real request does not get from the caller — so a
// mistake in which branch reads it is invisible there. Here the address comes
// from a veth pair: the client is 10.77.1.2 inside its own namespace and the
// server is 10.77.1.1 on this side, and every assertion below is about what the
// server saw on a socket rather than what a test wrote into a struct.
//
// Spec §4 lists the four things this owes, and §4.4 says a test whose failure
// mode has not been observed does not count as written. The mutation table in
// the plan's Step 6 is that observation.

func TestMain(m *testing.M) {
	if os.Getenv("EASYWALL_IN_NETNS") != "" {
		// Loopback comes up DOWN in a fresh namespace, and four test files in
		// this package listen on 127.0.0.1. Without this the whole web suite
		// fails inside the namespace with "connect: network is unreachable" —
		// which reads as a broken test rather than as a missing interface.
		_ = exec.Command("ip", "link", "set", "lo", "up").Run()
		os.Exit(m.Run())
	}

	cmd := exec.Command("/proc/self/exe", os.Args[1:]...)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	cmd.Env = append(os.Environ(), "EASYWALL_IN_NETNS=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Cloneflags: syscall.CLONE_NEWNET}
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1) // CLONE_NEWNET needs CAP_SYS_ADMIN
	}
}

// peerHarness is one namespace on the far end of a veth pair. Narrower than
// internal/core's router, which routes between two of them: nothing here is
// forwarded, and the only thing needed is a client whose source address this
// process did not choose.
type peerHarness struct {
	t    *testing.T
	pid  string
	cmd  *exec.Cmd
	near string // the veth end on this side
	far  string // the end moved into the namespace
	port string // the near-side server's port, set by serve
}

const (
	harnessClient = "10.77.1.2"
	harnessServer = "10.77.1.1"
)

// One name pair per harness, because a shared pair is a race: cleanup kills the
// namespace holder and namespace teardown is asynchronous, so the next
// `ip link add` can still find the old end and fail with EEXIST. Seven
// harnesses are created per run. Well inside the kernel's 15-character limit.
var harnessSeq atomic.Int64

func newPeerHarness(t *testing.T) *peerHarness {
	t.Helper()
	for _, bin := range []string{"unshare", "nsenter", "ip", "bash", "timeout"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("skipping: %s is not installed, and this test opens a real TCP connection", bin)
		}
	}

	h := &peerHarness{t: t}
	cmd := exec.Command("unshare", "-n", "sleep", "120")
	if err := cmd.Start(); err != nil {
		t.Fatalf("cannot create a network namespace: %v", err)
	}
	h.cmd = cmd
	h.pid = strconv.Itoa(cmd.Process.Pid)

	n := harnessSeq.Add(1)
	h.near, h.far = fmt.Sprintf("vp%d-r", n), fmt.Sprintf("vp%d", n)

	// Armed before anything can fail, because everything below it can: a
	// t.Fatalf between here and the registration would Goexit past it and
	// orphan the namespace holder for its full two minutes. Best-effort
	// deletion, unlike the setup calls — the interface may not exist yet, and
	// the near-side end outlives the namespace when it does.
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		_ = exec.Command("ip", "link", "del", h.near).Run()
	})

	ready := false
	deadline := time.Now().Add(2 * time.Second)
	for !ready && time.Now().Before(deadline) {
		if _, err := os.Stat("/proc/" + h.pid + "/ns/net"); err == nil {
			ready = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ready {
		t.Fatalf("/proc/%s/ns/net did not appear within 2s: the namespace holder "+
			"never got its own network namespace", h.pid)
	}

	h.run("ip", "link", "add", h.far, "type", "veth", "peer", "name", h.near)
	h.run("ip", "link", "set", h.far, "netns", h.pid)
	h.run("ip", "addr", "add", harnessServer+"/24", "dev", h.near)
	h.run("ip", "link", "set", h.near, "up")
	h.inNS("ip", "addr", "add", harnessClient+"/24", "dev", h.far)
	h.inNS("ip", "link", "set", h.far, "up")
	return h
}

// run fails the test rather than skipping it. By the time this is reached
// TestMain has already created a CLONE_NEWNET namespace, so CAP_SYS_ADMIN is
// present and an `ip` error is a broken harness, not an environment that cannot
// run the proof. Nothing in CI inspects for SKIP, so a harness that skips on
// setup failure is a green job with the proof never executed — the failure this
// whole task exists to remove, moved from the workflow into the test.
func (h *peerHarness) run(args ...string) {
	h.t.Helper()
	if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
		h.t.Fatalf("harness setup failed: %s: %v: %s", strings.Join(args, " "), err, out)
	}
}

func (h *peerHarness) inNS(args ...string) {
	h.t.Helper()
	h.run(append([]string{"nsenter", "-t", h.pid, "-n"}, args...)...)
}

// get issues one HTTP request from inside the namespace and returns the whole
// response. bash's /dev/tcp rather than curl, which is not installed on every
// runner; plain HTTP rather than TLS for the same reason — this test is about
// the peer address, and TLS would add a dependency without adding a claim.
//
// The request goes in as printf's *format string*, which is what turns the
// literal \r\n sequences below into a real CRLF. Nothing here may contain a
// percent sign for the same reason.
func (h *peerHarness) get(path string, headers ...string) string {
	h.t.Helper()
	req := "GET " + path + " HTTP/1.0\\r\\nHost: " + harnessServer + "\\r\\n"
	for _, hdr := range headers {
		req += hdr + "\\r\\n"
	}
	req += "\\r\\n"

	script := fmt.Sprintf(
		"exec 3<>/dev/tcp/%s/%s; printf '%s' >&3; cat <&3",
		harnessServer, h.port, req)
	out, err := exec.Command("nsenter", "-t", h.pid, "-n",
		"timeout", "5", "bash", "-c", script).CombinedOutput()
	if err != nil {
		h.t.Fatalf("request from the namespace failed: %v: %s", err, out)
	}
	return string(out)
}

// serve starts an HTTP server on the near side of the veth and records the port
// the harness's client should reach it on.
//
// httptest builds the server and then has its listener swapped, because
// httptest binds 127.0.0.1 and the whole point here is an address the client
// reaches over a wire.
func (h *peerHarness) serve(handler http.Handler) {
	h.t.Helper()
	ln, err := net.Listen("tcp", harnessServer+":0")
	if err != nil {
		h.t.Fatalf("cannot listen on %s: %v", harnessServer, err)
	}
	srv := httptest.NewUnstartedServer(handler)
	_ = srv.Listener.Close()
	srv.Listener = ln
	srv.Start()
	h.t.Cleanup(srv.Close)
	_, h.port, _ = net.SplitHostPort(ln.Addr().String())
}

// resolveHandler answers with what the server resolved, so the assertion is
// about a value computed from a real socket rather than about a page.
func resolveHandler(trusted []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		addr, proxied := resolveClient(r, trusted)
		fmt.Fprintf(w, "client=%s proxied=%v", addr, proxied)
	})
}

// §4.1 — a header from a peer that is not on the list changes nothing.
func TestIntegration_AnUntrustedPeerCannotChooseItsAddress(t *testing.T) {
	h := newPeerHarness(t)
	// 10.77.1.2 is the peer; the list names somebody else entirely.
	h.serve(resolveHandler([]string{"192.0.2.1"}))

	body := h.get("/", "X-Forwarded-For: 198.51.100.1")
	if !strings.Contains(body, "client="+harnessClient) {
		t.Errorf("the server resolved a client other than the real peer:\n%s", body)
	}
	if !strings.Contains(body, "proxied=true") {
		t.Errorf("an untrusted header's presence did not mark the request:\n%s", body)
	}
}

// §4.2 — a header from a peer on the list resolves to the client.
func TestIntegration_ATrustedPeerResolvesToTheClient(t *testing.T) {
	h := newPeerHarness(t)
	h.serve(resolveHandler([]string{harnessClient}))

	body := h.get("/", "X-Forwarded-For: 198.51.100.1")
	if !strings.Contains(body, "client=198.51.100.1") {
		t.Errorf("a trusted peer's header was not believed:\n%s", body)
	}
	if !strings.Contains(body, "proxied=false") {
		t.Errorf("the resolved client is the caller's own address and was still "+
			"marked via-proxy:\n%s", body)
	}
}

// §4.3 — naming a trusted proxy in the header does not let the caller pick its
// own identity. The rightmost-untrusted rule, against a real peer.
//
// Two chains, because one of them alone does not measure the direction of the
// walk. In "a forged hop to the left" the two directions disagree — the
// rightmost hop is the one the real proxy appended and the leftmost is the one
// the caller wrote — so only a right-to-left walk answers 198.51.100.1. A
// left-to-right walk hands the caller 203.0.113.99, which is a value it picks
// per request and therefore a fresh rate-limit bucket every time.
func TestIntegration_TheCallerCannotNameATrustedProxyAsItself(t *testing.T) {
	for _, tc := range []struct {
		name    string
		trusted []string
		xff     string
	}{
		{
			name:    "a forged hop to the left",
			trusted: []string{harnessClient},
			xff:     "203.0.113.99, 198.51.100.1",
		},
		{
			name:    "a trusted proxy named as the last hop",
			trusted: []string{harnessClient, "10.1.0.9"},
			xff:     "198.51.100.1, 10.1.0.9",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newPeerHarness(t)
			h.serve(resolveHandler(tc.trusted))

			body := h.get("/", "X-Forwarded-For: "+tc.xff)
			if !strings.Contains(body, "client=198.51.100.1") {
				t.Errorf("the walk did not end at the rightmost untrusted hop, so "+
					"the caller chose its own identity:\n%s", body)
			}
		})
	}
}

// §4.1, the limiter's half — the bucket. A rewritten header must not buy a
// fresh budget from an untrusted peer; through a trusted one each client gets
// its own budget, and one client still runs out of it.
//
// The third case is what makes the second one mean anything: six requests from
// distinct clients passing would also be what a limiter that refuses nobody on
// the trusted branch looks like.
func TestIntegration_TheLimiterKeysOnTheResolvedClient(t *testing.T) {
	perRequest := func(i int) string { return fmt.Sprintf("198.51.100.%d", i+1) }
	always := func(int) string { return "198.51.100.7" }

	for _, tc := range []struct {
		name     string
		trusted  []string
		client   func(i int) string
		wantLast string
	}{
		{"untrusted: one budget however the header reads", nil, perRequest, "429"},
		{"trusted: a budget per client", []string{harnessClient}, perRequest, "200"},
		{"trusted: one client, one budget", []string{harnessClient}, always, "429"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetLoginLimiter()
			t.Cleanup(resetLoginLimiter)

			h := newPeerHarness(t)
			resolve := func(r *http.Request) (string, bool) { return resolveClient(r, tc.trusted) }
			h.serve(LoginRateLimit(resolve, nil)(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })))

			var last string
			for i := 0; i < 6; i++ {
				last = h.get("/login", "X-Forwarded-For: "+tc.client(i))
			}
			if !strings.Contains(last, " "+tc.wantLast+" ") {
				t.Errorf("the sixth attempt did not answer %s:\n%s", tc.wantLast, last)
			}
		})
	}
}
