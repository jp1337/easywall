package web

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

// validStepOne is a step-1 form every other check would accept, so a refusal
// can only come from the setup token.
const validStepOne = "username=admin&password=averysecurepass1!&password_confirm=averysecurepass1!&ssh_port=22"

// countHashes replaces firstRunHash for one test and returns the call count.
func countHashes(t *testing.T) *int {
	t.Helper()
	n := 0
	orig := firstRunHash
	firstRunHash = func(p string) (string, error) { n++; return orig(p) }
	t.Cleanup(func() { firstRunHash = orig })
	return &n
}

func pendingCount() int {
	firstRunPending.mu.Lock()
	defer firstRunPending.mu.Unlock()
	return len(firstRunPending.at)
}

// sessionValues decodes the session a response set, the way the next request
// would present it.
func sessionValues(t *testing.T, s *Server, rec *httptest.ResponseRecorder) map[any]any {
	t.Helper()
	req, _ := http.NewRequest("GET", "/firstrun", nil)
	for _, c := range lastCookiePerName(rec.Result().Cookies()) {
		req.AddCookie(c)
	}
	sess, err := s.store.Get(req, SessionName)
	if err != nil {
		t.Fatal(err)
	}
	return sess.Values
}

// Whoever finished /firstrun first owned the firewall (2.22 spec, B), and an
// anonymous POST cost a 64 MiB Argon2id run before anything was proven (X1).
// No token, or the wrong one, must stop the request before either.
func TestFirstRunRefusesAMissingOrWrongSetupToken(t *testing.T) {
	for name, field := range map[string]string{
		"no field":          "",
		"empty":             "&setup_token=",
		"wrong token":       "&setup_token=BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB",
		"a prefix of it":    "&setup_token=ABCDEFGH",
		"not base32":        "&setup_token=hunter2",
		"in the query only": "",
	} {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			prev := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
			t.Cleanup(func() { slog.SetDefault(prev) })

			s := newFirstRunTestServer(t, newFakeCore(t))
			hashes := countHashes(t)
			before := pendingCount()

			path := "/firstrun"
			if name == "in the query only" {
				path += "?setup_token=" + testSetupToken
			}
			rec := doFormRequest(s, "POST", path, validStepOne+field)

			assertRedirect(t, rec, "/firstrun")
			// The refusal line must not match the same grep as the token
			// announcement — otherwise a flood of bad guesses buries the line
			// an operator needs under lines that fail the same "setup token" test.
			if strings.Contains(buf.String(), "setup token") {
				t.Errorf("the refusal log line matches grep 'setup token': %s", buf.String())
			}
			back := doRequest(s, "GET", "/firstrun", nil, lastCookiePerName(rec.Result().Cookies())...)
			if !strings.Contains(back.Body.String(), "That setup token does not match") {
				t.Error("the refused page does not show the message that a restart prints a new one")
			}
			if *hashes != 0 {
				t.Errorf("HashPassword ran %d time(s) for a request without the token", *hashes)
			}
			if got := pendingCount(); got != before {
				t.Errorf("a pending first run was stored: %d entries, was %d", got, before)
			}
			if id, _ := sessionValues(t, s, rec)[firstRunPendingKey].(string); id != "" {
				t.Errorf("the session carries a pending id %q", id)
			}
			if !s.cfg.IsFirstRun() {
				t.Error("an account was created")
			}
		})
	}
}

// Right token → step 2, however it was pasted: the log prints it in groups of
// four, and a copy may lose the spaces or change case.
func TestFirstRunAcceptsTheSetupTokenHoweverItIsPasted(t *testing.T) {
	for name, pasted := range map[string]string{
		"as printed":      formatTOTPSecret(testSetupToken),
		"without spaces":  testSetupToken,
		"lower, with -":   strings.ToLower(strings.ReplaceAll(formatTOTPSecret(testSetupToken), " ", "-")),
		"padded with = ":  testSetupToken + "====",
		"trailing tab":    testSetupToken + "\t",
		"with its quotes": "\"" + formatTOTPSecret(testSetupToken) + "\"",
	} {
		t.Run(name, func(t *testing.T) {
			s := newFirstRunTestServer(t, newFakeCore(t))
			hashes := countHashes(t)

			rec := doFormRequest(s, "POST", "/firstrun",
				validStepOne+"&setup_token="+url.QueryEscape(pasted))

			assertStatus(t, rec, http.StatusOK)
			if !strings.Contains(rec.Body.String(), `class="totp-secret"`) {
				t.Error("the right token did not reach the TOTP step")
			}
			if *hashes != 1 {
				t.Errorf("HashPassword ran %d times, want 1", *hashes)
			}
		})
	}
}

