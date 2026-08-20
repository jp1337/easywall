# A second factor from the first minute — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The first-run wizard can switch a second factor on, optionally, before
the account is written — and a wrong clock can never turn that option into "no
account at all".

**Architecture:** The wizard's existing single write stays a single write. When
the operator ticks the box, step 1 validates and hashes but stores nothing;
everything it collected goes into an in-memory table keyed by a random id held in
the session the wizard already has. Step 2 shows the QR and confirms a code, and
only then does `SaveFirstRun` run — once, carrying the account *and* the factor.
Step 2 also carries an escape hatch that takes today's path exactly.

**Tech Stack:** Go 1.25 (`toolchain go1.26.6`), chi v5, gorilla/sessions,
golang.org/x/crypto/argon2, go-i18n v2, Tailwind, Playwright.

## Global Constraints

- **Nothing is written before a code confirms.** Not the account, not the ports,
  not the IPv6 mode. Step 1 with the box ticked must leave `IsFirstRun()` true
  and `web.toml` untouched.
- **`SaveFirstRun` still writes exactly once**, under the same lock, still
  refusing when a password already exists, still rolling back its in-memory
  fields on a failed write (that rollback landed in the 2.8 final fix wave —
  do not remove it).
- **The escape hatch is not optional.** easywall runs on boards with no RTC. If a
  correct code were the only way past step 2, a flat battery would mean no
  account on a machine already reachable from the network.
- **The password hash and the TOTP secret never enter a cookie.**
  `gorilla/sessions` signs but does not encrypt. Only a random id travels.
- **The new routes live and die with the wizard** — registered only inside the
  existing `if cfg.IsFirstRun()` block, so they stop existing once an account does.
- **Both locales, same change.** Every new key in `locales/en.json` AND
  `locales/de.json`, German never byte-identical to English.
- **Generated files are rebuilt and diffed, never assumed.** After any
  `web/src/app.css` change: `npm run build:css`, then grep `web/static/style.css`.
- **gosec is run standalone by CI and only honours `#nosec`,** not `//nolint`.
  Annotate for both if you suppress anything.
- Commit format: Conventional Commits.

## Deviations from the spec, and why

| Spec said | Actually | Consequence |
|---|---|---|
| "there is no session yet", so the id needs its own `easywall_setup` cookie | The wizard already uses `s.store.Get(r, SessionName)` for flashes and `firstRunKey` | The id goes in that session under its own key. One less cookie, one less `MaxAge` to get wrong |
| The pending state holds username, hash, telemetry | The wizard also collects SSH port, open-web, IPv6 mode, and `applyFirstRunChoices` needs them | The table carries the whole `firstRunData` |

## File Structure

**New**

| File | Responsibility |
|---|---|
| `internal/web/firstrunpending.go` | The table, its id, its sweep. No HTTP. |
| `internal/web/firstrunpending_test.go` | Expiry, isolation, that nothing leaks into the cookie |
| `internal/web/handler_firstrun_2fa_test.go` | The wizard's two new routes end to end |

**Changed**

| File | Change |
|---|---|
| `internal/web/config.go` | `SaveFirstRun` takes `FirstRunAccount` |
| `internal/web/handler_firstrun.go` | The branch, the two handlers, `applyFirstRunChoices` moved behind the confirmation |
| `internal/web/server.go` | Two routes inside the existing `if cfg.IsFirstRun()` block |
| `web/templates/firstrun.html` | The checkbox and two further states |
| `locales/{en,de}.json` | Every new string |
| `docs/_docs/installation/first-run.md` | Rewritten where it says the wizard does not ask |
| `docs/_docs/features/two-factor.md` | One line, linking back |
| `docs/_docs/roadmap.md` | Passkeys as a second factor |
| `docs-tech/invariants.md` | The escape-hatch guard and its reason |
| `docs/assets/img/screens/` | `firstrun-*` retaken, two new pairs |

---

### Task 1: One value for what the wizard decides

A pure refactor with no behaviour change, so the interesting task can be reviewed
without it. `SaveFirstRun` grows from three arguments to five, two of them
optional and two of them adjacent strings — that is a swap waiting to happen, so
it becomes one struct instead.

**Files:**
- Modify: `internal/web/config.go` (`SaveFirstRun`), `internal/web/handler_firstrun.go` (its one caller)
- Test: `internal/web/config_test.go` (existing callers), `internal/web/config_totp_test.go` (the rollback case)

**Interfaces:**
- Produces:
  - `type FirstRunAccount struct { Username, PasswordHash string; Telemetry bool; TOTPSecret string; RecoveryHashes []string }`
  - `func (c *Config) SaveFirstRun(a FirstRunAccount) error`

- [ ] **Step 1: Write the failing test**

Append to `internal/web/config_totp_test.go`:

```go
// The wizard's one write carries the factor too, when there is one. Two values
// that must land together or not at all: a config holding a secret for an
// account that was never created is unreachable, and an account whose factor
// half-landed cannot be signed into.
func TestSaveFirstRun_WritesTheAccountAndTheFactorTogether(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/web.toml"
	if err := os.WriteFile(path, []byte(`
# A comment that has to survive the first run.
session_key = "test-session-key-32bytes-padding!"
username    = ""
password    = ""
`), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveFirstRun(FirstRunAccount{
		Username:       "admin",
		PasswordHash:   "$argon2id$hash",
		Telemetry:      true,
		TOTPSecret:     "JBSWY3DPEHPK3PXP",
		RecoveryHashes: []string{"$argon2id$r1", "$argon2id$r2"},
	}); err != nil {
		t.Fatal(err)
	}

	reloaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("the file this program just wrote does not load: %v", err)
	}
	if reloaded.Username != "admin" || reloaded.Password != "$argon2id$hash" {
		t.Errorf("account came back as %q / %q", reloaded.Username, reloaded.Password)
	}
	if reloaded.TOTPSecret() != "JBSWY3DPEHPK3PXP" {
		t.Errorf("secret came back as %q", reloaded.TOTPSecret())
	}
	if n := len(reloaded.RecoveryCodes()); n != 2 {
		t.Errorf("%d recovery hashes came back, want 2", n)
	}

	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "A comment that has to survive") {
		t.Errorf("the first run took the file's comments with it:\n%s", raw)
	}
}

// Without a factor the write is exactly what it was before this change.
func TestSaveFirstRun_WithoutAFactorLeavesBothKeysEmpty(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/web.toml"
	_ = os.WriteFile(path, []byte(`
