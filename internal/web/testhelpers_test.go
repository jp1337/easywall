package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/sessions"
	"github.com/jp1337/easywall/internal/shared"
)

// fakeCore is a minimal Unix socket server that returns canned responses.
type fakeCore struct {
	socketPath string
	listener   net.Listener
	mu         sync.Mutex
	responses  map[shared.CommandType]shared.Response
	defaultResp shared.Response
	lastCmd    *shared.Command
}

func newFakeCore(t *testing.T) *fakeCore {
	t.Helper()
	dir := t.TempDir()
	socketPath := dir + "/core.sock"

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("fakeCore listen: %v", err)
	}

	fc := &fakeCore{
		socketPath:  socketPath,
		listener:    ln,
		responses:   make(map[shared.CommandType]shared.Response),
		defaultResp: shared.Response{Success: true},
	}
	go fc.serve()
	t.Cleanup(func() { ln.Close() })
	return fc
}

func (fc *fakeCore) SetResponse(cmdType shared.CommandType, resp shared.Response) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.responses[cmdType] = resp
}

func (fc *fakeCore) SetDefaultResponse(resp shared.Response) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.defaultResp = resp
}

func (fc *fakeCore) LastCommand() *shared.Command {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if fc.lastCmd == nil {
		return nil
	}
	cp := *fc.lastCmd
	return &cp
}

func (fc *fakeCore) serve() {
	for {
		conn, err := fc.listener.Accept()
		if err != nil {
			return
		}
		go fc.handleConn(conn)
	}
}

func (fc *fakeCore) handleConn(conn net.Conn) {
	defer conn.Close()

	data, err := io.ReadAll(io.LimitReader(conn, 1<<20))
	if err != nil {
		return
	}

	var cmd shared.Command
	if err := json.Unmarshal(data, &cmd); err != nil {
		resp := shared.Response{Success: false, Error: "invalid JSON"}
		out, _ := json.Marshal(resp)
		_, _ = conn.Write(out)
		return
	}

	fc.mu.Lock()
	fc.lastCmd = &cmd
	resp, ok := fc.responses[cmd.Type]
	if !ok {
		resp = fc.defaultResp
	}
	fc.mu.Unlock()

	out, _ := json.Marshal(resp)
	_, _ = conn.Write(out)
}

// defaultStatus returns a typical FirewallStatus response data.
func defaultStatusData(t *testing.T) json.RawMessage {
	t.Helper()
	status := shared.FirewallStatus{
		Active:     true,
		Acceptance: shared.AcceptanceIdle,
		HasPending: false,
		LastApply:  "",
	}
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("marshal status: %v", err)
	}
	return data
}

// defaultRulesData returns a typical RulesState response data.
func defaultRulesData(t *testing.T) json.RawMessage {
	t.Helper()
	state := shared.RulesState{
		Current: shared.Rules{
			TCP: []shared.PortRule{{Port: "22", Description: "SSH", SSH: true}},
			UDP: []shared.PortRule{},
			Blacklist: []string{},
			Whitelist: []string{},
			Forwarding: []shared.ForwardingRule{},
			Custom: []string{},
		},
		Staged: shared.Rules{
			TCP: []shared.PortRule{{Port: "22", Description: "SSH", SSH: true}},
			UDP: []shared.PortRule{},
			Blacklist: []string{"192.168.1.100"},
			Whitelist: []string{"10.0.0.1"},
			Forwarding: []shared.ForwardingRule{{Protocol: "tcp", SourcePort: 8080, DestPort: 80}},
			Custom: []string{"# custom rule"},
		},
		Backup: shared.Rules{
			TCP: []shared.PortRule{},
			UDP: []shared.PortRule{},
			Blacklist: []string{},
			Whitelist: []string{},
			Forwarding: []shared.ForwardingRule{},
			Custom: []string{},
		},
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal rules: %v", err)
	}
	return data
}

// defaultOptionsData returns a FirewallOptions response data.
func defaultOptionsData(t *testing.T) json.RawMessage {
	t.Helper()
	opts := shared.FirewallOptions{
		SSHBruteForce: true,
		ICMPFlood:     true,
	}
	data, err := json.Marshal(opts)
	if err != nil {
		t.Fatalf("marshal options: %v", err)
	}
	return data
}