// The token is generated only while no account exists, and printed once with
// the phrase the documentation and CI grep for.
func TestTheSetupTokenExistsOnlyBeforeTheAccount(t *testing.T) {
	for _, withAccount := range []bool{false, true} {
		t.Run(fmt.Sprintf("account=%v", withAccount), func(t *testing.T) {
			var buf bytes.Buffer
			prev := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
			t.Cleanup(func() { slog.SetDefault(prev) })

			dir := t.TempDir()
			path := dir + "/web.toml"
			if err := os.WriteFile(path, []byte(`
bind_addr = "127.0.0.1:19877"
socket_path = "`+dir+`/core.sock"
ssl_dir = "`+dir+`/ssl"
data_dir = "`+dir+`"
session_key = "test-session-key-32bytes-padding!"
update_check = false
`), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := LoadConfig(path)
			if err != nil {
				t.Fatal(err)
			}
			if withAccount {
				cfg.Password = "$argon2id$v=19$m=65536,t=3,p=4$c2FsdHNhbHRzYWx0c2FsdA$aGFzaGhhc2hoYXNoaGFzaGhhc2hoYXNoaGFzaGhhc2g"
			}
			s, err := NewServer(cfg)
			if err != nil {
				t.Fatalf("NewServer: %v", err)
			}

			logged := strings.Contains(buf.String(), "setup token")
			if withAccount {
				if s.setupToken != "" || logged {
					t.Errorf("an installation with an account generated a setup token (%q, logged=%v)", s.setupToken, logged)
				}
				return
			}
			raw, err := decodeTOTPSecret(s.setupToken)
			if err != nil || len(raw) != totpSecretBytes {
				t.Fatalf("setup token %q: %d bytes, err %v; want %d random bytes", s.setupToken, len(raw), err, totpSecretBytes)
			}
			if !logged || !strings.Contains(buf.String(), s.setupToken) {
				t.Errorf("the token was not printed with the phrase \"setup token\":\n%s", buf.String())
			}
		})
	}
}

// The session cookie is signed, not encrypted (threat-model.md), and a flash
// is rendered into the page: the token must reach neither, on any path back.
func TestTheSetupTokenNeverTravelsInTheSessionOrAFlash(t *testing.T) {
	for name, form := range map[string]string{
		"wrong token":            validStepOne + "&setup_token=BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB",
		"right token, bad form":  "username=admin&password=averysecurepass1!&password_confirm=nope" + withSetupToken,
		"right token, good form": validStepOne + withSetupToken,
	} {
		t.Run(name, func(t *testing.T) {
			s := newFirstRunTestServer(t, newFakeCore(t))
			rec := doFormRequest(s, "POST", "/firstrun", form)

			leaks := func(where, text string) {
				for _, spelling := range []string{testSetupToken, formatTOTPSecret(testSetupToken)} {
					if strings.Contains(text, spelling) {
						t.Errorf("the setup token is in %s", where)
					}
				}
			}
			for k, v := range sessionValues(t, s, rec) {
				leaks(fmt.Sprintf("session value %v", k), fmt.Sprint(v))
			}
			leaks("the response body", rec.Body.String())
			back := doRequest(s, "GET", "/firstrun", nil, lastCookiePerName(rec.Result().Cookies())...)
			leaks("the re-rendered wizard", back.Body.String())
		})
	}
}

// A token compared with == leaks, per byte, how much of a guess was right.
// A source guard, because no timing test in CI is stable enough to catch it.
func TestTheSetupTokenIsComparedInConstantTime(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "handler_firstrun.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var fn *ast.FuncDecl
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Name.Name == "setupTokenMatches" {
			fn = fd
		}
	}
	if fn == nil {
		t.Fatal("handler_firstrun.go has no setupTokenMatches")
	}
	literal := func(e ast.Expr) bool {
		if _, ok := e.(*ast.BasicLit); ok {
			return true
		}
		id, ok := e.(*ast.Ident)
		return ok && id.Name == "nil"
	}
	constantTime := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CallExpr:
			if sel, ok := n.Fun.(*ast.SelectorExpr); ok {
				if x, ok := sel.X.(*ast.Ident); ok && x.Name == "subtle" && sel.Sel.Name == "ConstantTimeCompare" {
					constantTime = true
				}
				if x, ok := sel.X.(*ast.Ident); ok && x.Name == "bytes" && sel.Sel.Name == "Equal" {
					t.Errorf("%s: bytes.Equal returns at the first differing byte", fset.Position(n.Pos()))
				}
			}
		case *ast.BinaryExpr:
			if (n.Op == token.EQL || n.Op == token.NEQ) && !literal(n.X) && !literal(n.Y) {
				t.Errorf("%s: the token is compared with %s", fset.Position(n.Pos()), n.Op)
			}
		}
		return true
	})
	if !constantTime {
		t.Error("setupTokenMatches does not call subtle.ConstantTimeCompare")
	}
}