session_key = "test-session-key-32bytes-padding!"
username    = ""
password    = ""
`), 0600)

	cfg, _ := LoadConfig(path)
	if err := cfg.SaveFirstRun(FirstRunAccount{
		Username: "admin", PasswordHash: "$argon2id$hash", Telemetry: false,
	}); err != nil {
		t.Fatal(err)
	}

	reloaded, _ := LoadConfig(path)
	if reloaded.TOTPEnabled() {
		t.Error("a first run with no factor left one enabled")
	}
	if n := len(reloaded.RecoveryCodes()); n != 0 {
		t.Errorf("%d recovery hashes stored for an account with no factor", n)
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./internal/web/ -run TestSaveFirstRun -v`
Expected: FAIL — `undefined: FirstRunAccount`.

- [ ] **Step 3: Change the signature**

In `internal/web/config.go`, replace `SaveFirstRun`:

```go
// FirstRunAccount is everything the wizard decides about the account, in one
// value, so the write that persists it cannot be called with the username and
// the hash swapped — they are adjacent strings, and two of the five fields are
// optional.
//
// It lives here beside Config rather than in shared: it is not protocol, and the
// core never sees any of it.
type FirstRunAccount struct {
	Username     string
	PasswordHash string
	Telemetry    bool

	// Empty and nil when no second factor was set up. They are written together
	// with the account or not at all — there is no path that stores one without
	// the other, which is what keeps a secret from existing for an account that
	// does not.
	TOTPSecret     string
	RecoveryHashes []string
}

// SaveFirstRun persists everything the setup wizard decides, in one write.
//
// One write rather than several, because a failure halfway through the first run
// is the worst moment to leave a half-configured file behind: the wizard closes
// as soon as a password exists, and whatever did not land cannot be asked again.
//
// The "is it still the first run" test happens here, under the same lock as the
// write. The handler checks too, but that check and this write are two moments:
// two POSTs arriving together both passed it, both wrote, and the second one
// decided who owns the firewall. The window is small and it sits on a machine
// that is, by definition, freshly exposed and not yet protected.
//
// The in-memory fields are restored when the write fails, for the reason the
// sibling savers record: a transient failure that left c.Password set would send
// the operator's retry into the ErrAlreadySetUp branch above, dead-ending the
// wizard with nothing on disk.
func (c *Config) SaveFirstRun(a FirstRunAccount) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Password != "" {
		return ErrAlreadySetUp
	}

	prevUser, prevPass := c.Username, c.Password
	prevTelemetry := c.Telemetry
	prevSecret, prevCodes := c.WebConfig.TOTPSecret, c.WebConfig.RecoveryCodes

	c.Username = a.Username
	c.Password = a.PasswordHash
	c.Telemetry = &a.Telemetry
	c.WebConfig.TOTPSecret = a.TOTPSecret
	c.WebConfig.RecoveryCodes = append([]string(nil), a.RecoveryHashes...)

	if err := c.saveLocked(); err != nil {
		c.Username, c.Password = prevUser, prevPass
		c.Telemetry = prevTelemetry
		c.WebConfig.TOTPSecret, c.WebConfig.RecoveryCodes = prevSecret, prevCodes
		return err
	}
	return nil
}
```

**Note the `&a.Telemetry`:** `a` is a value receiver parameter, so taking its
address is safe — it does not alias the caller's struct.

- [ ] **Step 4: Update the one production caller**

In `internal/web/handler_firstrun.go`, replace the call:

```go
	if err := s.cfg.SaveFirstRun(FirstRunAccount{
		Username:     answers.Username,
		PasswordHash: hash,
		Telemetry:    answers.Telemetry,
	}); err != nil {
```

- [ ] **Step 5: Update every other caller**

Run `grep -rn "SaveFirstRun(" --include='*.go' .` and convert each remaining call
site — they are in tests. Do not guess the list; grep it.

- [ ] **Step 6: Run the tests**

```bash
go test ./internal/web/ -run 'TestSaveFirstRun|TestFirstRun|TestHandleFirstRun' -v
go test ./internal/...
make lint
```

Expected: PASS. No behaviour changed, so nothing outside the signature should move.

- [ ] **Step 7: Commit**

```bash
git add internal/web/config.go internal/web/handler_firstrun.go internal/web/config_totp_test.go
git commit -m "refactor(web): SaveFirstRun takes one value

Five positional parameters, two optional and two adjacent strings, is a swap
waiting to happen. No behaviour change; the rollback and the already-set-up
guard are unchanged."
```

---

### Task 2: What the wizard is holding while it waits

**Files:**
- Create: `internal/web/firstrunpending.go`, `internal/web/firstrunpending_test.go`

**Interfaces:**
- Consumes: `firstRunData` (`handler_firstrun.go`), `FirstRunAccount` (Task 1)
- Produces:
  - `type pendingFirstRun struct { Answers firstRunData; PasswordHash, Secret string; Issued time.Time }`
  - `func newFirstRunPendingID() string`
  - `func firstRunPendingStore(id string, p pendingFirstRun)`
  - `func firstRunPendingLookup(id string) (pendingFirstRun, bool)`
  - `func firstRunPendingClear(id string)`
  - `const firstRunPendingKey = "firstrun_pending"`, `const firstRunPendingLifetime = 10 * time.Minute`

- [ ] **Step 1: Write the failing test**

Create `internal/web/firstrunpending_test.go`:

```go
package web

import (
	"testing"
	"time"
)

func TestFirstRunPending_RoundTripsAndIsolates(t *testing.T) {
	a := newFirstRunPendingID()
	b := newFirstRunPendingID()
	if a == "" || b == "" || a == b {
		t.Fatalf("ids are not distinct and non-empty: %q %q", a, b)
	}

	firstRunPendingStore(a, pendingFirstRun{
		Answers:      firstRunData{Username: "admin", SSHPort: "2222"},
		PasswordHash: "$argon2id$hash",
		Secret:       "JBSWY3DPEHPK3PXP",
		Issued:       time.Now(),
	})
	t.Cleanup(func() { firstRunPendingClear(a) })

	got, ok := firstRunPendingLookup(a)
	if !ok {
		t.Fatal("a stored entry does not read back")
	}
	if got.Answers.SSHPort != "2222" {
		t.Errorf("the wizard's other answers were lost: %+v", got.Answers)
	}
	if got.PasswordHash != "$argon2id$hash" || got.Secret != "JBSWY3DPEHPK3PXP" {
		t.Error("the hash or the secret did not survive")
	}

	if _, ok := firstRunPendingLookup(b); ok {
		t.Error("one wizard's id reads another's entry")
	}
}

func TestFirstRunPending_ExpiresAndClears(t *testing.T) {
	id := newFirstRunPendingID()
	firstRunPendingStore(id, pendingFirstRun{
		PasswordHash: "$argon2id$hash",
		Issued:       time.Now().Add(-firstRunPendingLifetime - time.Second),
	})
	t.Cleanup(func() { firstRunPendingClear(id) })

	if _, ok := firstRunPendingLookup(id); ok {
		t.Errorf("an entry issued more than %s ago was accepted", firstRunPendingLifetime)
	}

	fresh := newFirstRunPendingID()
	firstRunPendingStore(fresh, pendingFirstRun{PasswordHash: "x", Issued: time.Now()})
	firstRunPendingClear(fresh)
	if _, ok := firstRunPendingLookup(fresh); ok {
		t.Error("a cleared entry still reads back")
	}
}

func TestFirstRunPending_AnEmptyIDNeverMatches(t *testing.T) {
	firstRunPendingStore("", pendingFirstRun{PasswordHash: "x", Issued: time.Now()})
	if _, ok := firstRunPendingLookup(""); ok {
		t.Error("an empty id resolved to an entry; a request carrying no id would " +
			"then inherit somebody else's half-finished setup")
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./internal/web/ -run TestFirstRunPending -v`
Expected: FAIL — `undefined: newFirstRunPendingID`.

- [ ] **Step 3: Write `internal/web/firstrunpending.go`**

```go
package web

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

const (
	// firstRunPendingKey names the session value that carries the id. The
	// wizard already keeps state in this session — firstRunKey holds a rejected
	// submission between the POST and the re-render — so this needs no cookie
	// of its own, and adding one would mean one more MaxAge to get right. That
	// has cost this project a release before; see newSessionStore.
	firstRunPendingKey = "firstrun_pending"

	// firstRunPendingLifetime is how long a half-finished setup is held. Long
	// enough to unlock a phone, open an app, scan and type. The same value as
	// pendingSecretLifetime and a separate constant on purpose: they answer
	// different questions and may diverge.
	firstRunPendingLifetime = 10 * time.Minute
)

// pendingFirstRun is everything the wizard has collected but not yet written.
//
// Nothing in it reaches disk, and only its id travels to the browser.
// gorilla/sessions signs but does not encrypt, so a cookie value is readable
// plaintext: an argon2 digest in one is an offline cracking target handed out
// for free, and an unconfirmed secret in one is simply unnecessary.
//
// A restart mid-wizard therefore means "start again", and that costs nothing —
// the account does not exist yet either, so the first run is still the first run.
type pendingFirstRun struct {
	// The whole of the wizard's answers, not just the account: the ports and the
	// IPv6 mode are staged by applyFirstRunChoices after the write, and dropping
	// them here would silently discard everything the operator chose above the
	// password.
	Answers firstRunData

	PasswordHash string // argon2id, computed once in step 1
	Secret       string // base32, unconfirmed
	Issued       time.Time
}

// firstRunPending holds them, keyed by the id in the session.
//
// Built on the pattern of pendingSecrets in handler_2fa.go. It is bounded by
// the same argument: this is reachable only while cfg.IsFirstRun() holds, on a
// machine that has no account yet, and the wizard closes the moment one exists.
var firstRunPending = struct {
	mu sync.Mutex
	at map[string]pendingFirstRun
}{at: make(map[string]pendingFirstRun)}

// newFirstRunPendingID returns a fresh identifier for a half-finished setup.
func newFirstRunPendingID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func firstRunPendingStore(id string, p pendingFirstRun) {
	if id == "" {
		return
	}
	now := time.Now()

	firstRunPending.mu.Lock()
	defer firstRunPending.mu.Unlock()
	for k, v := range firstRunPending.at {
		if now.Sub(v.Issued) > firstRunPendingLifetime {
			delete(firstRunPending.at, k)
		}
	}
	firstRunPending.at[id] = p
}

func firstRunPendingLookup(id string) (pendingFirstRun, bool) {
	if id == "" {
		return pendingFirstRun{}, false
	}
	firstRunPending.mu.Lock()
	defer firstRunPending.mu.Unlock()

	p, ok := firstRunPending.at[id]
	if !ok || time.Since(p.Issued) > firstRunPendingLifetime {
		return pendingFirstRun{}, false
	}
	return p, true
}

func firstRunPendingClear(id string) {
	firstRunPending.mu.Lock()
	defer firstRunPending.mu.Unlock()
	delete(firstRunPending.at, id)
}
```

- [ ] **Step 4: Run the tests**

```bash
go test ./internal/web/ -run TestFirstRunPending -v
go test ./internal/web/ -race
make lint
```

Expected: PASS, no race.

- [ ] **Step 5: Commit**

```bash
git add internal/web/firstrunpending.go internal/web/firstrunpending_test.go
git commit -m "feat(web): hold a half-finished first run in memory

Keyed by an id in the session the wizard already has. Only the id travels:
gorilla/sessions signs without encrypting, so neither the argon2 hash nor an
unconfirmed secret has any business in a cookie."
```

---

### Task 3: The wizard offers it, and lets you past

The task the rest exists for. Read `internal/web/handler_firstrun.go` in full
first — particularly the comment above `handleFirstRunPOST` explaining why the
account is written before the choices are staged. That ordering is preserved;
only its trigger moves.

**Files:**
- Modify: `internal/web/handler_firstrun.go`, `internal/web/server.go`, `web/templates/firstrun.html`, `locales/en.json`, `locales/de.json`
- Test: `internal/web/handler_firstrun_2fa_test.go` (create)

**Interfaces:**
- Consumes: everything from Tasks 1 and 2; `newTOTPSecret`, `decodeTOTPSecret`, `formatTOTPSecret`, `otpauthURI`, `matchTOTP`, `totpWindowLogin`, `totpWindowEnrol` (2.8); `newRecoveryCodes` (2.8); `qrPNGDataURI` (2.8); `clockSkewKey`, `skewMinutes` (2.8)
- Produces:
  - `func (s *Server) handleFirstRunConfirm(w http.ResponseWriter, r *http.Request)`
  - `func (s *Server) handleFirstRunSkip(w http.ResponseWriter, r *http.Request)`
  - `firstRunData.WantTOTP bool`, and a `firstRunSetup` page-data shape

- [ ] **Step 1: Write the failing tests**

Create `internal/web/handler_firstrun_2fa_test.go`:

```go
package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// beginFirstRunWith2FA submits step 1 with the box ticked and returns the body
// of the step-2 page plus the cookies that carry the pending id.
func beginFirstRunWith2FA(t *testing.T, s *Server) (string, []*http.Cookie) {
	t.Helper()
	rec := doFormRequest(s, "POST", "/firstrun",
		"username=admin&password=firstrunpassword1&password_confirm=firstrunpassword1"+
			"&ssh_port=22&ipv6_mode=filter&want_totp=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("step 1 answered %d, want 200 with the setup step rendered in place", rec.Code)
	}
	return rec.Body.String(), rec.Result().Cookies()
}

func firstRunPendingSecret(t *testing.T, s *Server, cookies []*http.Cookie) string {
	t.Helper()
	req := httptest.NewRequest("GET", "/firstrun", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	sess, err := s.store.Get(req, SessionName)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := sess.Values[firstRunPendingKey].(string)
	p, ok := firstRunPendingLookup(id)
	if !ok {
		t.Fatalf("no pending first run for id %q", id)
	}
	return p.Secret
}

// Nothing is written until a code confirms. If this ever stops holding, an
// abandoned wizard leaves an account nobody can sign into.
func TestFirstRun2FA_StepOneStoresNothing(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	body, _ := beginFirstRunWith2FA(t, s)

	if !s.cfg.IsFirstRun() {
		t.Error("step 1 created the account; the wizard is closed before a code was seen")
	}
	if !strings.Contains(body, "data:image/png;base64,") {
		t.Error("no QR code on the setup step")
	}
	if !strings.Contains(body, "UTC") {
		t.Error("the server time is not shown; a wrong clock is the one failure this " +
			"page exists to make visible")
	}
}

// The whole point of the change.
func TestFirstRun2FA_ConfirmCreatesTheAccountWithTheFactor(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	_, cookies := beginFirstRunWith2FA(t, s)
	raw, err := decodeTOTPSecret(firstRunPendingSecret(t, s, cookies))
	if err != nil {
		t.Fatal(err)
	}

	rec := doFormRequest(s, "POST", "/firstrun/confirm",
		"code="+totpAt(raw, stepAt(time.Now())), cookies...)
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm answered %d, want 200 with the codes shown", rec.Code)
	}

	if s.cfg.IsFirstRun() {
		t.Fatal("a confirmed code did not create the account")
	}
	if !s.cfg.TOTPEnabled() {
		t.Error("the account was created without the factor that was just confirmed")
	}
	if n := len(s.cfg.RecoveryCodes()); n != recoveryCodeCount {
		t.Errorf("%d recovery hashes stored, want %d", n, recoveryCodeCount)
	}

	shown := 0
	for _, tok := range strings.Fields(rec.Body.String()) {
		if isRecoveryShape(strings.Trim(tok, "<>\"")) {
			shown++
		}
	}
	if shown < recoveryCodeCount {
		t.Errorf("%d recovery codes on the page, want %d — they are shown once", shown, recoveryCodeCount)
	}
}

// THE test of this change. A board with a dead RTC must still end up with an
// account. If this fails, an optional feature has become a way of bricking the
// wizard.
func TestFirstRun2FA_SkipCreatesTheAccountWithoutAFactor(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	_, cookies := beginFirstRunWith2FA(t, s)

	rec := doFormRequest(s, "POST", "/firstrun/skip", "", cookies...)
	assertRedirect(t, rec, "/login")

	if s.cfg.IsFirstRun() {
		t.Fatal("skipping the second factor did not create the account — a wrong " +
			"clock can now prevent an account existing at all")
	}
	if s.cfg.TOTPEnabled() {
		t.Error("skipping the second factor enrolled one anyway")
	}
	if u, _ := s.cfg.Credentials(); u != "admin" {
		t.Errorf("the account was created as %q, so the wizard's answers were lost", u)
	}
}

// The wizard collects more than the account. Confirming a factor must not drop
// the ports and the IPv6 mode the operator chose above the password.
func TestFirstRun2FA_ConfirmStillStagesTheOtherAnswers(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	var savedTCP *shared.Command
	fc.OnCommand(shared.CmdSaveRules, func(c shared.Command) { savedTCP = &c })

	_, cookies := beginFirstRunWith2FA(t, s)
	raw, _ := decodeTOTPSecret(firstRunPendingSecret(t, s, cookies))
	_ = doFormRequest(s, "POST", "/firstrun/confirm",
		"code="+totpAt(raw, stepAt(time.Now())), cookies...)

	if savedTCP == nil {
		t.Fatal("the ports were never staged; applyFirstRunChoices did not run")
	}
	if !strings.Contains(string(savedTCP.Payload), "\"22\"") {
		t.Errorf("the SSH port is not in the staged rules: %s", savedTCP.Payload)
	}
}

// A code that is right against a clock that is not.
func TestFirstRun2FA_AFarOutCodeDiagnosesTheClockAndStoresNothing(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	_, cookies := beginFirstRunWith2FA(t, s)
	raw, _ := decodeTOTPSecret(firstRunPendingSecret(t, s, cookies))

	rec := doFormRequest(s, "POST", "/firstrun/confirm",
		"code="+totpAt(raw, stepAt(time.Now())+8), cookies...)

	if !s.cfg.IsFirstRun() {
		t.Fatal("a code eight steps out created the account")
	}
	body := strings.ToLower(rec.Body.String())
	if !strings.Contains(body, "clock") && !strings.Contains(body, "uhr") {
		t.Error("the message does not point at the clock; the fault is on the server " +
			"and the message must not point at the human")
	}
}

func TestFirstRun2FA_AWrongCodeStoresNothing(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	_, cookies := beginFirstRunWith2FA(t, s)
	_ = doFormRequest(s, "POST", "/firstrun/confirm", "code=000000", cookies...)

	if !s.cfg.IsFirstRun() {
		t.Error("a wrong code created the account")
	}
}

// Without a pending id the routes create nothing and send the operator back.
func TestFirstRun2FA_NoPendingSetupStartsAgain(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	for _, path := range []string{"/firstrun/confirm", "/firstrun/skip"} {
		rec := doFormRequest(s, "POST", path, "code=000000")
		assertRedirect(t, rec, "/firstrun")
		if !s.cfg.IsFirstRun() {
			t.Fatalf("%s created an account with no pending setup behind it", path)
		}
	}
}

// The routes exist only while the wizard does.
func TestFirstRun2FA_RoutesAreGoneOnceAnAccountExists(t *testing.T) {
	fc := newFakeCore(t)
	s := newTestServer(t, fc) // has a password, so IsFirstRun() is false

	for _, path := range []string{"/firstrun/confirm", "/firstrun/skip"} {
		rec := doFormRequest(s, "POST", path, "")
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s answered %d on a configured install, want 404 — a "+
				"credential-writing route must not outlive the wizard", path, rec.Code)
		}
	}
}

// Unticked, nothing about the wizard changes.
func TestFirstRun2FA_UntickedIsTodaysPathExactly(t *testing.T) {
	fc := newFakeCore(t)
	s := newFirstRunTestServer(t, fc)

	rec := doFormRequest(s, "POST", "/firstrun",
		"username=admin&password=firstrunpassword1&password_confirm=firstrunpassword1"+
			"&ssh_port=22&ipv6_mode=filter")
	assertRedirect(t, rec, "/login")

	if s.cfg.IsFirstRun() {
		t.Fatal("the plain wizard stopped creating the account")
	}
	if s.cfg.TOTPEnabled() {
		t.Error("a wizard run with the box unticked enrolled a factor")
	}
}
```