// newTestServer creates a Server backed by a fake core for handler testing.
// Templates are loaded from testdata/templates/.
func newTestServer(t *testing.T, fc *fakeCore) *Server {
	t.Helper()

	dir := t.TempDir()
	sslDir := dir + "/ssl"
	if err := os.MkdirAll(sslDir, 0750); err != nil {
		t.Fatalf("create ssl dir: %v", err)
	}

	// Write a minimal web.toml so SaveCredentials works
	cfgPath := dir + "/web.toml"
	_ = os.WriteFile(cfgPath, []byte(`
bind_addr = "127.0.0.1:19999"
socket_path = "`+fc.socketPath+`"
ssl_dir = "`+sslDir+`"
data_dir = "`+dir+`"
session_key = "test-session-key-32bytes-padding!"
language = "en"
username = "admin"
password = ""
[tls]
cert = ""
key  = ""
`), 0600)

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	// Set a real hashed password for login tests
	hash, err := HashPassword("testpassword123!")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	cfg.Password = hash

	client := NewCoreClient(fc.socketPath)

	store := sessions.NewCookieStore([]byte("test-session-key-32bytes-padding!"))
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   SessionLifetime,
		HttpOnly: true,
		Secure:   false, // allow non-HTTPS in tests
		SameSite: http.SameSiteLaxMode,
	}

	bundle := NewBundle("locales")

	tmpl, err := loadTemplates("testdata/templates")
	if err != nil {
		t.Fatalf("loadTemplates: %v", err)
	}

	s := &Server{
		cfg:    cfg,
		client: client,
		store:  store,
		bundle: bundle,
		tmpl:   tmpl,
	}
	s.router = s.buildRouter(cfg)
	return s
}

// newFirstRunTestServer creates a Server in first-run mode (no password set).
func newFirstRunTestServer(t *testing.T, fc *fakeCore) *Server {
	t.Helper()

	dir := t.TempDir()
	sslDir := dir + "/ssl"
	_ = os.MkdirAll(sslDir, 0750)

	cfgPath := dir + "/web.toml"
	_ = os.WriteFile(cfgPath, []byte(`
bind_addr = "127.0.0.1:19999"
socket_path = "`+fc.socketPath+`"
ssl_dir = "`+sslDir+`"
data_dir = "`+dir+`"
session_key = "test-session-key-32bytes-padding!"
language = "en"
username = ""
password = ""
[tls]
cert = ""
key  = ""
`), 0600)

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	client := NewCoreClient(fc.socketPath)

	store := sessions.NewCookieStore([]byte("test-session-key-32bytes-padding!"))
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   SessionLifetime,
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
	}

	bundle := NewBundle("locales")
	tmpl, err := loadTemplates("testdata/templates")
	if err != nil {
		t.Fatalf("loadTemplates: %v", err)
	}

	s := &Server{
		cfg:    cfg,
		client: client,
		store:  store,
		bundle: bundle,
		tmpl:   tmpl,
	}
	s.router = s.buildRouter(cfg)
	return s
}

// makeAuthCookie creates a session cookie with admin user logged in.
func makeAuthCookie(t *testing.T, store sessions.Store) *http.Cookie {
	t.Helper()
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	sess, err := store.Get(req, SessionName)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	sess.Values[SessionUserKey] = "admin"
	if err := sess.Save(req, rec); err != nil {
		t.Fatalf("sess.Save: %v", err)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("no session cookie set")
	}
	return cookies[0]
}

// doRequest performs a request against the server's router and returns the response.
func doRequest(s *Server, method, url string, body io.Reader, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, url, body)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	return rec
}

// remoteAddrCounter is used to generate unique IPs for rate-limit isolation.
var remoteAddrCounter int

// doFormRequest performs a POST (or given method) with URL-encoded form data and Content-Type set.
// Uses a unique RemoteAddr to avoid interference from the global login rate limiter.
func doFormRequest(s *Server, method, url, formBody string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, url, strings.NewReader(formBody))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Use unique IP per request to prevent rate limiter interference across tests
	remoteAddrCounter++
	req.RemoteAddr = fmt.Sprintf("10.%d.%d.%d:12345",
		(remoteAddrCounter/65025)%256,
		(remoteAddrCounter/255)%256,
		remoteAddrCounter%255+1)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	return rec
}

// doAuthRequest performs an authenticated request (with session cookie).
func doAuthRequest(t *testing.T, s *Server, method, url string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	cookie := makeAuthCookie(t, s.store)
	return doRequest(s, method, url, body, cookie)
}

// doAuthFormRequest performs an authenticated POST with URL-encoded form data.
func doAuthFormRequest(t *testing.T, s *Server, url, formBody string) *httptest.ResponseRecorder {
	t.Helper()
	cookie := makeAuthCookie(t, s.store)
	return doFormRequest(s, "POST", url, formBody, cookie)
}

// assertRedirect checks that the response is a redirect to the expected location.
func assertRedirect(t *testing.T, rec *httptest.ResponseRecorder, expectedLoc string) {
	t.Helper()
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != expectedLoc {
		t.Errorf("expected redirect to %q, got %q", expectedLoc, loc)
	}
}

// assertStatus checks the response status code.
func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, expected int) {
	t.Helper()
	if rec.Code != expected {
		t.Errorf("expected status %d, got %d", expected, rec.Code)
	}
}

// newFormBody returns an io.Reader for URL-encoded form data.
func newFormBody(data string) io.Reader {
	return strings.NewReader(data)
}

// newFormRequest creates a POST request with URL-encoded form data.
func newFormRequest(method, url, body string) *http.Request {
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

// successResp creates a success Response with JSON data.
func successResp(data interface{}) shared.Response {
	raw, _ := json.Marshal(data)
	return shared.Response{Success: true, Data: raw}
}

// errorResp creates a failure Response.
func errorRespFor(msg string) shared.Response {
	return shared.Response{Success: false, Error: msg}
}
