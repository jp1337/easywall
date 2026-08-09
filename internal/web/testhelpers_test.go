package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/sessions"
	"github.com/jp1337/easywall/internal/shared"
)

// fakeCore is a minimal Unix socket server that returns canned responses.
type fakeCore struct {
	socketPath  string
	listener    net.Listener
	mu          sync.Mutex
	responses   map[shared.CommandType]shared.Response
	defaultResp shared.Response
	lastCmd     *shared.Command
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

// repoTemplates returns the real web/templates directory.
//
// The handler tests used to render one-line stubs from testdata/templates/, which
// meant no test ever exercised the markup that ships — a template naming a class
// that no longer existed, or a fragment built with the wrong variant, was
// invisible to the suite. Rendering the real templates costs nothing here and
// closes that gap.
func repoDir(t *testing.T, parts ...string) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	rel := filepath.Join(parts...)
	for i := 0; i < 5; i++ {
		candidate := filepath.Join(dir, rel)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		dir = filepath.Dir(dir)
	}
	t.Fatalf("could not locate %s above the package directory", rel)
	return ""
}

func repoTemplates(t *testing.T) string { return repoDir(t, "web", "templates") }

// The real locale files too, so a handler test sees the copy that ships rather
// than a bare message id.
func repoLocales(t *testing.T) string { return repoDir(t, "locales") }

// newTestServer creates a Server backed by a fake core for handler testing.
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
# No test may reach for the network. Tests that want the update check switch it
# on themselves, with a cache primed so nothing is fetched.
update_check = false
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

	bundle := NewBundle(repoLocales(t))

	tmpl, err := loadTemplates(repoTemplates(t))
	if err != nil {
		t.Fatalf("loadTemplates: %v", err)
	}

	s := &Server{
		cfg:     cfg,
		client:  client,
		store:   store,
		bundle:  bundle,
		tmpl:    tmpl,
		version: shared.NewChecker(cfg.VersionCachePath(), cfg.UpdateCheckEnabled()),
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
update_check = false
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

	bundle := NewBundle(repoLocales(t))
	tmpl, err := loadTemplates(repoTemplates(t))
	if err != nil {
		t.Fatalf("loadTemplates: %v", err)
	}

	s := &Server{
		cfg:     cfg,
		client:  client,
		store:   store,
		bundle:  bundle,
		tmpl:    tmpl,
		version: shared.NewChecker(cfg.VersionCachePath(), cfg.UpdateCheckEnabled()),
	}
	s.router = s.buildRouter(cfg)
	return s
}

// makeAuthCookie creates a session cookie with admin user logged in.
//
// It carries the credential fingerprint the server is currently running with,
// exactly as a real login would leave it — a session without one is refused,
// which is the whole point of it.
func makeAuthCookie(t *testing.T, s *Server) *http.Cookie {
	t.Helper()
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	sess, err := s.store.Get(req, SessionName)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	sess.Values[SessionUserKey] = "admin"
	sess.Values[SessionCredentialKey] = credentialFingerprint(s.cfg.Password)
	sess.Values[SessionIDKey] = newSessionID()
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
	cookie := makeAuthCookie(t, s)
	return doRequest(s, method, url, body, cookie)
}

// doAuthFormRequest performs an authenticated POST with URL-encoded form data.
func doAuthFormRequest(t *testing.T, s *Server, url, formBody string) *httptest.ResponseRecorder {
	t.Helper()
	cookie := makeAuthCookie(t, s)
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

// successResp creates a success Response with JSON data.
func successResp(data interface{}) shared.Response {
	raw, _ := json.Marshal(data)
	return shared.Response{Success: true, Data: raw}
}

// errorResp creates a failure Response.
func errorRespFor(msg string) shared.Response {
	return shared.Response{Success: false, Error: msg}
}