Add `"github.com/jp1337/easywall/internal/shared"` to the imports.

- [ ] **Step 2: Run and watch it fail**

Run: `go test ./internal/web/ -run TestFirstRun2FA -v`
Expected: FAIL — the routes 404 and `want_totp` is ignored.

- [ ] **Step 3: Carry the choice through the form data**

In `internal/web/handler_firstrun.go`, add to `firstRunData`:

```go
	// WantTOTP is the checkbox. It survives a rejected submission like every
	// other answer: an operator who mistypes the confirmation must not have to
	// remember they had asked for a second factor.
	WantTOTP bool
```

and in `handleFirstRunPOST`, in the `answers := &firstRunData{...}` literal:

```go
		WantTOTP:  r.FormValue("want_totp") != "",
```

- [ ] **Step 4: Split the write out of the handler**

Still in `handler_firstrun.go`, add — this is today's tail, moved so both the
plain path and the confirmed path use it and cannot drift:

```go
// completeFirstRun performs the one write and everything that follows it.
//
// Both the plain wizard and the confirmed-second-factor path end here, so there
// is one place that gets the order right. The account is written first and the
// choices staged afterwards, because the wizard closes the moment a password
// exists: an operator with an account can still get in and set the rest by hand,
// whereas an operator without one cannot get in at all.
//
// **It returns two answers, not one, and the caller must keep them apart.** The
// write either happened or it did not; the staging is best-effort on top. A
// confirmed second factor has eight one-time codes waiting to be shown, and they
// must reach the operator whenever the *account* was written — losing them
// because the *ports* could not be staged is the same class of mistake this
// project has twice refused: it would leave a working factor whose only way back
// nobody has ever seen, behind a message that talks about something else.
func (s *Server) completeFirstRun(w http.ResponseWriter, r *http.Request, a FirstRunAccount, answers *firstRunData) (written, staged bool) {
	if err := s.cfg.SaveFirstRun(a); err != nil {
		if errors.Is(err, ErrAlreadySetUp) {
			slog.Warn("first run: a second setup arrived after the account existed")
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return false
		}
		slog.Error("save credentials error", "error", err)
		s.firstRunError(w, r, "save_error", answers)
		return false
	}

	if err := s.applyFirstRunChoices(answers); err != nil {
		slog.Warn("first run: could not stage the initial choices", "error", err)
		s.setFlash(w, r, "firstrun_choices_failed")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return false
	}
	return true
}
```

Then replace the tail of `handleFirstRunPOST` (from the `SaveFirstRun` call to
the end) with the branch:

```go
	// With the box ticked, nothing is written yet. The answers and the hash go
	// into memory, the secret is generated, and step 2 is rendered as this
	// POST's own response — not a GET with a URL, so a reload cannot mint a
	// second secret.
	if answers.WantTOTP {
		s.beginFirstRunTOTP(w, r, answers, hash)
		return
	}

	if !s.completeFirstRun(w, r, FirstRunAccount{
		Username:     answers.Username,
		PasswordHash: hash,
		Telemetry:    answers.Telemetry,
	}, answers) {
		return
	}

	s.setFlash(w, r, "firstrun_done")
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
```

- [ ] **Step 5: The three new handlers**

Append to `handler_firstrun.go`:

```go
// firstRunSetup is the setup step's page data. Non-nil only on that step.
type firstRunSetup struct {
	QR         template.URL
	SecretText string
	ServerTime string
}

// firstRunPage is what firstrun.html reads once the wizard has more than one
// state. Form is the answers to re-display; Setup and Codes are each non-nil on
// exactly one step.
type firstRunPage struct {
	Form  *firstRunData
	Setup *firstRunSetup
	Codes []string
}

// beginFirstRunTOTP generates a secret and shows it. Nothing is stored.
func (s *Server) beginFirstRunTOTP(w http.ResponseWriter, r *http.Request, answers *firstRunData, hash string) {
	secret, err := newTOTPSecret()
	if err != nil {
		slog.Error("could not generate a TOTP secret", "error", err)
		s.firstRunError(w, r, "internal_error", answers)
		return
	}

	id := newFirstRunPendingID()
	firstRunPendingStore(id, pendingFirstRun{
		Answers:      *answers,
		PasswordHash: hash,
		Secret:       secret,
		Issued:       time.Now(),
	})

	sess, _ := s.store.Get(r, SessionName)
	sess.Values[firstRunPendingKey] = id
	if err := sess.Save(r, w); err != nil {
		slog.Error("could not record the pending first run", "error", err)
		s.firstRunError(w, r, "internal_error", answers)
		return
	}

	s.renderFirstRunSetup(w, r, answers, secret)
}

// renderFirstRunSetup draws the setup step, with the same secret, so a wrong
// code or a failed write does not cost the operator their pairing.
func (s *Server) renderFirstRunSetup(w http.ResponseWriter, r *http.Request, answers *firstRunData, secret string) {
	qrURI, err := qrPNGDataURI(otpauthURI(answers.Username, secret))
	if err != nil {
		slog.Error("could not render the QR code", "error", err)
		s.firstRunError(w, r, "internal_error", answers)
		return
	}
	s.render(w, r, "firstrun.html", "firstrun", &firstRunPage{
		Form: answers,
		Setup: &firstRunSetup{
			// #nosec G203 -- qrURI is "data:image/png;base64," followed by base64
			// of PNG bytes this process just encoded; base64 output is
			// [A-Za-z0-9+/=], so nothing from the form can leave the attribute.
			// template.URL is the escaper's sanctioned bypass — a plain string is
			// silently defanged to #ZgotmplZ and the code never renders.
			QR:         template.URL(qrURI), //nolint:gosec // G203 — see above
			SecretText: formatTOTPSecret(secret),
			ServerTime: time.Now().UTC().Format("2 Jan 2006, 15:04:05 MST"),
		},
	})
}

// pendingFirstRunFor returns the half-finished setup this request carries.
func (s *Server) pendingFirstRunFor(r *http.Request) (string, pendingFirstRun, bool) {
	sess, err := s.store.Get(r, SessionName)
	if err != nil {
		return "", pendingFirstRun{}, false
	}
	id, _ := sess.Values[firstRunPendingKey].(string)
	p, ok := firstRunPendingLookup(id)
	return id, p, ok
}

// handleFirstRunConfirm checks the code and, only then, writes.
func (s *Server) handleFirstRunConfirm(w http.ResponseWriter, r *http.Request) {
	id, p, ok := s.pendingFirstRunFor(r)
	if !ok {
		http.Redirect(w, r, "/firstrun", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.firstRunError(w, r, "internal_error", &p.Answers)
		return
	}

	raw, err := decodeTOTPSecret(p.Secret)
	if err != nil {
		slog.Error("a secret this process generated does not decode", "error", err)
		s.firstRunError(w, r, "internal_error", &p.Answers)
		return
	}

	// The wide window first, so a right code against a wrong clock is told the
	// truth rather than "wrong code".
	_, offset, hit := matchTOTP(raw, time.Now(), strings.TrimSpace(r.FormValue("code")), totpWindowEnrol)
	switch {
	case !hit:
		s.setFlash(w, r, "totp_code_wrong")
		s.renderFirstRunSetup(w, r, &p.Answers, p.Secret)
		return
	case offset < -totpWindowLogin || offset > totpWindowLogin:
		s.setFlashN(w, r, clockSkewKey(offset), skewMinutes(offset))
		s.renderFirstRunSetup(w, r, &p.Answers, p.Secret)
		return
	}

	plain, hashes, err := newRecoveryCodes()
	if err != nil {
		slog.Error("could not generate recovery codes", "error", err)
		s.firstRunError(w, r, "internal_error", &p.Answers)
		return
	}

	if !s.completeFirstRun(w, r, FirstRunAccount{
		Username:       p.Answers.Username,
		PasswordHash:   p.PasswordHash,
		Telemetry:      p.Answers.Telemetry,
		TOTPSecret:     p.Secret,
		RecoveryHashes: hashes,
	}, &p.Answers) {
		// The entry deliberately survives a failed write: otherwise the operator
		// retypes a password and re-pairs a phone because a disk was briefly full.
		return
	}
	firstRunPendingClear(id)

	s.setFlash(w, r, "firstrun_done")
	s.render(w, r, "firstrun.html", "firstrun", &firstRunPage{Form: &p.Answers, Codes: plain})
}

// handleFirstRunSkip creates the account without a factor.
//
// This is the branch that keeps an optional feature from becoming a way of not
// getting an account. easywall runs on boards with no RTC, which come up at the
// epoch until NTP lands; if a correct code were the only way past the setup step,
// a flat battery would mean no account at all on a machine already reachable from
// the network. It takes today's path exactly.
func (s *Server) handleFirstRunSkip(w http.ResponseWriter, r *http.Request) {
	id, p, ok := s.pendingFirstRunFor(r)
	if !ok {
		http.Redirect(w, r, "/firstrun", http.StatusSeeOther)
		return
	}

	if !s.completeFirstRun(w, r, FirstRunAccount{
		Username:     p.Answers.Username,
		PasswordHash: p.PasswordHash,
		Telemetry:    p.Answers.Telemetry,
	}, &p.Answers) {
		return
	}
	firstRunPendingClear(id)

	s.setFlash(w, r, "firstrun_done")
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
```

Add `"html/template"`, `"strings"` and `"time"` to the imports as needed.

- [ ] **Step 6: `handleFirstRunGET` learns the new page shape**

```go
func (s *Server) handleFirstRunGET(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.IsFirstRun() {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	s.render(w, r, "firstrun.html", "firstrun", &firstRunPage{Form: s.firstRunForm(w, r)})
}
```

- [ ] **Step 7: The routes**

In `internal/web/server.go`, inside the **existing** `if cfg.IsFirstRun() { … }`
block, beside the two that are already there:

```go
			// Inside this block on purpose: these write credentials, and they
			// must stop existing the moment an account does. That is also why
			// they are not in credentialWritingRoutes — the demo ships with a
			// password set, so they are never registered there at all.
			r.Post("/firstrun/confirm", s.handleFirstRunConfirm)
			r.Post("/firstrun/skip", s.handleFirstRunSkip)
```

- [ ] **Step 8: The template**

`web/templates/firstrun.html` now reads `.Data.Form`, `.Data.Setup` and
`.Data.Codes`. Change every existing `{{.Data.X}}` to `{{.Data.Form.X}}` — grep
the file, do not guess — then add the checkbox inside the existing form, above
the submit button:

**Take the shape from the file, not from here — but this is what is in it**
(`firstrun.html:90-96`): the text is wrapped in a `switch-text` span, the
description class is `switch-desc`, and the input comes **after** the text and
carries `class="toggle"`. Add this beside the existing `open_web` switch, inside
the same `{{with .Data.Form}}` block, so the bare field reference resolves:

```html
              <label class="switch-row">
                <span class="switch-text">
                  <span class="switch-name">{{T "firstrun_totp_offer"}}</span>
                  <span class="switch-desc">{{T "firstrun_totp_help"}}</span>
                </span>
                <input type="checkbox" name="want_totp" value="1" class="toggle" {{if .WantTOTP}}checked{{end}}>
              </label>
```

**The data migration is two lines, not nine.** The template reaches its fields
through two `{{with .Data}}` blocks (`firstrun.html:52` and `:70`) and then
references bare `.Username`, `.SSHPort`, `.OpenWeb`, `.IPv6Mode`, `.Telemetry`,
`.WebPort` inside them. So change those two openers to `{{with .Data.Form}}` and
leave every inner reference exactly as it is. Renaming the fields one by one
would be nine chances to miss one, and a missed one renders empty with no error.

Then wrap the whole existing form in `{{if not .Data.Setup}}{{if not .Data.Codes}}`
… `{{end}}{{end}}`, and add the two new states after it. These sit **outside**
the `with` block, so they address `.Data.Setup` and `.Data.Codes` in full:

```html
{{if .Data.Setup}}
<div class="card mt-4">
  <div class="section-head">
    <div class="section-head-text">
      <h2 class="section-title">{{T "totp_setup_title"}}</h2>
    </div>
  </div>
  <div class="card-pad">
    <div class="totp-setup">
      {{/* White plate in both themes: an inverted QR code is rejected by a good
           share of scanners, and that is a defect only a dark-theme screenshot
           shows. */}}
      <div class="qr-plate">
        <img src="{{.Data.Setup.QR}}" width="256" height="256" alt="{{T "totp_qr_alt"}}">
      </div>
      <div>
        <p>{{T "totp_scan"}}</p>
        <p class="mt-2">{{T "totp_or_type"}}</p>
        <p class="totp-secret">{{.Data.Setup.SecretText}}</p>
        <p class="field-help">{{T "totp_server_time"}} {{.Data.Setup.ServerTime}}</p>

        <form method="POST" action="/firstrun/confirm" class="flex gap-2 items-end mt-3">
          <div class="field">
            <label class="field-label" for="code">{{T "totp_code_label"}}</label>
            <input type="text" id="code" name="code" inputmode="numeric"
                   autocomplete="one-time-code" spellcheck="false" required class="input">
          </div>
          <button type="submit" class="btn btn-primary">{{T "totp_confirm"}}</button>
        </form>

        {{/* Not a small link. A clock this page cannot fix must never be able to
             prevent an account existing at all. */}}
        <form method="POST" action="/firstrun/skip" class="mt-3">
          <button type="submit" class="btn">{{T "firstrun_totp_skip"}}</button>
          <p class="field-help mt-1">{{T "firstrun_totp_skip_help"}}</p>
        </form>
      </div>
    </div>
  </div>
</div>
{{end}}

{{if .Data.Codes}}
<div class="card mt-4">
  <div class="section-head">
    <div class="section-head-text">
      <h2 class="section-title">{{T "totp_codes_title"}}</h2>
    </div>
  </div>
  <div class="card-pad">
    <ul class="recovery-codes">
      {{range .Data.Codes}}
      <li class="recovery-code">
        {{.}}
      </li>
      {{end}}
    </ul>
    <div class="flex gap-2 mt-3 items-center">
      <button type="button" class="btn" data-copy-codes>{{T "totp_copy"}}</button>
      <p class="field-help"><strong>{{T "totp_codes_once"}}</strong></p>
    </div>
    <p class="mt-3"><a class="btn btn-primary" href="/login">{{T "firstrun_totp_done"}}</a></p>
  </div>
</div>
{{end}}
```

**Each recovery code goes on its own line inside its `<li>`.** All on one line
and the whole `<ul>` collapses into a single whitespace-delimited token, which
defeats the test's `strings.Fields` tokenisation — that cost a fix round in 2.8.

**Check the class names against the built stylesheet before using them.** The
2.8 plan invented `badge`/`card-title`, which do not exist here.
`TestTemplateClassesExistInStylesheet` has a blind spot: its regex skips any
`class="…"` containing braces, so a class inside an `{{if}}` is not checked.
Grep `web/static/style.css` yourself for `switch-row`, `switch-text`,
`switch-name`, `switch-desc`, `toggle`, `qr-plate`, `totp-setup`, `totp-secret`,
`recovery-codes`, `recovery-code`, `section-head`, `section-title`, `card-pad`.
Every one of those was checked against the built file while this plan was
written; if one is missing now, something else changed and you should find out
what before adding it.

- [ ] **Step 9: Both locales**

New keys in `locales/en.json` and `locales/de.json`:

```json
  {"id": "firstrun_totp_offer", "translation": "Set up a second factor now"},
  {"id": "firstrun_totp_help", "translation": "You will need an authenticator app on your phone. You can also do this later under Password."},
  {"id": "firstrun_totp_skip", "translation": "Continue without a second factor"},
  {"id": "firstrun_totp_skip_help", "translation": "Creates the account with a password alone. If the code will not match, check the server time above — you can add a second factor later under Password."},
  {"id": "firstrun_totp_done", "translation": "Continue to sign in"},
```

German counterparts, none byte-identical to the English. `totp_setup_title`,
`totp_scan`, `totp_or_type`, `totp_qr_alt`, `totp_server_time`,
`totp_code_label`, `totp_confirm`, `totp_codes_title`, `totp_codes_once`,
`totp_copy`, `totp_code_wrong`, `clock_*` already exist from 2.8 — reuse them,
do not add second copies.

- [ ] **Step 10: Run the tests**

```bash
go test ./internal/web/ -run 'TestFirstRun|TestHandleFirstRun' -v
go test ./internal/... 
go test ./internal/web/ -race
make lint
```

Expected: PASS, including `TestTemplatesOnlyUseTranslatedKeys`,
`TestLocaleFilesAreAtParity`, `TestGermanTranslationsAreNotCopiedEnglish` and
`TestNoTemplateCarriesAVersionLiteral`.

- [ ] **Step 11: Prove the escape hatch binds**

Temporarily make `handleFirstRunSkip` return without calling `completeFirstRun`.
Run `TestFirstRun2FA_SkipCreatesTheAccountWithoutAFactor`, confirm it FAILS with
the "a wrong clock can now prevent an account existing" message, then restore and
confirm it PASSES. Paste both outputs in your report — this is the test that
must not be allowed to rot.

- [ ] **Step 12: Commit**

```bash
git add internal/web/handler_firstrun.go internal/web/handler_firstrun_2fa_test.go \
        internal/web/server.go web/templates/firstrun.html locales/en.json locales/de.json
git commit -m "feat(web): the first-run wizard can switch a second factor on

Optional, and enrolled before the account is written, so the wizard still writes
exactly once and nothing unconfirmed is stored. The setup step carries a
first-class way past it: easywall runs on boards with no RTC, and a correct code
must never be the only route to having an account at all."
```

---

### Task 4: The documentation, and the roadmap

**Files:**
- Modify: `docs/_docs/installation/first-run.md`, `docs/_docs/features/two-factor.md`, `docs/_docs/roadmap.md`, `docs-tech/invariants.md`, `CHANGELOG.md`

- [ ] **Step 1: Rewrite what is now untrue**

`docs/_docs/installation/first-run.md` currently states the wizard deliberately
does not ask about a second factor, and gives the reason. That paragraph is now
false. Replace it with what the wizard does: it offers, unticked; you need an
authenticator app in hand; skipping is a first-class answer and not a failure;
and it can be done later under Password. Keep the page's voice — a table before a
list of sentences.

- [ ] **Step 2: One line in `two-factor.md`**

Under "Switching it on", say it can also be switched on during the first run, and
link to `first-run.md`. One sentence — the detail belongs on the other page.

- [ ] **Step 3: Passkeys on the roadmap**

`docs/_docs/roadmap.md` mentions passkeys only as a clause inside 3.0's row. Give
them their own entry, positioned after the hostname and certificate work, and
say plainly:

- they are a **second factor**, an alternative to TOTP — never a replacement for
  the password;
- WebAuthn requires a registrable domain as its RP ID and **rejects a bare IP
  address**, which is how most easywall installations are reached
  (`https://192.168.1.10:12227`), so they cannot come before a real hostname and
  certificate exist.

Update the ordering code block and the table together — they are two
representations of one list and drifting apart is the obvious failure — and the
front-matter release count if the number of entries changes.

- [ ] **Step 4: The invariant**

Add to `docs-tech/invariants.md`, in the file's existing shape (the guard, what
it protects, and the incident behind it):

> `TestFirstRun2FA_SkipCreatesTheAccountWithoutAFactor` — the wizard's setup step
> must always offer a way past it that still creates the account. easywall runs
> on single-board computers with no RTC, which come up at the epoch until NTP
> lands; TOTP cannot verify against a clock like that. Without this branch an
> optional feature becomes a way of bricking the wizard on a machine that is
> already reachable from the network.

Never write a version number in `docs-tech/`.

- [ ] **Step 5: CHANGELOG**

Add to `[Unreleased]` under `### Added`, in the house voice — read the 2.7.0
entry first and match it.

- [ ] **Step 6: Run the guards**

```bash
go test ./internal/shared/ -run 'TestEveryPageIsDocumented|TestTheTechnicalDocs' -v
go test ./internal/...
```

- [ ] **Step 7: Commit**

```bash
git add docs/ docs-tech/ CHANGELOG.md
git commit -m "docs: the wizard offers a second factor, and passkeys get their own entry

first-run.md said the wizard deliberately does not ask; it does now. Passkeys
move out of a clause inside 3.0 into an entry of their own, with the dependency
written out — WebAuthn needs a registrable domain and rejects a bare IP, which is
how most installations are reached."
```

---

### Task 5: Screenshots, and looking at it

**Files:**
- Modify: `docs/assets/img/screens/firstrun-{light,dark}.png`
- Create: `docs/assets/img/screens/firstrun-2fa-{light,dark}.png`, `docs/assets/img/screens/firstrun-codes-{light,dark}.png`
- Modify: `docs/_docs/installation/first-run.md` (the figure includes)

- [ ] **Step 1: Build with an explicit version**

```bash
go build -ldflags "-X github.com/jp1337/easywall/internal/shared.CurrentVersion=2.8.0" -o bin/easywall-web ./cmd/easywall-web
```

Never rely on `git describe` here: with new PNGs unstaged the tree is dirty and
the string picks up a `-dirty` suffix, which then appears in the auth-page footer
and the dashboard body line — both of which have no width cap. That shipped once.

- [ ] **Step 2: Take them**

Stand up an instance with no account (`username = ""`, `password = ""` in a
throwaway `web.toml`) and drive the wizard the way `scripts/ui-check.mjs` drives
the interface. Three screens: the form with the checkbox, the setup step, the
codes step. Both themes. Match the existing files' viewport so the diff is the
new content and not a relayout.

The codes must come from a throwaway instance that is then discarded.

- [ ] **Step 3: Look at three things, and report what you saw in words**

1. **The setup step at 390 px**, both themes, both languages. The QR, the key and
   the code field must fit; the escape-hatch button must be visible without
   scrolling past the fold if that is achievable, and must never be the thing
   that falls off the edge.
2. **The QR plate in the dark theme** — genuinely white, code dark on light.
   Sample the pixels; do not infer it from the CSS.
3. **The version string** in the wizard's footer — it must read `2.8.0`, with no
   `dirty` and no `g`-prefixed hash. Read it out of the rendered page or the
   image, not out of the build command.

- [ ] **Step 4: Reference them**

Add `themed-figure` includes to `first-run.md` in the shape the other pages use.

- [ ] **Step 5: Run everything and commit**

```bash
go test ./internal/... && make lint
git add docs/assets/img/screens/ docs/_docs/installation/first-run.md
git commit -m "docs: screenshots of the wizard's second-factor step"
```

---

## Self-review

**Spec coverage.** Every section of
`docs-tech/specs/2026-08-20-first-run-second-factor-design.md` maps to a task:
the new state → Task 2; `SaveFirstRun`'s struct → Task 1; the data flow, both
routes and the escape hatch → Task 3; error cases → Task 3 (clock, wrong code,
failed write, absent handle) and Task 1 (rollback); the proof table → Tasks 1–3;
interface and screenshots → Tasks 3 and 5; documentation and the passkeys entry →
Task 4.

**Placeholder scan.** No "TBD", no "add error handling", no "similar to Task N".
Task 4 and Task 5 describe prose and images rather than code, and give the
content and the file list — the right form for those deliverables.

**Type consistency.** `FirstRunAccount` is defined in Task 1 and used in Tasks 1
and 3. `pendingFirstRun` is defined in Task 2 and used in Task 3.
`firstRunPage`/`firstRunSetup` are defined in Task 3 and used only there.
`completeFirstRun` returns `bool` and every caller checks it. The template reads
`.Data.Form`, `.Data.Setup`, `.Data.Codes`, matching `firstRunPage`'s fields.

**A hazard that was designed out rather than flagged.** The first draft of Task 3
had the implementer rename nine `{{.Data.X}}` references in `firstrun.html`. A
missed one renders empty with no error, and no guard test catches it —
`TestTemplatesOnlyUseTranslatedKeys` checks message ids, not field paths. Reading
the template showed the fields are reached through two `{{with .Data}}` blocks,
so the migration is two openers and nothing else. Task 3 now says that.

**One class in the first draft did not exist.** `switch-help` was invented; the
file uses `switch-desc` inside a `switch-text` wrapper, with the input after the
text and carrying `class="toggle"`. Every class the plan names has since been
grepped out of the built stylesheet — the 2.8 plan shipped two invented ones, and
`TestTemplateClassesExistInStylesheet` would have caught only one of them,
because its regex skips any `class="…"` containing braces.
