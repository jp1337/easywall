# The carried-forward sweep — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close all 43 open entries in `docs-tech/carried-forward.md` — by a diff where a diff closes them, by the ruling already taken where a decision was the blocker, and by moving the entry to where a reader meets the limit where nothing can close it — so the file ends with no open row.

**Architecture:** Six phases, drawn along the files they touch rather than along the buckets, because that is what decides what can run at the same time. Phase 1 is `internal/web` tests; Phase 2 is `internal/core` and CI; Phase 3 is the Node checks under `scripts/`; Phase 4 is CSS and must finish before the screenshots; Phase 5 is the four documents; Phase 6 closes — screenshots, the file itself, and one whole-branch verification. Nothing in Phases 1–3 touches a file another task in those phases touches.

**Tech Stack:** Go 1.27 (`go test ./internal/...`, `sudo go test -tags integration`), Node 22 (`npm run check:*`), Tailwind v4, Jekyll + rouge, `golangci-lint`, `gosec`, `codespell`.

**Spec:** `docs-tech/specs/2026-09-11-the-carried-forward-sweep.md` — read it first. It carries the twelve rulings, the three corrected premises and the measurements this plan argues from.

## Global Constraints

- **Branch `chore/carried-forward-sweep`, unversioned.** No version bump, no tag, no CHANGELOG version section, no release assets. `CurrentVersion` is not touched and every screenshot's chip stays `v2.18.0`.
- **Every claim of *pre-existing* is proven against `765d615`** — the commit this branch is cut from — and never against the branch head. Use `git show 765d615:<path>` or `git diff 765d615 -- <path>`.
- **A test is verified by breaking the code, not by reading it.** Every task that adds a test names the mutation that must turn it red, and the task is not done until that mutation has been applied, observed red, and reverted.
- **`en.json` and `de.json` stay at exact parity.** Any locale key touched is touched in both.
- **A generated file is rebuilt and committed**, never assumed: `web/static/style.css` (`npm run build:css`), `docs/assets/css/style.css` (`npm run build:docs-css`), the changelog page (`npm run build:changelog`). **The diagrams are the exception** — Mermaid is not byte-reproducible; verify them with `npm run check:diagrams`, which compares a `data-source-digest`.
- **Any visible change is verified by rendering it** in a browser at **1600 / 900 / 390 px in both themes**, never by reading CSS.
- **`docs-tech/` is never published.** `TestTheTechnicalDocsAreNotPublished` enforces it; this plan and its spec live there deliberately.
- **Toolchain on this machine:** `~/go/bin/golangci-lint` and `~/go/bin/gosec`, not the ones on `PATH`. A version error from `golangci-lint` means the wrong binary, not a missing tool.
- **Commit format:** `<area>: <sentence>`, and every commit ends with `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>`.

## Dependency graph

```
Phase 1  internal/web tests        T1 T2 T3 T4 T5 T6      ── all parallel, disjoint files
Phase 2  internal/core + CI        T7 T8 T9 T10 T11 T12 T13 T14
                                   T8..T11 parallel; T13 before T14 (both edit CI config)
Phase 3  scripts/ checks           T15 T16 T17 T18 T19    ── T15 before T16 (same file)
Phase 4  CSS and design            T20 T21 T22 T23 T24 T25 T26
                                   T20..T24 parallel; T25 LAST in the phase (rebuilds both stylesheets)
Phase 5  documents                 T27 T28 T29 T30 T31    ── all parallel
Phase 6  closing                   T32 → T33 → T34        ── strictly sequential
```

**The one hard ordering rule:** Phase 4 must be complete and its stylesheets rebuilt before **T32 (screenshots)**, because the screenshots photograph what Phase 4 changes. **T33** empties `carried-forward.md` and needs every other task's outcome to be known. **T34** is the only whole-branch verification.

---

# Phase 1 · `internal/web` — six test gaps

All six tasks touch different files. They may run at the same time.

---

### Task 1: The ACME paths get unit coverage

Closes three carried entries: *The ACME serving path has no unit coverage at all*, *`Prompt` could return false*, and *`newACMEManager`'s hostname re-check is unguarded*.

Today the only coverage is `acme_integration_test.go`, which is behind `//go:build integration` with a `CLONE_NEWNET` `TestMain` — so without `CAP_SYS_ADMIN` it exits 1 with **no message at all** (`FAIL … 0.004s`). Three mutations pass the whole suite: deleting `m.sslDir = ""` from `newCertManager` (which makes easywall generate and periodically renew a self-signed certificate *alongside* the ACME one), making `GetCertificate` fall through to the self-signed path, and changing `Prompt` to return false.

**Files:**
- Modify: `internal/web/tlscert_test.go` — append two tests
- Modify: `internal/web/acme_test.go` — append two tests
- Read for context: `internal/web/tlscert.go:64-88` (`newCertManager`), `:119-125` (`GetCertificate`), `internal/web/acme.go:42-73` (`newACMEManager`)

**Interfaces:**
- Consumes: `acmeTestConfig(t *testing.T) *Config` from `internal/web/acme_test.go:21` — `validTestConfig` plus `TLS.ACME = true`, `TLS.Hostname = "firewall.example.org"`, `TLS.ACMEAgreeTOS = true`
- Produces: nothing other tasks depend on

- [ ] **Step 1: Write the two failing `tlscert_test.go` tests**

Append to `internal/web/tlscert_test.go`:

```go
// TestCertManager_WithACMEGeneratesNothingItself asserts that an ACME
// installation has no self-signed half.
//
// Deleting m.sslDir = "" from newCertManager is invisible to the whole suite:
// easywall then generates a self-signed certificate and re-generates it every
// 30 days, beside a CA-issued one it also serves. The operator sees a working
// site and a renewal loop nobody asked for.
func TestCertManager_WithACMEGeneratesNothingItself(t *testing.T) {
	cfg := acmeTestConfig(t)

	m, err := newCertManager(cfg)
	if err != nil {
		t.Fatalf("newCertManager: %v", err)
	}
	t.Cleanup(m.close)

	if m.acme == nil {
		t.Fatal("acme = true produced a manager with no autocert manager")
	}
	if m.sslDir != "" {
		t.Errorf("sslDir is %q with ACME on — easywall will generate and renew a "+
			"self-signed certificate beside the CA-issued one", m.sslDir)
	}
	if !m.usesACME() {
		t.Error("usesACME() is false with an autocert manager present")
	}
}

// TestCertManager_WithACMEServesFromAutocertNotSelfSigned asserts that the
// serving path delegates.
//
// A GetCertificate that falls through to the self-signed branch serves a
// certificate no browser believes, on an installation whose whole point is one
// that browsers do. autocert's HostPolicy is what makes the wrong SNI fail, so
// a handshake for a name nobody configured must return autocert's error rather
// than a certificate.
func TestCertManager_WithACMEServesFromAutocertNotSelfSigned(t *testing.T) {
	cfg := acmeTestConfig(t)

	m, err := newCertManager(cfg)
	if err != nil {
		t.Fatalf("newCertManager: %v", err)
	}
	t.Cleanup(m.close)

	hello := &tls.ClientHelloInfo{ServerName: "somebody-elses.example.com"}
	cert, err := m.GetCertificate(hello)
	if err == nil {
		t.Fatalf("a handshake for an unconfigured name was answered with a certificate (%v); "+
			"GetCertificate is not delegating to autocert", cert != nil)
	}
	if !strings.Contains(err.Error(), "somebody-elses.example.com") {
		t.Errorf("the refusal does not name the host it refused, so it is not autocert's "+
			"host policy talking: %v", err)
	}
}
```

Check the imports at the top of `tlscert_test.go` already include `crypto/tls` and `strings`; add whichever is missing.

- [ ] **Step 2: Run them and watch them pass, then prove they are not vacuous**

```bash
go test ./internal/web/ -run 'TestCertManager_WithACME' -v
```

Expected: both PASS. A passing new test proves nothing yet — Step 3 is the proof.

- [ ] **Step 3: Mutate the implementation three times and watch each go red**

```bash
# Mutation 1 — delete the sslDir reset
sed -i 's|^\t\tm.sslDir = ""|\t\t_ = 0 // MUTATION|' internal/web/tlscert.go
go test ./internal/web/ -run 'TestCertManager_WithACME' 2>&1 | tail -5
# Expected: FAIL — "sslDir is ... with ACME on"
git checkout internal/web/tlscert.go

# Mutation 2 — stop delegating in GetCertificate
sed -i '120,121s|if m.acme != nil {|if false { // MUTATION|' internal/web/tlscert.go
go test ./internal/web/ -run 'TestCertManager_WithACME' 2>&1 | tail -5
# Expected: FAIL — "a handshake for an unconfigured name was answered with a certificate"
git checkout internal/web/tlscert.go
```

Both mutations must FAIL the run. If either stays green, the test is wrong — fix the test, not the expectation.

- [ ] **Step 4: Write the two failing `acme_test.go` tests**

Append to `internal/web/acme_test.go`:

```go
// TestACMEManagerNeedsAHostname is the manager-level twin of
// TestACMENeedsAHostname, which asserts the config refusal.
//
// Both halves are needed for the same reason TestACMEManagerRefusesWithoutAgreedTerms
// gives for the terms: the config refuses to start, and this asserts the
// manager would also refuse if it were ever built from a config that got past
// that. An autocert.Manager with no HostPolicy answers any SNI by asking the CA
// for a certificate for it.
func TestACMEManagerNeedsAHostname(t *testing.T) {
	cfg := validTestConfig(t)
	cfg.TLS.ACME = true
	cfg.TLS.ACMEAgreeTOS = true
	cfg.TLS.Hostname = ""

	if _, err := newACMEManager(cfg); err == nil {
		t.Fatal("a manager was built with no hostname; autocert would answer any SNI")
	}
}

// TestACMEManagerReportsTheOperatorsAgreement asserts Prompt returns true.
//
// It is one line in acme.go and it decides whether any certificate is ever
// issued: autocert calls Prompt with the subscriber agreement's URL and
// abandons the order if it returns false. A Prompt that returned false would
// fail every issuance on every installation, and today only the integration
// test — the one that cannot run without CAP_SYS_ADMIN — would notice.
//
// The reason it is written as a closure rather than autocert.AcceptTOS is in
// newACMEManager's own comment: easywall reports the operator's agreement,
// recorded in acme_agree_tos, rather than making it on their behalf.
func TestACMEManagerReportsTheOperatorsAgreement(t *testing.T) {
	cfg := acmeTestConfig(t)

	m, err := newACMEManager(cfg)
	if err != nil {
		t.Fatalf("newACMEManager: %v", err)
	}
	if m.Prompt == nil {
		t.Fatal("Prompt is nil; autocert refuses every order without one")
	}
	if !m.Prompt("https://letsencrypt.org/documents/LE-SA-v1.5-February-24-2025.pdf") {
		t.Error("Prompt returned false — every certificate order on every installation fails")
	}
}
```

- [ ] **Step 5: Run and mutate**

```bash
go test ./internal/web/ -run 'TestACMEManagerNeedsAHostname|TestACMEManagerReportsTheOperatorsAgreement' -v
# Expected: both PASS

sed -i 's|Prompt: func(tosURL string) bool { return true },|Prompt: func(tosURL string) bool { return false }, // MUTATION|' internal/web/acme.go
go test ./internal/web/ -run TestACMEManagerReportsTheOperatorsAgreement 2>&1 | tail -4
# Expected: FAIL — "Prompt returned false"
git checkout internal/web/acme.go

sed -i '44,46s|if host == "" {|if false { // MUTATION|' internal/web/acme.go
go test ./internal/web/ -run TestACMEManagerNeedsAHostname 2>&1 | tail -4
# Expected: FAIL — "a manager was built with no hostname"
git checkout internal/web/acme.go
```

- [ ] **Step 6: Commit**

```bash
go test ./internal/web/ 2>&1 | tail -3
git add internal/web/tlscert_test.go internal/web/acme_test.go
git commit -m "$(cat <<'MSG'
test(web): the ACME paths get unit coverage that does not need a kernel

Three mutations passed the whole suite: deleting m.sslDir = "" (which makes
easywall generate and renew a self-signed certificate beside the ACME one),
making GetCertificate fall through to the self-signed branch, and Prompt
returning false, which fails every issuance. All three were caught only by
acme_integration_test.go, which exits 1 with no message at all without
CAP_SYS_ADMIN.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

---

### Task 2: The passkey store's two silent constants

Closes two carried entries: *`passkeys.json`'s file mode is unpinned* and *The `v3:` domain separator is not pinned*.

`0600` → `0644` is silent. The file holds credential IDs and public keys, not secrets — which is why this is low — but this repository pins modes elsewhere, and an operator reading `ls -l` for reassurance gets it from the mode. Reverting `credentialFingerprint`'s separator to `v2:` is also silent; the same claim was made for v1 → v2 in 2.8 and guarded neither time.

**Files:**
- Modify: `internal/web/passkeystore_test.go` — append one test
- Modify: `internal/web/config_totp_test.go` — append one test (it already holds the `credentialFingerprint` tests at `:173`)
- Read for context: `internal/web/passkeystore.go:279` (`writeFileAtomic(p.path, data, 0600)`), `internal/web/auth.go:185-188` (`credentialFingerprint`)

**Interfaces:**
- Consumes: whatever constructor `passkeystore_test.go` already uses to get a `*passkeyStore` with a temp path — read the top of that file and use the same one
- Produces: nothing

- [ ] **Step 1: Write the failing mode test**

Append to `internal/web/passkeystore_test.go`. Build a store the way the tests already in that file do, enrol or save one credential so the file exists, then:

```go
// TestThePasskeyStoreFileModeIsPinned asserts 0600 on passkeys.json.
//
// The file holds credential IDs and public keys, not secrets, so 0644 would
// leak nothing an attacker could authenticate with. It is pinned anyway for
// the reason every other mode in this repository is: an operator who runs
// ls -l on data_dir is asking a question, and the mode is the answer. A
// silent 0644 makes that answer wrong.
func TestThePasskeyStoreFileModeIsPinned(t *testing.T) {
	path := filepath.Join(t.TempDir(), "passkeys.json")
	p := newPasskeyStore(path)
	// add() is what persists — there is no bare save(); saveLocked() is called
	// under the store's own lock by add, remove and updateCounter.
	if err := p.add("YubiKey on the keyring", webauthn.Credential{ID: []byte("cred-1")}); err != nil {
		t.Fatalf("add: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("passkeys.json is mode %04o, want 0600", got)
	}
}
```

This is the construction `TestThePasskeyStoreSurvivesARestart` at `internal/web/passkeystore_test.go:17` already uses; `os`, `path/filepath` and the `webauthn` package are already imported in that file.

- [ ] **Step 2: Write the failing separator test**

Append to `internal/web/config_totp_test.go`:

```go
// TestTheSessionFingerprintDomainSeparatorIsPinned holds the v3 separator by
// its digest.
//
// Reverting "easywall-session-v3:" to "v2:" is invisible: the added passkey
// input already changes every real digest, so no behavioural test notices the
// separator itself. The bump is belt-and-braces — and the same was true of
// v1 → v2 in 2.8, which was also unguarded, which is how this entry came to
// be written twice.
//
// The two golden values were computed from the implementation at the commit
// that added this test. A change to either the separator or the input order
// changes them, which is the point: whoever changes the scheme updates this
// test deliberately and says so in the changelog, as v1 → v2 was.
func TestTheSessionFingerprintDomainSeparatorIsPinned(t *testing.T) {
	tests := []struct {
		name                            string
		hash, totpSecret, passkeySet    string
		want                            string
	}{
		{"all three inputs present", "hash", "secret", "set", "quWtKv9akyc3XLZsq/fIUQ"},
		{"password only", "hash", "", "", "k1HQR0C1DiDMEK0DnJXh0A"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := credentialFingerprint(tc.hash, tc.totpSecret, tc.passkeySet)
			if got != tc.want {
				t.Errorf("credentialFingerprint(%q, %q, %q) = %q, want %q — the domain "+
					"separator or the input order changed; if that was deliberate it needs "+
					"a changelog line, because every session in flight ends once",
					tc.hash, tc.totpSecret, tc.passkeySet, got, tc.want)
			}
		})
	}
}
```

Both golden values were computed against `internal/web/auth.go:186` as it stands — `sha256("easywall-session-v3:" + hash + "\x00" + totp + "\x00" + passkeys)`, first 16 bytes, `base64.RawStdEncoding`.

- [ ] **Step 3: Run both**

```bash
go test ./internal/web/ -run 'TestThePasskeyStoreFileModeIsPinned|TestTheSessionFingerprintDomainSeparatorIsPinned' -v
```

Expected: PASS. If the fingerprint test fails, the golden values are wrong for this tree — recompute rather than adjusting the test's expectation blindly:

```bash
cat > /tmp/fp.go <<'EOF'
package main
import ("crypto/sha256";"encoding/base64";"fmt")
func main(){
 s:=sha256.Sum256([]byte("easywall-session-v3:hash\x00secret\x00set"))
 fmt.Println(base64.RawStdEncoding.EncodeToString(s[:16]))
}
EOF
go run /tmp/fp.go
```

- [ ] **Step 4: Mutate both and watch them go red**

```bash
sed -i 's|easywall-session-v3:|easywall-session-v2:|' internal/web/auth.go
go test ./internal/web/ -run TestTheSessionFingerprintDomainSeparatorIsPinned 2>&1 | tail -4
# Expected: FAIL, both subtests
git checkout internal/web/auth.go

sed -i '279s|data, 0600)|data, 0644) // MUTATION|' internal/web/passkeystore.go
go test ./internal/web/ -run TestThePasskeyStoreFileModeIsPinned 2>&1 | tail -4
# Expected: FAIL — "passkeys.json is mode 0644, want 0600"
git checkout internal/web/passkeystore.go
```

- [ ] **Step 5: Commit**

```bash
git add internal/web/passkeystore_test.go internal/web/config_totp_test.go
git commit -m "$(cat <<'MSG'
test(web): pin passkeys.json's mode and the session fingerprint's separator

Both were silent under mutation. 0600 -> 0644 leaks no secret — the file holds
credential IDs and public keys — but an operator reading ls -l gets their
answer from the mode. The v3 separator is held by two golden digests rather
than by a behavioural assertion, because the added passkey input already
changes every real digest and hides the separator itself.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

---

### Task 3: The password floor's guard sees the rule, not the identifier

Closes *`TestTheRuleIsStatedOnce` catches the identifier, not the rule*.

The guard at `internal/web/auth_test.go:191` greps every non-test file in the package for the string `minPasswordLen`. So a second copy of the rule written `len(pw) < minPasswordLen` fails it — and written `len(pw) < 12`, which is the way a careless author actually restates a rule, passes. The one restatement it must catch is the one it cannot see.

`go/parser` and `go/ast` are in the standard library and are the right tools: the guard becomes a search for a comparison between a `len(...)` call and an integer literal of 11 or 12, in any non-test file except `auth.go`. Measured baseline: `internal/web` has **no** such comparison today, so the new half is green on an unmodified tree.

**Files:**
- Modify: `internal/web/auth_test.go:185-213` — extend `TestTheRuleIsStatedOnce`
- Read for context: `internal/web/auth.go:33-36` (`minPasswordLen = 12`)

**Interfaces:**
- Consumes: nothing
- Produces: nothing

- [ ] **Step 1: Extend the guard**

Replace the body of `TestTheRuleIsStatedOnce` in `internal/web/auth_test.go` with:

```go
// TestTheRuleIsStatedOnce guards the thing minPasswordLen's own comment asks for
// and did not get: the comparison lived at handler_firstrun.go:126 and
// handler_password.go:93, and two copies of a rule is how the second one comes
// to disagree.
//
// It checks two shapes, because the first version checked only one and a 2.18
// mutation proved which one mattered. Grepping for the identifier catches
// `len(pw) < minPasswordLen`, which a careful author writes. It does not catch
// `len(pw) < 12`, which is what a careless one writes — there is no identifier
// left to find. The second half therefore parses instead of grepping, and
// refuses a comparison between a len() call and the literal 11 or 12 anywhere
// in the package outside auth.go.
//
// Both bounds, because `> 11` and `< 12` are the same rule spelled two ways.
func TestTheRuleIsStatedOnce(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("*.go"))
	if err != nil {
		t.Fatal(err)
	}

	var identifierOffenders, literalOffenders []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || filepath.Base(f) == "auth.go" {
			continue
		}

		body, err := os.ReadFile(f) // #nosec G304 -- globbing this package
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "minPasswordLen") {
			identifierOffenders = append(identifierOffenders, f)
		}

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, f, body, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			be, ok := n.(*ast.BinaryExpr)
			if !ok {
				return true
			}
			switch be.Op {
			case token.LSS, token.LEQ, token.GTR, token.GEQ, token.EQL, token.NEQ:
			default:
				return true
			}
			if isLenCall(be.X) && isPasswordFloorLiteral(be.Y) ||
				isLenCall(be.Y) && isPasswordFloorLiteral(be.X) {
				literalOffenders = append(literalOffenders,
					fmt.Sprintf("%s:%d", f, fset.Position(be.Pos()).Line))
			}
			return true
		})
	}

	if len(identifierOffenders) > 0 {
		t.Errorf("minPasswordLen is compared outside auth.go, in %v — "+
			"call passwordPolicyError instead", identifierOffenders)
	}
	if len(literalOffenders) > 0 {
		t.Errorf("a password length is compared against a bare 11 or 12 at %v — "+
			"that is the password floor restated without naming it, which is the "+
			"copy this guard exists to stop. Call passwordPolicyError instead",
			literalOffenders)
	}
}

// isLenCall reports whether e is a call to the builtin len.
func isLenCall(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	ident, ok := call.Fun.(*ast.Ident)
	return ok && ident.Name == "len"
}

// isPasswordFloorLiteral reports whether e is the integer 11 or 12 — the floor
// itself, and the floor written as the value one below it.
func isPasswordFloorLiteral(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.INT {
		return false
	}
	return lit.Value == "11" || lit.Value == "12"
}
```

Add `fmt`, `go/ast`, `go/parser` and `go/token` to the file's imports.

- [ ] **Step 2: Run it on the unmodified tree**

```bash
go test ./internal/web/ -run TestTheRuleIsStatedOnce -v
```

Expected: PASS. The package has no `len(...) < 12` comparison today — this was measured before the task was written. If it fails, read the reported line: either it is a genuine second statement of the rule (fix the code) or it is a length check about something else entirely (narrow the guard, and say in its comment what it now excludes).

- [ ] **Step 3: Mutate — the shape the old guard could not see**

```bash
# The careless restatement, in a file that has no business stating it
sed -i 's|^func (s \*Server) handlePasswordPOST|func minPasswordMutation(pw string) bool { return len(pw) < 12 }\n\nfunc (s *Server) handlePasswordPOST|' internal/web/handler_password.go
go test ./internal/web/ -run TestTheRuleIsStatedOnce 2>&1 | tail -4
# Expected: FAIL — "a password length is compared against a bare 11 or 12"
git checkout internal/web/handler_password.go
```

Then confirm the old half still works:

```bash
sed -i 's|^func (s \*Server) handlePasswordPOST|func minPasswordMutation(pw string) bool { return len(pw) < minPasswordLen }\n\nfunc (s *Server) handlePasswordPOST|' internal/web/handler_password.go
go test ./internal/web/ -run TestTheRuleIsStatedOnce 2>&1 | tail -4
# Expected: FAIL — "minPasswordLen is compared outside auth.go"
git checkout internal/web/handler_password.go
```

If `handlePasswordPOST` is not the function name in that file, insert the mutation before whatever the first top-level `func` is.

- [ ] **Step 4: Commit**

```bash
go test ./internal/web/ 2>&1 | tail -3
git add internal/web/auth_test.go
git commit -m "$(cat <<'MSG'
test(web): the password-floor guard sees the rule, not only its name

It grepped for minPasswordLen, so `len(pw) < minPasswordLen` failed it and
`len(pw) < 12` passed — the one restatement a careless author actually writes
was the one it could not see. The second half parses with go/ast and refuses a
len() comparison against a bare 11 or 12 outside auth.go. Measured green on an
unmodified tree: the package has no such comparison.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

---

### Task 4: A ceremony runs with a port in the origin

Closes *No end-to-end passkey ceremony ever runs with a port in the origin*.

`newPasskeyTestServer` pins `BindAddr = "127.0.0.1:443"`, and its comment says why: that is how an ordinary HTTPS installation reads, so a fixture's `RelyingParty.Origin` can be written as a bare `https://host`. easywall's own default is `:12227`. `TestPublicOriginCarriesTheListeningPort` holds the string and `webAuthn()` passes `publicOrigin()` straight into `RPOrigins`, so the gap is narrow — but a ceremony that disagreed with it would still not fail here.

The fixture already takes options (`passkeyTestOption`, `internal/web/handler_passkey_test.go:34`). This adds one and uses it in one ceremony test.

**Files:**
- Modify: `internal/web/handler_passkey_test.go` — add `withBindAddr`, add one test
- Read for context: `internal/web/handler_passkey_test.go:46-58` (`withHostname`, the option to copy), `:86-94` (`newPasskeyTestServer`), `internal/web/handler_passkey.go:278-292` (`webAuthn`), `internal/web/handler_firstrun.go:103-109` (`publicOrigin`)

**Interfaces:**
- Consumes: `passkeyTestOption func(*Server)`; `newPasskeyTestServer(t, opts ...passkeyTestOption) *Server`
- Produces: `withBindAddr(addr string) passkeyTestOption` — available to any later passkey test

- [ ] **Step 1: Add the option**

Insert after `withHostname` in `internal/web/handler_passkey_test.go`:

```go
// withBindAddr overrides the fixture's :443 with a real listening address, so
// publicOrigin() names a port and the ceremony has to agree with it.
//
// The fixture's default stays :443 deliberately — see newPasskeyTestServer's
// comment — because that is how an ordinary HTTPS installation reads. easywall's
// own default is :12227, and until this option existed no end-to-end ceremony
// ran with a port in the origin at all: webAuthn() passes publicOrigin()
// straight into RPOrigins, so a ceremony that disagreed with the port would
// have failed on a real installation and passed here.
func withBindAddr(addr string) passkeyTestOption {
	return func(s *Server) {
		s.cfg.BindAddr = addr
	}
}
```

- [ ] **Step 2: Write the failing ceremony test**

Read `TestAPasskeyCanBeEnrolledAndCounts` at `internal/web/handler_passkey_test.go:347` in full. Copy its body into a new test below it, changing only the fixture construction and adding the origin assertion:

```go
// TestAPasskeyIsEnrolledOnEasywallsOwnDefaultPort runs the enrolment ceremony
// on :12227 rather than the fixture's :443.
//
// Every other ceremony test runs on :443, where publicOrigin() names no port —
// which is how an ordinary HTTPS installation reads, and is not how an ordinary
// *easywall* installation reads. The origin is part of what a browser signs
// over, so a ceremony that agreed with the RP ID and disagreed with the port
// would pass the whole suite and fail on every default install.
func TestAPasskeyIsEnrolledOnEasywallsOwnDefaultPort(t *testing.T) {
	s := newPasskeyTestServer(t,
		withHostname("firewall.example.org"),
		withBindAddr(":12227"),
	)

	if got := s.publicOrigin(); got != "https://firewall.example.org:12227" {
		t.Fatalf("publicOrigin() = %q — the fixture is not on the port this test exists for", got)
	}
	wa, err := s.webAuthn()
	if err != nil {
		t.Fatalf("webAuthn: %v", err)
	}
	if wa.Config.RPID != "firewall.example.org" {
		t.Errorf("RPID = %q, want the bare hostname — the port belongs to the origin, not the RP ID",
			wa.Config.RPID)
	}
	if len(wa.Config.RPOrigins) != 1 || wa.Config.RPOrigins[0] != "https://firewall.example.org:12227" {
		t.Errorf("RPOrigins = %v, want exactly [https://firewall.example.org:12227]", wa.Config.RPOrigins)
	}

	// …then the full enrolment ceremony, copied from
	// TestAPasskeyCanBeEnrolledAndCounts: begin, sign the challenge with the
	// virtual authenticator that test uses, finish, and assert the credential
	// counts. Copy it rather than extracting a shared helper — the two tests
	// assert different things and a helper would hide which one broke.
}
```

Fill the ceremony half from the existing test. Do not invent an authenticator API — use exactly what `TestAPasskeyCanBeEnrolledAndCounts` uses.

- [ ] **Step 3: Run it**

```bash
go test ./internal/web/ -run TestAPasskeyIsEnrolledOnEasywallsOwnDefaultPort -v
```

Expected: PASS. If the ceremony half fails on origin mismatch, that is the finding this task exists to find — read the error before changing the test, and report it rather than adjusting the expectation.

- [ ] **Step 4: Mutate `publicOrigin` and watch it go red**

```bash
sed -i '105s|port != "443"|port != "443" \&\& false // MUTATION|' internal/web/handler_firstrun.go
go test ./internal/web/ -run TestAPasskeyIsEnrolledOnEasywallsOwnDefaultPort 2>&1 | tail -4
# Expected: FAIL — publicOrigin() drops the port
git checkout internal/web/handler_firstrun.go
```

- [ ] **Step 5: Commit**

```bash
go test ./internal/web/ 2>&1 | tail -3
git add internal/web/handler_passkey_test.go
git commit -m "$(cat <<'MSG'
test(web): one passkey ceremony runs on easywall's own default port

Every ceremony test ran on the fixture's :443, where publicOrigin() names no
port. easywall's default is :12227, the origin is part of what the browser
signs over, and webAuthn() passes publicOrigin() straight into RPOrigins — so
a ceremony that disagreed with the port passed the suite and would fail on
every default installation. The fixture's :443 default stays; this adds a
withBindAddr option and one test that uses it.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

---

### Task 5: `JustGated` becomes observable

Closes *`JustGated` is unobservable when no codes are minted*.

`web/templates/password.html:302` nests `{{if .JustGated}}` inside the `{{if .Codes}}` card that starts at `:283`. A test can therefore only see the flag through the codes block, and a future change that set it without minting codes would be invisible to any HTTP-level test — and would also leave the operator with no way onward, which is what the block is for.

**Files:**
- Modify: `web/templates/password.html:283-312` — move the `JustGated` block out of the `Codes` card
- Modify: `internal/web/handler_2fa_test.go` — add one test, beside the existing `TestEnrol_ConfirmMintsNothingForASecondFactorWithNoCodesStored` at `:163` whose comment records this nesting as a limitation
- Read for context: `internal/web/handler_password.go:46` (the `JustGated` field comment), `internal/web/handler_2fa.go:247` and `:325` (the two places it is set)

**Interfaces:**
- Consumes: `passwordPageData` and its `JustGated bool` / `Codes []string` fields
- Produces: nothing

- [ ] **Step 1: Move the block in the template**

In `web/templates/password.html`, cut lines 302–308 (the `{{if .JustGated}}` … `{{end}}` block including its comment) out of the codes card, and place them **after** the `{{end}}` that closes `{{if .Codes}}`, as a sibling. Update the comment to say why it is a sibling:

```html
        {{end}}

        {{if .JustGated}}
        {{/* The gate is now open — hasSecondFactor() is true from this response
             onward — so the way onward is a link rather than a redirect, which
             would skip the recovery codes above when there are any.

             A sibling of the codes card, not a child of it: the flag and the
             codes are two different facts. Nesting it meant no HTTP-level test
             could see JustGated without codes also being present, so a change
             that set one without the other would have been invisible — and
             would have left the operator on this page with no way forward. */}}
        <p class="mt-3"><a href="/dashboard" class="btn btn-primary">{{T "nav_dashboard"}} &rarr;</a></p>
        {{end}}
```

Get the nesting right by reading `:283-313` in full first — the card has a `card-pad` div and two `{{end}}`s in a row at `:311-312` that belong to outer blocks.

- [ ] **Step 2: Write the failing test**

Append to `internal/web/handler_2fa_test.go`, beside the existing test that
asserts the negative of this (`TestEnrol_ConfirmMintsNothingForASecondFactorWithNoCodesStored`
at `:163`, whose comment records the nesting as a limitation):

```go
// TestTheWayOnwardDoesNotDependOnCodesBeingMinted asserts that JustGated is
// observable on its own.
//
// The link used to live inside the recovery-codes card, so it rendered only
// when codes were minted too. Two consequences: no test could see the flag by
// itself — the sibling test above says so in its own comment — and an operator
// whose gate opened without codes would have been left on this page with no
// way forward.
//
// No handler produces that state today: both setters (handle2FAConfirm's
// success path and handle2FAEnrolUnverified) mint codes in the same breath.
// The template is therefore rendered directly, which is the honest way to
// assert a state the handlers cannot currently reach and a future change can.
func TestTheWayOnwardDoesNotDependOnCodesBeingMinted(t *testing.T) {
	s := serverWithPassword(t)

	page := s.passwordPage(nil, nil) // no setup, no codes
	page.JustGated = true

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/password", nil)
	s.render(rec, req, "password.html", "password", page)

	if rec.Code != http.StatusOK {
		t.Fatalf("render answered %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "recovery-code") {
		t.Fatal("the codes block rendered with no codes — the fixture is wrong, not the template")
	}
	if !strings.Contains(body, `href="/dashboard"`) {
		t.Error("the gate opened with no recovery codes and the page offers no way onward; " +
			"JustGated is unobservable without Codes")
	}
}
```

`serverWithPassword(t)`, `httptest`, `net/http` and `strings` are all already
used in `handler_2fa_test.go`. `s.render` is `internal/web/server.go:647`, and
`s.passwordPage(setup *totpSetup, codes []string) passwordPageData` is
`internal/web/handler_password.go:70`.

**Do not break the sibling.** `TestEnrol_ConfirmMintsNothingForASecondFactorWithNoCodesStored`
asserts the body contains neither `recovery-code` nor `&rarr;`. In that
scenario `JustGated` is **false**, so un-nesting leaves it green — `&rarr;`
appears nowhere else on the page. Run it explicitly in Step 3 to confirm that
rather than assuming it.

- [ ] **Step 3: Run it, then prove it by reverting the template**

```bash
go test ./internal/web/ -run 'TestTheWayOnwardDoesNotDependOnCodesBeingMinted|TestEnrol_ConfirmMintsNothingForASecondFactorWithNoCodesStored' -v
# Expected: both PASS — the new one, and the sibling that must not regress

git stash push web/templates/password.html
go test ./internal/web/ -run TestTheWayOnwardDoesNotDependOnCodesBeingMinted 2>&1 | tail -4
# Expected: FAIL — the nesting itself is the defect, so reverting the template is the mutation
git stash pop
```

- [ ] **Step 4: Check the page still renders both ways in a browser**

The codes card and the link are now siblings, so the spacing between them changed. Start the local review server and load `/password` in both states (see `docs-tech/local-review.md`), at **1600 / 900 / 390 px in both themes**. A `mt-3` on a sibling paragraph is not the same vertical rhythm as one inside `card-pad`; if it reads wrong, fix the spacing here rather than leaving it for the screenshot task to discover.

- [ ] **Step 5: Commit**

```bash
go test ./internal/web/ 2>&1 | tail -3
git add web/templates/password.html internal/web/handler_2fa_test.go
git commit -m "$(cat <<'MSG'
fix(web): the way onward no longer depends on recovery codes being minted

password.html nested {{if .JustGated}} inside {{if .Codes}}, so the link was
reachable only when codes were minted too. A test could not see the flag on its
own, and an operator whose gate opened without codes would have been left on
the page with no way forward. The two are now siblings, which is what they are.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

---

### Task 6: The core client's malformed-reply edge

Closes *`CoreClient.GetHealth`, `GetUsage` and `GetAppliedConfig` have no test for a malformed reply*.

`fakeCore` is a real Unix socket listener (`internal/web/testhelpers_test.go:24-53`), so `TestTheDashboardFallsBackWhenTheCoreCannotAnswerAboutHealth` already exercises `GetHealth`'s `!resp.Success` path at wire level. What is uncovered is the reply that parses as a **frame** and not as the **payload** — `Success: true` with a `Data` that is valid JSON of the wrong shape. All three methods then return a `parse …: %w` error that nothing asserts. `GetUsage` and `GetAppliedConfig` predate the branch with the same gap; `GetHealth` is new in 2.17.

**Files:**
- Modify: `internal/web/client_test.go` (confirm the name with `ls internal/web/ | grep client`) — append one table-driven test
- Read for context: `internal/web/client.go:281-336` (the three methods), `internal/shared/protocol.go:256-260` (`Response.Data` is `json.RawMessage`)

**Interfaces:**
- Consumes: `newFakeCore(t) *fakeCore` (`internal/web/testhelpers_test.go:35`), `fc.SetResponse(cmdType shared.CommandType, resp shared.Response)`, `fc.socketPath`, `NewCoreClient(socketPath string) *CoreClient` (`internal/web/client.go:20`)
- Produces: nothing

- [ ] **Step 1: Write the failing test**

```go
// TestTheClientReportsAReplyThatIsNotItsPayload covers the gap between "the
// core said no" and "the core said something else".
//
// !resp.Success is already covered at wire level. This is the other half: a
// reply that parses as a frame and not as the payload — Success true, Data
// valid JSON of the wrong shape. Each method must return an error naming what
// it could not parse, because the caller's fallback depends on knowing the
// difference between a core that refused and a core that answered nonsense.
func TestTheClientReportsAReplyThatIsNotItsPayload(t *testing.T) {
	tests := []struct {
		name    string
		cmd     shared.CommandType
		call    func(*CoreClient) error
		wantMsg string
	}{
		{
			name: "health",
			cmd:  shared.CmdGetHealth,
			call: func(c *CoreClient) error { _, err := c.GetHealth(); return err },
			wantMsg: "parse health",
		},
		{
			name: "usage",
			cmd:  shared.CmdGetUsage,
			call: func(c *CoreClient) error { _, err := c.GetUsage(); return err },
			wantMsg: "parse usage",
		},
		{
			name: "applied config",
			cmd:  shared.CmdGetAppliedConfig,
			call: func(c *CoreClient) error { _, err := c.GetAppliedConfig(); return err },
			wantMsg: "parse applied config",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fc := newFakeCore(t)
			// Valid JSON, wrong shape: an array where a struct is expected.
			fc.SetResponse(tc.cmd, shared.Response{
				Success: true,
				Data:    json.RawMessage(`["not", "an", "object"]`),
			})
			c := NewCoreClient(fc.socketPath)

			err := tc.call(c)
			if err == nil {
				t.Fatal("a reply that is not the payload was accepted as one")
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("the error does not say what could not be parsed: %v", err)
			}
		})
	}
}
```

`NewCoreClient(socketPath string) *CoreClient` is at `internal/web/client.go:20` and `fakeCore.socketPath` is its field — no helper is needed. Add `encoding/json` to the imports.

- [ ] **Step 2: Run it**

```bash
go test ./internal/web/ -run TestTheClientReportsAReplyThatIsNotItsPayload -v
```

Expected: all three subtests PASS.

- [ ] **Step 3: Mutate — delete one error check and watch it go red**

```bash
sed -i 's|return nil, fmt.Errorf("parse health: %w", err)|_ = err // MUTATION\n\t\treturn \&res, nil|' internal/web/client.go
go test ./internal/web/ -run TestTheClientReportsAReplyThatIsNotItsPayload/health 2>&1 | tail -4
# Expected: FAIL — "a reply that is not the payload was accepted as one"
git checkout internal/web/client.go
```

- [ ] **Step 4: Commit**

```bash
go test ./internal/web/ 2>&1 | tail -3
git add internal/web/client_test.go
git commit -m "$(cat <<'MSG'
test(web): the three getters report a reply that is not their payload

!resp.Success was already covered at wire level through fakeCore. The uncovered
half is the reply that parses as a frame and not as the payload: Success true,
Data valid JSON of the wrong shape. GetUsage and GetAppliedConfig had the gap
before 2.17; GetHealth arrived with it.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

---

# Phase 2 · `internal/core`, the CLI and CI

T8–T11 touch different files and may run at the same time. **T13 must land before T14** — both edit CI configuration, and T14's new guard test asserts a fact T13 does not change but a merge conflict would obscure.

---

### Task 7: A skip that reads like a pass becomes a failure

Closes *Four `TestIntegration_Forward_*` tests skip in a container*.

`internal/core/nftables_forward_test.go` is byte-identical to `765d615`. Four tests share one precondition and one message:

```
:142  t.Skip("skipping: the two sides cannot reach each other before any firewall exists")
:180  (same)
:235  (same)
:262  (same)
```

A skip whose precondition is absent reads exactly like a pass — `go test` prints the reason and the job exits 0. That is the class `EASYWALL_REQUIRE_SELFTEST` was added to close for the self-test provers in 2.17, and the mechanism is already in this package: `skipOrFailUnprovable(t, reason)` at `internal/core/selftest_required_test.go:51`, same package, same `//go:build integration` tag, directly callable.

**Reuse the existing variable rather than adding a second one.** `EASYWALL_REQUIRE_SELFTEST` is already set by `.github/workflows/test.yml:143`, already kept in sync with its Go spelling by `TestTheCIProofGateIsStillWiredUp` in `internal/shared`, and already documented. A second variable would need a second wiring, a second guard and a second changelog line to buy the same thing. The cost is that the variable's name now covers more than the self-test; that is recorded in the helper's comment and in `invariants.md` rather than fixed by a rename, which would touch `test.yml`, `build.yml`, `CHANGELOG.md`, the generated changelog page and the shared guard.

**Files:**
- Modify: `internal/core/nftables_forward_test.go:142`, `:180`, `:235`, `:262`
- Modify: `internal/core/selftest_required_test.go:45-51` — widen the helper's comment
- Read for context: `internal/core/selftest_required_test.go` in full (it is 60 lines and it argues the whole case)

**Interfaces:**
- Consumes: `skipOrFailUnprovable(t *testing.T, reason string)` and `requireSelftestEnv` from `internal/core/selftest_required_test.go`
- Produces: nothing

- [ ] **Step 1: Prove the four tests skip on this host today**

```bash
sudo go test -tags integration -run 'TestIntegration_Forward_' -v ./internal/core/ 2>&1 | grep -E '^(---|=== RUN|ok|FAIL)' | head -20
```

Record what you see. If they all run and pass here, the skip is not reachable on this machine and Step 4's proof must use the environment variable instead.

- [ ] **Step 2: Replace the four skips**

In `internal/core/nftables_forward_test.go`, replace each of the four

```go
	if !r.reachable() {
		t.Skip("skipping: the two sides cannot reach each other before any firewall exists")
	}
```

with

```go
	if !r.reachable() {
		skipOrFailUnprovable(t, "the two sides cannot reach each other before any firewall exists")
	}
```

`skipOrFailUnprovable` calls `t.Skipf` on a developer's machine and `t.Fatalf` when `EASYWALL_REQUIRE_SELFTEST` is set, so the four tests keep behaving exactly as they do locally and stop being green-for-nothing in CI.

Leave the other four skips in that file alone — `:41` (a missing binary), `:88` (forwarding cannot be enabled), `:97` (no namespace) and `:115` (a command failed) are the router harness's own construction, reached before any claim is made, and turning those into failures is a separate decision about what the job requires.

- [ ] **Step 3: Widen the helper's comment**

In `internal/core/selftest_required_test.go`, extend `skipOrFailUnprovable`'s doc comment:

```go
// skipOrFailUnprovable is the one place a test is allowed to give up on a host
// that cannot build what it measures in. On a developer's machine it skips,
// which is right — rootless `unshare -n` is refused on plenty of them, and the
// three-state result exists precisely so that "cannot prove" does not read as
// "broken firewall". In CI it fails.
//
// Its scope is wider than the environment variable's name. It started with the
// self-test provers and now also covers the four TestIntegration_Forward_*
// tests, whose shared precondition — the two sides reaching each other before
// any firewall exists — produced the same green-for-nothing tick for the same
// reason. Reusing the variable rather than adding EASYWALL_REQUIRE_FORWARD
// beside it was deliberate: the variable's job is "a skip in this job is a
// failure", which is exactly what both callers want, and a second variable
// would need a second wiring in test.yml and a second guard keeping the two
// spellings the same. If the name is ever made to match the scope, it has to
// change in test.yml, build.yml and TestTheCIProofGateIsStillWiredUp together.
```

- [ ] **Step 4: Prove the gate fires**

```bash
# With the variable set, an absent precondition must fail rather than skip.
sudo EASYWALL_REQUIRE_SELFTEST=1 go test -tags integration -run 'TestIntegration_Forward_' -v ./internal/core/ 2>&1 | tail -20
```

If the precondition holds on this host the four tests pass, which proves nothing about the gate. In that case force the precondition false for one run:

```bash
sed -i 's|^\tif !r.reachable() {|\tif true { // MUTATION: pretend the harness is unreachable|' internal/core/nftables_forward_test.go
sudo EASYWALL_REQUIRE_SELFTEST=1 go test -tags integration -run TestIntegration_Forward_DockerNetworksAreRouted ./internal/core/ 2>&1 | tail -6
# Expected: FAIL — "nothing was proven here, and EASYWALL_REQUIRE_SELFTEST is set"
sudo go test -tags integration -run TestIntegration_Forward_DockerNetworksAreRouted ./internal/core/ 2>&1 | tail -4
# Expected: SKIP, and the job would still be green — which is the defect
git checkout internal/core/nftables_forward_test.go
```

Then re-apply Step 2 (the `git checkout` reverted it) and re-run Step 2's edit before committing. Better: make the mutation on a copy, or stash your change instead of checking the file out.

- [ ] **Step 5: Commit**

```bash
go test ./internal/core/ 2>&1 | tail -3
git add internal/core/nftables_forward_test.go internal/core/selftest_required_test.go
git commit -m "$(cat <<'MSG'
test(core): the four forward tests stop being green for nothing in CI

Their shared precondition — the two sides reaching each other before any
firewall exists — was a t.Skip, and a skip whose precondition is absent reads
exactly like a pass. 2.17 built skipOrFailUnprovable for that class and it sits
in the same package. EASYWALL_REQUIRE_SELFTEST is reused rather than doubled:
its job is "a skip in this job is a failure", which is what both callers want.
The scope now exceeds the name, which is recorded in the helper's comment.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

---

### Task 8: The peer says why it could not dial

Closes *`RunPeer` collapses every dial error into `blocked`* — the first of the approved §5 contract changes.

`internal/core/selftest.go:268-283` (`inboundCrosses`) separates ECONNREFUSED from a timeout from anything else, and its comment says exactly why: *"Anything else — unreachable, a bind failure, an ICMP error the kernel turned into EHOSTUNREACH — is the harness and not a verdict, so it is an error."* The peer side does not. `internal/core/netns.go:81-83` is:

```go
		if err != nil {
			_, _ = fmt.Fprintln(stdout, "blocked")
			continue
		}
```

So a harness that breaks between claim 1's control and its measurement records `failed` — the one inversion `foldClaims`'s own doc comment forbids. The reachable trigger is a second concurrent `selftest`: `wire()` takes no lock and `deleteLink` is unconditional (`netns.go:263`). **Measured: three runs of two concurrent `RunSelftest()` gave all six `unprovable`** — the setup collision wins the race every time, so today the inversion is prevented by accident rather than by design.

The pipe protocol already has a word for *not a verdict*: `unparsed`, documented in `netns.go:21-35`. This adds a second, carrying a reason.

**Files:**
- Modify: `internal/core/netns.go:21-35` (the protocol table in the header comment), `:79-86` (`RunPeer`'s dial), `:350-357` (`Harness.Dial`'s verdict switch)
- Create: `internal/core/netns_verdict_test.go` — a plain unit test, no build tag, no kernel
- Read for context: `internal/core/selftest.go:251-283` (`inboundCrosses`, the classification to mirror)

**Interfaces:**
- Produces: `peerVerdict(err error) string` — `"open"` for a nil error, `"blocked"` for a timeout, `"failed <reason>"` for anything else. Called by `RunPeer`; its output is the line the parent reads.

- [ ] **Step 1: Write the failing unit test**

Create `internal/core/netns_verdict_test.go`:

```go
package core

import (
	"errors"
	"net"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestPeerVerdictNeverCallsAHarnessFaultAVerdict is the peer-side half of the
// classification inboundCrosses has made since 2.17.
//
// The parent separates ECONNREFUSED from a timeout from anything else,
// precisely so a harness fault is never reported as a verdict. The peer used
// to collapse every dial error into "blocked" — so a harness that broke
// between claim 1's control and its measurement recorded `failed`, which is
// the one inversion foldClaims' doc comment forbids.
//
// It is prevented today by accident and not by design: two concurrent
// RunSelftest() calls collide in wire(), which takes no lock and deletes
// ewst-r unconditionally, and the collision wins the race every time —
// measured as all six claims unprovable, three runs of two.
func TestPeerVerdictNeverCallsAHarnessFaultAVerdict(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"a completed handshake is open", nil, "open"},
		{"a timeout is the only thing a dropping chain produces", timeoutError{}, "blocked"},
		{"a refusal is not a drop", syscall.ECONNREFUSED, "failed"},
		{"an unreachable network is the harness", syscall.ENETUNREACH, "failed"},
		{"an unreachable host is the harness", syscall.EHOSTUNREACH, "failed"},
		{"anything nobody thought of is the harness", errors.New("something else"), "failed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := peerVerdict(tc.err)
			if tc.want == "failed" {
				if !strings.HasPrefix(got, "failed ") {
					t.Fatalf("peerVerdict(%v) = %q — a harness fault must not be a verdict", tc.err, got)
				}
				if strings.ContainsAny(got, "\n\r") {
					t.Errorf("the reason spans lines: %q — the pipe protocol is one line each way", got)
				}
				return
			}
			if got != tc.want {
				t.Errorf("peerVerdict(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

// TestTheParentRefusesToReadAFailureAsAVerdict is the other end of the wire.
func TestTheParentRefusesToReadAFailureAsAVerdict(t *testing.T) {
	crossed, err := interpretPeerLine("failed connect: network is unreachable")
	if err == nil {
		t.Fatal("a harness failure was read as a verdict")
	}
	if crossed {
		t.Error("a harness failure reported a packet crossing")
	}
	if !strings.Contains(err.Error(), "network is unreachable") {
		t.Errorf("the error drops the peer's reason, which is the whole point: %v", err)
	}

	if crossed, err := interpretPeerLine("open"); err != nil || !crossed {
		t.Errorf(`interpretPeerLine("open") = %v, %v — want true, nil`, crossed, err)
	}
	if crossed, err := interpretPeerLine("blocked"); err != nil || crossed {
		t.Errorf(`interpretPeerLine("blocked") = %v, %v — want false, nil`, crossed, err)
	}
	if _, err := interpretPeerLine("unparsed"); err == nil {
		t.Error(`"unparsed" was read as a verdict`)
	}
}

// timeoutError is a net.Error that reports a timeout, which is what a dial
// against a dropping chain produces.
type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

var _ net.Error = timeoutError{}

var _ = time.Second // keep the import if the file ends up not needing it
```

Drop the trailing `var _ = time.Second` line and the `time` import if nothing else in the file uses them.

- [ ] **Step 2: Run it and watch it fail to compile**

```bash
go test ./internal/core/ -run 'TestPeerVerdict|TestTheParentRefuses' 2>&1 | tail -5
```

Expected: FAIL — `undefined: peerVerdict`, `undefined: interpretPeerLine`.

- [ ] **Step 3: Add the two functions and use them**

In `internal/core/netns.go`, add beside `RunPeer`:

```go
// peerVerdict maps a dial error to the one word the parent reads.
//
// It mirrors inboundCrosses' classification, and for the same reason that
// comment gives: a timeout is the only thing a dropping chain produces, so it
// is the verdict `blocked`. Anything else — a refusal, an unreachable network,
// an ICMP error the kernel turned into EHOSTUNREACH — is the harness, and
// reporting it as a verdict would let a broken harness record `failed` against
// a firewall that is working. foldClaims' doc comment forbids exactly that.
//
// The reason travels back so the parent can say what happened rather than
// "not a verdict". It is flattened to one line because the pipe protocol is
// one line each way, by design: a hung peer is then a read deadline rather
// than a parser.
func peerVerdict(err error) string {
	if err == nil {
		return "open"
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return "blocked"
	}
	return "failed " + strings.Join(strings.Fields(err.Error()), " ")
}

// interpretPeerLine turns the peer's line into a verdict or an error.
//
// Split out of Harness.Dial so both ends of the protocol are testable without
// a namespace: the classification is the part that has been wrong, and it
// needs no kernel to check.
func interpretPeerLine(line string) (bool, error) {
	switch {
	case line == "open":
		return true, nil
	case line == "blocked":
		return false, nil
	case strings.HasPrefix(line, "failed "):
		return false, fmt.Errorf("the peer could not dial: %s", strings.TrimPrefix(line, "failed "))
	default:
		return false, fmt.Errorf("the peer answered %q, which is not a verdict", line)
	}
}
```

Then replace `RunPeer`'s dial block (`netns.go:79-86`) with:

```go
		d := net.Dialer{Timeout: time.Duration(ms) * time.Millisecond}
		conn, err := d.Dial("tcp", net.JoinHostPort(addr, strconv.Itoa(port)))
		if err == nil {
			_ = conn.Close()
		}
		_, _ = fmt.Fprintln(stdout, peerVerdict(err))
```

And replace `Harness.Dial`'s switch (`netns.go:350-357`) with:

```go
	return interpretPeerLine(line)
```

Add `strings` to the imports if it is not already there.

- [ ] **Step 4: Update the protocol table in the header comment**

`internal/core/netns.go:21-35` documents the protocol. Add the fourth line and say why:

```go
// The pipe protocol, one line each way, so a hung peer is a read deadline
// rather than a parser:
//
//	child -> parent   "ready\n"                     once, after clone+exec
//	parent -> child   "dial 10.77.9.1 12227 2000\n" address, port, milliseconds
//	child -> parent   "open\n" | "blocked\n"        the verdict
//	child -> parent   "unparsed\n"                  not a verdict; see below
//	child -> parent   "failed <reason>\n"           not a verdict: the dial
//	                                                could not be made at all
//	parent -> child   (pipe closed)                 child exits
//
// "unparsed" exists because "I could not read your line" and "the firewall
// dropped it" must not be the same word. It is unreachable today — the parent
// is the only writer and always formats a well-formed line — but a release
// whose subject is not lying about the firewall's state cannot have a parse
// failure spelling itself as a verdict. Dial's default branch turns it into an
// error, which is what it is.
//
// "failed" exists for the same reason one layer down, and it is reachable.
// Every dial error used to come back as "blocked", so a harness that broke
// between a claim's control and its measurement recorded `failed` against a
// working firewall — the one inversion foldClaims forbids. A timeout stays
// "blocked" because a dropping chain can produce nothing else; a refusal, an
// unreachable network and an ICMP error are the harness, and they now say so.
// See peerVerdict, which mirrors inboundCrosses' classification on this side.
```

- [ ] **Step 5: Run everything and prove the mutation**

```bash
go test ./internal/core/ -run 'TestPeerVerdict|TestTheParentRefuses' -v
# Expected: PASS

sed -i 's|^	return "failed " + strings.Join(strings.Fields(err.Error()), " ")|	return "blocked" // MUTATION: the old collapse|' internal/core/netns.go
go test ./internal/core/ -run TestPeerVerdict 2>&1 | tail -6
# Expected: FAIL on four subtests — refusal, ENETUNREACH, EHOSTUNREACH, anything else
git checkout internal/core/netns.go
```

Re-apply Steps 3 and 4 after the checkout, or stash rather than check out.

- [ ] **Step 6: Run the integration suite, which is what actually uses the wire**

```bash
sudo go test -tags integration -run 'TestIntegration_Selftest|TestSelftest' -v ./internal/core/ 2>&1 | tail -25
```

Expected: no change in outcome — every claim that was provable stays provable. If a claim flips to `unprovable` that is a genuine finding: the peer is now admitting a harness fault it used to report as `blocked`. Read the detail before concluding anything, and report it.

- [ ] **Step 7: Commit**

```bash
git add internal/core/netns.go internal/core/netns_verdict_test.go
git commit -m "$(cat <<'MSG'
fix(core): the self-test peer says why it could not dial

RunPeer collapsed every dial error into "blocked", so a harness that broke
between a claim's control and its measurement recorded `failed` against a
working firewall — the one inversion foldClaims' doc comment forbids. The
parent side has separated a refusal from a timeout from anything else since
2.17 and says why in inboundCrosses' comment; this mirrors that classification
on the peer side and carries the reason back over the pipe.

Measured before: three runs of two concurrent RunSelftest() gave all six claims
unprovable, because wire() takes no lock and deletes ewst-r unconditionally —
so the inversion was prevented by the setup collision winning the race, not by
design.

Both ends of the classification are now unit-testable without a namespace.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

---

### Task 9: `GET_HEALTH`'s comments stop claiming what the code does not do

Closes *`GET_HEALTH` is documented as short-deadline because it is read-only, and both its netlink reads take the nft mutex*.

**This task does not do what the spec's §5 proposed, and the reason is two measurements taken while planning it. Read this section before starting.**

§5 proposed moving `CmdGetHealth` out of `CommandTimeout`'s default branch. Two facts make that the wrong change:

| Measured | Consequence |
|---|---|
| `Dockerfile:156` is `HEALTHCHECK --interval=10s --timeout=5s --start-period=15s --retries=3` | Docker abandons the probe at **5 s** whatever the core's deadline is. A 35 s deadline would change nothing for the only documented consumer, and would make `/healthz` hold a request for 35 s for every other caller |
| `internal/shared/protocol_test.go:15` is `TestCommandTimeoutKeepsGetHealthShort`, which **pins the 5 s deadline deliberately** | Moving the command would mean inverting a guard written on purpose in 2.17. Its own comment says a five-second poll that silently became thirty-five "would be invisible to every caller: this test is what turns that drift into a failure" |

So the deadline is right and stays. What is wrong is that **three comments state a reason that is false**, and one of them is that guard test's — which makes it a test passing for the wrong reason, the class this repository hunts:

- `internal/shared/protocol.go:119-122` — *"Read-only — two netlink reads and one file read — so it keeps the short deadline"*
- `internal/shared/protocol_test.go:8-9` — *"nothing that queues behind the nft mutex the way IMPORT_RULES and VALIDATE_CUSTOM do"*
- `internal/web/handler_health.go` — the handler's own framing of read-only as non-blocking

**Both reads take the mutex:** `Enforcing()` at `internal/core/nftables.go:346-347` and `RuleCounters()` at `:415-416` both open with `m.mu.Lock()`, and `Apply` holds that lock across `applyCustomRules`' nft subprocess for up to `NftTimeout` (30 s). During a slow custom-rules apply, `/healthz` therefore answers 503 and Docker's third retry lands inside that window. **Nothing restarts on unhealthy**, so the measured cost is a wrong word in `docker ps`.

The entry closes as an honest scope stated where a reader meets it, which is what the file's own rule for a contract hole asks for.

**Files:**
- Modify: `internal/shared/protocol.go:114-127` — `CmdGetHealth`'s comment
- Modify: `internal/shared/protocol_test.go:8-18` — the guard's comment, and its name
- Modify: `internal/web/handler_health.go` — the handler comment
- Modify: `docs/_docs/features/health.md` — the operator-facing sentence
- Read for context: `internal/core/nftables.go:346`, `:415`; `internal/shared/protocol.go:40-64` (`CommandTimeout`'s own rule, *"the client must wait as long as the server might"*)

**Interfaces:**
- Consumes: nothing
- Produces: nothing. `CommandTimeout(CmdGetHealth)` stays 5 s

- [ ] **Step 1: Confirm both reads take the mutex, here, now**

```bash
sed -n '346,348p;415,417p' internal/core/nftables.go
grep -n 'applyCustomRules' internal/core/nftables.go | head -3
```

Expected: both functions open with `m.mu.Lock()`. If either no longer does, this task's premise has changed — stop and report rather than rewriting comments to match a guess.

- [ ] **Step 2: Correct `CmdGetHealth`'s comment**

Replace the comment at `internal/shared/protocol.go:114-127`:

```go
	// CmdGetHealth returns whether this firewall is doing what it says: three
	// facts, evaluated in order, plus the identity of the last self-test.
	//
	// It keeps the short deadline, and *not* because it is non-blocking. Both
	// of its netlink reads take the nft mutex — NftablesManager.Enforcing and
	// RuleCounters each open with m.mu.Lock() — and Apply holds that lock
	// across applyCustomRules' nft subprocess for up to NftTimeout. The
	// deadline is short anyway, for the consumer: Dockerfile's HEALTHCHECK is
	// --timeout=5s, so docker abandons the probe at five seconds whatever this
	// says, and a longer deadline would only make every other caller wait.
	//
	// The honest scope: during a slow custom-rules apply, /healthz answers 503
	// and the image's --interval=10s --retries=3 reaches its third failure
	// inside that window. Nothing restarts on unhealthy, so the cost is a
	// wrong word in `docker ps` — see features/health.md, which says so to
	// operators. CmdGetUsage avoids this by never touching netlink at all;
	// GET_HEALTH cannot, because whether the kernel is enforcing is the
	// question it exists to answer. Closing it properly needs a second source
	// of truth — a cached snapshot the way Server.statusForRender does it —
	// which is a decision about what GET_HEALTH is, not a fix to a wrong line.
	//
	// No audit entry: reading a counter is not an event, which is
	// CmdGetStatus's and CmdGetUsage's reasoning already.
	//
	// The reply carries no rule detail and no counter values. It is rendered by
	// /healthz, which is unauthenticated so that an orchestrator holding no
	// session can ask.
	CmdGetHealth CommandType = "GET_HEALTH"
```

- [ ] **Step 3: Correct the guard test's comment and rename it for what it holds**

In `internal/shared/protocol_test.go`, replace the comment and the name:

```go
// TestGetHealthKeepsTheShortDeadlineForItsConsumer holds the 5 s deadline.
//
// It was called TestCommandTimeoutKeepsGetHealthShort and it argued from a
// false premise: "two netlink reads and one file read — nothing that queues
// behind the nft mutex the way IMPORT_RULES and VALIDATE_CUSTOM do". Both
// reads do queue behind it. NftablesManager.Enforcing (nftables.go:347) and
// RuleCounters (:416) each take m.mu, and Apply holds it across an nft
// subprocess for up to NftTimeout.
//
// The deadline is still right, for a different reason: Dockerfile's
// HEALTHCHECK is --timeout=5s, so the only documented consumer gives up at
// five seconds regardless, and a longer deadline would buy nothing there while
// making every other caller wait. What the old comment got right is the drift
// it catches — a five-second poll that silently became thirty-five would be
// invisible to every caller.
//
// What this test does NOT assert is that the call cannot block. It can. See
// CmdGetHealth's own comment for the honest scope and what closing it needs.
func TestGetHealthKeepsTheShortDeadlineForItsConsumer(t *testing.T) {
	if got, want := CommandTimeout(CmdGetHealth), 5*time.Second; got != want {
		t.Errorf("CommandTimeout(CmdGetHealth) = %s, want %s", got, want)
	}
}
```

Then check nothing else names the old test:

```bash
grep -rn 'TestCommandTimeoutKeepsGetHealthShort' . --include='*.go' --include='*.md' --include='*.yml' | grep -v node_modules
```

`docs-tech/invariants.md` almost certainly lists it. Update that row too — it is part of this task, not Task 28's.

- [ ] **Step 4: Say it to operators**

In `docs/_docs/features/health.md`, add one short paragraph where the endpoint's behaviour is described. Keep it to the fact and its consequence — this is a published reference page, and the prose checker enforces sentences under 30 words:

```markdown
`/healthz` reads the kernel, so it waits for an apply that is in progress. A
custom-rules apply can hold that for up to 30 seconds. The endpoint answers
503 during it, and the container's health check reaches its third failure
inside that window. Nothing restarts: an unhealthy container is a label, not
an action.
```

Check the surrounding heading first and put it where a reader meets the endpoint, not at the end.

- [ ] **Step 5: Run the gates**

```bash
go test ./internal/shared/ ./internal/web/ 2>&1 | tail -4
npm run check:prose 2>&1 | tail -5
npm run check:docs 2>&1 | tail -5
```

All green. `check:prose` will reject a sentence over 30 words in the new paragraph — fix the sentence, not the checker's bound.

- [ ] **Step 6: Commit**

```bash
git add internal/shared/protocol.go internal/shared/protocol_test.go internal/web/handler_health.go docs/_docs/features/health.md docs-tech/invariants.md
git commit -m "$(cat <<'MSG'
docs(core): GET_HEALTH's deadline is right and its stated reason was not

Three comments claimed the command does not queue behind the nft mutex.
Enforcing (nftables.go:347) and RuleCounters (:416) both take m.mu, and Apply
holds it across an nft subprocess for up to NftTimeout — so one of the three
was a guard test passing for the wrong reason.

The 5 s deadline stays, and the spec's proposal to lengthen it is declined on
measurement: Dockerfile's HEALTHCHECK is --timeout=5s, so docker abandons the
probe at five seconds whatever the core's deadline says, and 35 s would only
make every other caller wait. The honest scope — 503 during a slow apply, a
third retry inside the window, nothing restarting on unhealthy — is now stated
in the command's comment, in the guard's comment, and to operators in
features/health.md.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

---

### Task 10: The self-test harness checks for a collision

Closes *`10.77.9.0/24`, `ewst-r` and `ewst-p` are created in the host namespace with no collision check* — the second approved §5 contract change.

`internal/core/netns.go:248-263` deletes any host interface called `ewst-r`, whoever made it, and its comment argues that correctly: a per-pid name would collide one step later on `10.77.9.1/24` anyway, because `addAddress` is `Create|Excl` too. What nothing checks is whether the host **already uses** the range or the peer name. **Measured: `easywall-core selftest` against a live host table reports `unprovable` with an accurate detail, never `failed`, 2 of 2 runs** — the ordinary case is honest. This hardens an honest path rather than fixing a dishonest one.

Two collisions are worth naming, and the third is deliberately left alone:

| | |
|---|---|
| **The range** `10.77.9.0/24` held by another interface | The case that harms a real operator: a LAN on 10.77.9.0/24. Refused with a sentinel, reported as `unprovable` with the interface named |
| **`ewst-p` on the host** | `createVethPair` would fail `EEXIST`, which does not satisfy `errors.Is(err, ErrNamespaceUnavailable)` — so the self-test reports a broken harness rather than *unprovable*, which is what `ewst-r`'s deletion exists to prevent for the other end |
| **`ewst-r` on the host** | **Left exactly as it is.** Its unconditional deletion is documented, reasoned and load-tested: a previous run's router end survives its own namespace by ~110 ms and longer under load. Adding a check here would break the case the deletion was written for |

The read is `net.Interfaces()` from the standard library, not netlink. `netns.go`'s header rule — *"Everything here is netlink and pipes"* — is about the privileged write path and the refusal to exec external binaries; a read of this host's own interface list is neither.

**Files:**
- Modify: `internal/core/netns.go` — add `ErrHarnessRangeInUse`, `harnessCollision()`, call it in `wire()` before the `deleteLink`
- Create: `internal/core/netns_collision_test.go` — unit test, no build tag
- Read for context: `internal/core/netns.go:101-121` (the constants), `:240-270` (`wire`), `:191-198` (`NewHarness`, where a sentinel becomes *unprovable*)

**Interfaces:**
- Produces:
  - `var ErrHarnessRangeInUse = errors.New("the self-test's address range or interface name is already in use on this host")`
  - `func harnessCollision(ifaces []net.Interface, addrsOf func(net.Interface) ([]net.Addr, error)) (string, error)` — returns a human-readable description of the collision, or `""` when there is none. Taking the interface list and an address accessor as parameters is what makes it testable without touching the host.

- [ ] **Step 1: Write the failing unit test**

Create `internal/core/netns_collision_test.go`:

```go
package core

import (
	"net"
	"strings"
	"testing"
)

// fakeAddr is a net.Addr carrying a CIDR string, which is what
// net.Interface.Addrs returns in practice (*net.IPNet).
func cidr(t *testing.T, s string) net.Addr {
	t.Helper()
	_, ipnet, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatalf("ParseCIDR(%q): %v", s, err)
	}
	return ipnet
}

// TestHarnessCollisionNamesWhatItFound covers the case nothing checked.
//
// The harness range and both interface names are fixed constants, and wire()
// deletes any host ewst-r unconditionally — which is right, and documented:
// a per-pid name would collide one step later on 10.77.9.1/24 because
// addAddress is Create|Excl. What was missing is the host that already uses
// the range. An operator whose LAN is 10.77.9.0/24 got a harness failure
// instead of "unprovable", on a release whose subject is not lying about the
// firewall's state.
//
// ewst-r is deliberately NOT a collision here: its deletion is what makes a
// second NewHarness in the same process work at all, since the previous
// router end outlives its namespace by about 110 ms.
func TestHarnessCollisionNamesWhatItFound(t *testing.T) {
	tests := []struct {
		name     string
		ifaces   []net.Interface
		addrs    map[string][]net.Addr
		wantHit  bool
		wantText string
	}{
		{
			name:   "an ordinary host is no collision",
			ifaces: []net.Interface{{Name: "lo"}, {Name: "eth0"}},
			addrs: map[string][]net.Addr{
				"lo":   {cidr(t, "127.0.0.1/8")},
				"eth0": {cidr(t, "192.168.1.10/24")},
			},
			wantHit: false,
		},
		{
			name:   "a LAN on the harness range collides",
			ifaces: []net.Interface{{Name: "eth0"}},
			addrs: map[string][]net.Addr{
				"eth0": {cidr(t, "10.77.9.40/24")},
			},
			wantHit:  true,
			wantText: "eth0",
		},
		{
			name:   "the harness's own router end is not a collision",
			ifaces: []net.Interface{{Name: harnessRouterIf}},
			addrs: map[string][]net.Addr{
				harnessRouterIf: {cidr(t, "10.77.9.1/24")},
			},
			wantHit: false,
		},
		{
			name:     "a host interface called ewst-p collides",
			ifaces:   []net.Interface{{Name: harnessPeerIf}},
			addrs:    map[string][]net.Addr{harnessPeerIf: nil},
			wantHit:  true,
			wantText: harnessPeerIf,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := harnessCollision(tc.ifaces, func(i net.Interface) ([]net.Addr, error) {
				return tc.addrs[i.Name], nil
			})
			if err != nil {
				t.Fatalf("harnessCollision: %v", err)
			}
			if tc.wantHit && got == "" {
				t.Fatal("a collision was not reported; the self-test would fail rather than report unprovable")
			}
			if !tc.wantHit && got != "" {
				t.Fatalf("a collision was reported where there is none: %q", got)
			}
			if tc.wantHit && !strings.Contains(got, tc.wantText) {
				t.Errorf("the detail does not name what collided: %q, want it to mention %q", got, tc.wantText)
			}
		})
	}
}
```

- [ ] **Step 2: Run it and watch it fail to compile**

```bash
go test ./internal/core/ -run TestHarnessCollisionNamesWhatItFound 2>&1 | tail -4
```

Expected: FAIL — `undefined: harnessCollision`.

- [ ] **Step 3: Implement it**

Add to `internal/core/netns.go`:

```go
// ErrHarnessRangeInUse is the sentinel for "this host already uses what the
// harness needs", which — like ErrNamespaceUnavailable — is a statement about
// the host and not about the firewall.
//
// Callers report "unprovable" with the detail, never "failed". A self-test
// that called an operator's own 10.77.9.0/24 LAN a broken firewall would be
// the exact inversion this release's peer-side fix also closes.
var ErrHarnessRangeInUse = errors.New("the self-test's address range or interface name is already in use on this host")

// harnessCollision reports what on this host stands in the harness's way, or
// "" when nothing does.
//
// Two things are checked and one deliberately is not:
//
//	the range   an interface other than ewst-r holding an address inside
//	            10.77.9.0/24 — an operator whose LAN is that range
//	ewst-p      a host interface with the peer's name, which would make
//	            createVethPair fail EEXIST rather than ErrNamespaceUnavailable
//	ewst-r      NOT checked. wire() deletes it unconditionally and must: a
//	            previous run's router end outlives its own namespace by about
//	            110 ms, and a check here would break the case that deletion
//	            was written for. See wire's own comment.
//
// The interface list and the address accessor are parameters so this is
// testable without touching the host.
func harnessCollision(ifaces []net.Interface, addrsOf func(net.Interface) ([]net.Addr, error)) (string, error) {
	harnessNet := netip.PrefixFrom(harnessRouterAddr, harnessPrefix).Masked()
	for _, iface := range ifaces {
		if iface.Name == harnessPeerIf {
			return fmt.Sprintf("a host interface is already called %s", harnessPeerIf), nil
		}
		if iface.Name == harnessRouterIf {
			continue
		}
		addrs, err := addrsOf(iface)
		if err != nil {
			// Not fatal: an interface that will not report its addresses is not
			// evidence of a collision, and refusing the self-test over it would
			// trade a false "unprovable" for a real measurement.
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip, ok := netip.AddrFromSlice(ipnet.IP)
			if !ok {
				continue
			}
			if harnessNet.Contains(ip.Unmap()) {
				return fmt.Sprintf("%s holds %s, inside the harness range %s",
					iface.Name, ip.Unmap(), harnessNet), nil
			}
		}
	}
	return "", nil
}
```

Then call it in `wire()`, immediately **before** the `deleteLink(local, harnessRouterIf)` at `netns.go:263`:

```go
	// Before anything is created: does this host already use what the harness
	// needs? Nothing checked, and the failure was not a clean "unprovable" —
	// createVethPair's EEXIST does not satisfy errors.Is(err,
	// ErrNamespaceUnavailable), so an operator whose LAN is 10.77.9.0/24 was
	// told the harness was broken. Measured before this check: the ordinary
	// case was honest anyway, reporting unprovable with an accurate detail 2 of
	// 2 runs against a live host table — so this hardens an honest path rather
	// than fixing a dishonest one.
	ifaces, err := net.Interfaces()
	if err == nil { // a host that will not list its interfaces is not evidence of a collision
		if hit, cerr := harnessCollision(ifaces, func(i net.Interface) ([]net.Addr, error) {
			return i.Addrs()
		}); cerr == nil && hit != "" {
			return fmt.Errorf("%w: %s", ErrHarnessRangeInUse, hit)
		}
	}
```

- [ ] **Step 4: Make the sentinel reach *unprovable***

`NewHarness` (`netns.go:191`) maps `EPERM`/`EINVAL` to `ErrNamespaceUnavailable`. Find where `wire`'s error reaches a caller and confirm `ErrHarnessRangeInUse` ends as *unprovable* and not as a bare error. Read `internal/core/selftest.go` around the harness construction, and if the caller only tests `errors.Is(err, ErrNamespaceUnavailable)`, add `ErrHarnessRangeInUse` to that classification — the sentinel is worthless if the caller does not read it.

Add the assertion to the test file:

```go
// TestARangeCollisionIsUnprovableAndNotAFailure pins the classification.
//
// A sentinel nothing checks is a comment. This asserts the caller treats a
// collision the way it treats an absent namespace: unprovable with a detail,
// never a verdict about the firewall.
func TestARangeCollisionIsUnprovableAndNotAFailure(t *testing.T) {
	err := fmt.Errorf("%w: eth0 holds 10.77.9.40, inside the harness range 10.77.9.0/24", ErrHarnessRangeInUse)
	if !errors.Is(err, ErrHarnessRangeInUse) {
		t.Fatal("the sentinel does not survive wrapping")
	}
	// …then assert the selftest classification function maps it to
	// shared.SelftestUnprovable. Use whatever function internal/core/selftest.go
	// exposes for that; if the mapping is inline in a prover, this test asserts
	// the sentinel's errors.Is behaviour and the prover gains its own case.
}
```

- [ ] **Step 5: Run everything**

```bash
go test ./internal/core/ -run 'TestHarnessCollision|TestARangeCollision' -v
go test ./internal/core/ 2>&1 | tail -3
sudo go test -tags integration -run 'TestIntegration_Selftest|TestSelftest' ./internal/core/ 2>&1 | tail -6
```

The integration run must still prove what it proved before — this host does not use 10.77.9.0/24, so the check is a no-op here. Verify that assumption:

```bash
ip -o addr | grep '10\.77\.9\.' || echo "the harness range is free on this host"
```

- [ ] **Step 6: Commit**

```bash
git add internal/core/netns.go internal/core/netns_collision_test.go
git commit -m "$(cat <<'MSG'
feat(core): the self-test harness refuses a host that already uses its range

Nothing checked whether 10.77.9.0/24 or ewst-p were already in use.
createVethPair's EEXIST does not satisfy errors.Is(err,
ErrNamespaceUnavailable), so an operator whose LAN is that range was told the
harness was broken rather than that nothing could be proven.

ewst-r's unconditional deletion stays exactly as it is: a previous run's router
end outlives its own namespace by about 110 ms, and a check there would break
the case the deletion was written for.

Measured before: the ordinary case was already honest — unprovable with an
accurate detail, 2 of 2 runs against a live host table — so this hardens an
honest path rather than fixing a dishonest one.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

---

### Task 11: `panic`'s timeout reads the marker

Closes *`CmdPanic`'s 35 s deadline* — the third approved §5 contract change, carried since 2.7 and deliberately re-deferred in 2.15 on the grounds that it belongs to a release looking at the console tool. This is that release, or the entry is refused rather than deferred a third time.

`cmd/easywall-core/subcommands.go:492-498`: on a `SendCommand` error, `daemonAbsent(err)` is false for a timeout — deliberately, and the reasoning is sound (*"a daemon that is slow is still a daemon, and two writers would be worse than a slow one"*). So the CLI prints *"the core daemon is not answering"* and returns `exitFailed`. But the daemon writes the marker **first**, in the first millisecond, for exactly the reason `runPanic`'s own fallback comment gives — and the teardown lands moments later. The operator is told a failure for work that succeeded.

The fix is the message, not the exit code. `exitFailed` is still right: the command did not complete as asked, and claiming success for an unconfirmed teardown would be the opposite defect.

**Files:**
- Modify: `cmd/easywall-core/subcommands.go:492-500` (`runPanic`'s error branch)
- Modify: `cmd/easywall-core/subcommands_test.go` — one test
- Read for context: `internal/core/panicmode.go:100-110` (`PanicState(markerPath) (engaged, known bool, err error)`), `cmd/easywall-core/subcommands.go:164-179` (`daemonAbsent`, and why a timeout is not absence)

**Interfaces:**
- Consumes: `core.PanicState(markerPath string) (engaged, known bool, err error)` — **not** `core.PanicEngaged`, which answers an unreadable marker with *engaged*; that default is right where a write would start filtering and wrong here, where it would tell an operator their teardown is recorded when nobody knows
- Produces: nothing

- [ ] **Step 1: Write the failing test**

Read `cmd/easywall-core/subcommands_test.go` for how `runPanic` is driven today — there is a fixture that points `cfg.SocketPath` at something and captures `stdout`/`stderr`. Then:

```go
// TestPanicTimesOutAndSaysWhatTheMarkerHolds closes an entry carried since 2.7.
//
// daemonAbsent is false for a timeout, and that is deliberate: a slow daemon is
// still a daemon, and two writers to the table would be worse than one slow
// one. So a timeout reports "the core daemon is not answering" — while the
// daemon has already put the marker on disk in the first millisecond, for the
// same reason the no-daemon fallback below writes it first, and the teardown
// lands moments later.
//
// The operator was told a failure for work that succeeded. The exit code stays
// exitFailed, because the command did not complete as asked and claiming
// success for an unconfirmed teardown is the opposite defect. What changes is
// that the message reports what the marker says.
func TestPanicTimesOutAndSaysWhatTheMarkerHolds(t *testing.T) {
	// A socket that accepts and never answers, so SendCommand times out
	// rather than being refused: that is the branch under test, and a
	// non-existent path would take the daemonAbsent fallback instead.
	dir := t.TempDir()
	sock := filepath.Join(dir, "core.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			// Accept and hold: never read, never answer.
			t.Cleanup(func() { _ = c.Close() })
		}
	}()

	cfg := testConfigWithSocket(t, sock) // use this file's own fixture
	if err := core.EngagePanic(cfg.PanicMarkerPath()); err != nil {
		t.Fatalf("EngagePanic: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runPanic(cfg, opts{}, &stdout, &stderr)

	if code != exitFailed {
		t.Errorf("exit code %d, want exitFailed — the command did not complete as asked", code)
	}
	out := stdout.String() + stderr.String()
	if !strings.Contains(out, "panic mode is recorded") {
		t.Errorf("a timeout with the marker on disk does not say so:\n%s", out)
	}
	if !strings.Contains(out, "easywall-core status") {
		t.Errorf("the operator is not told how to find out what actually happened:\n%s", out)
	}
}
```

Use this test file's own config fixture rather than inventing one; `runPanic`'s signature is `runPanic(cfg *core.Config, _ opts, stdout, stderr io.Writer) int`. A `CommandTimeout(CmdPanic)` of 35 s makes this test slow — check whether the fixture can shorten the deadline; if it cannot, mark the test `t.Parallel()` and say in a comment that it costs 35 s, or drive the branch by calling the reporting helper directly (see Step 2, which extracts one).

- [ ] **Step 2: Implement it**

In `cmd/easywall-core/subcommands.go`, replace `runPanic`'s non-absent branch:

```go
	if err != nil {
		if !daemonAbsent(err) {
			// A timeout is not absence — daemonAbsent says so on purpose — but
			// it is also not evidence that nothing happened. The daemon writes
			// the marker before it touches the table, for the same reason the
			// fallback below does, so by the time a 35 s deadline expires the
			// marker is on disk and the teardown is landing. Reporting only
			// "not answering" told an operator their firewall was still up.
			//
			// Read with PanicState and not PanicEngaged: the helper's default
			// for an unreadable marker is "engaged", which is the safe
			// direction at a write that would start filtering and the wrong one
			// here, where it would claim a teardown nobody can confirm.
			_, _ = fmt.Fprintf(stderr, "easywall-core: the core daemon is not answering on %s: %v\n",
				cfg.SocketPath, err)
			engaged, known, _ := core.PanicState(cfg.PanicMarkerPath())
			switch {
			case known && engaged:
				_, _ = fmt.Fprintf(stderr, "easywall-core: panic mode is recorded in %s, so the "+
					"teardown was very likely started and may already be done. The rules will not "+
					"come back on a restart. Run `easywall-core status` to see where it ended up.\n",
					cfg.PanicMarkerPath())
			case known && !engaged:
				_, _ = fmt.Fprintf(stderr, "easywall-core: panic mode is NOT recorded in %s, so "+
					"nothing was taken down. This machine is still filtering. Run "+
					"`easywall-core status`, and try again once the daemon answers.\n",
					cfg.PanicMarkerPath())
			default:
				_, _ = fmt.Fprintf(stderr, "easywall-core: and %s cannot be read, so whether panic "+
					"mode was recorded is unknown. Run `easywall-core status`.\n",
					cfg.PanicMarkerPath())
			}
			return exitFailed
		}
```

- [ ] **Step 3: Run it, and mutate**

```bash
go test ./cmd/easywall-core/ -run TestPanicTimesOutAndSaysWhatTheMarkerHolds -v
# Expected: PASS

# Mutation: go back to reporting only the timeout
sed -i 's|engaged, known, _ := core.PanicState(cfg.PanicMarkerPath())|engaged, known := false, false // MUTATION|' cmd/easywall-core/subcommands.go
go test ./cmd/easywall-core/ -run TestPanicTimesOutAndSaysWhatTheMarkerHolds 2>&1 | tail -4
# Expected: FAIL — "a timeout with the marker on disk does not say so"
git checkout cmd/easywall-core/subcommands.go
```

Re-apply Step 2 after the checkout, or stash instead.

- [ ] **Step 4: Document the console behaviour**

`docs/_docs/features/panic-mode.md` (confirm the filename with `ls docs/_docs/features/`) describes what the console command reports. Add the timeout case in one or two short sentences — an operator who sees this message needs to know `status` is the next command, not a second `panic`.

- [ ] **Step 5: Commit**

```bash
go test ./cmd/... ./internal/... 2>&1 | tail -4
npm run check:prose 2>&1 | tail -3
git add cmd/easywall-core/subcommands.go cmd/easywall-core/subcommands_test.go docs/_docs/features/panic-mode.md
git commit -m "$(cat <<'MSG'
fix(cli): a panic that times out reports what the marker holds

daemonAbsent is false for a timeout on purpose — a slow daemon is still a
daemon, and two writers to the table would be worse. But the daemon writes the
marker before it touches the table, so when a 35 s deadline expires the marker
is on disk and the teardown is landing: the operator was told a failure for
work that succeeded.

The exit code stays exitFailed; claiming success for an unconfirmed teardown is
the opposite defect. PanicState and not PanicEngaged, because the helper's
"engaged" default for an unreadable marker is the safe direction at a write
that starts filtering and the wrong one here.

Carried since 2.7 and re-deferred in 2.15 as work for a release looking at the
console tool.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

---

### Task 12: Two audit labels say what they cover

Closes *`boot_enforce_failed` reads "at startup"* and *`rollback_skipped`'s label*, both carried since 2.7.

`actionLabel` (`internal/web/server.go:698`) is what the audit filter searches, so the label is the string an operator types. Both are narrower than what the action now covers:

| Action | Label today | What it also covers |
|---|---|---|
| `boot_enforce_failed` | *Rules could not be restored at startup* | A mid-apply panic teardown that failed (`internal/core/restore.go:133`, `:145`) — somebody hunting a 15:00 teardown failure must search for *startup* |
| `rollback_skipped` | *Rollback skipped — panic mode is engaged* | Both write sites (`internal/core/firewall.go:619`, `:695`) end with **the rules file reverted and the kernel not written**. *Skipped* says nothing was rolled back; `docs/_docs/features/audit-log.md:44` already explains that it was |

`docs/_docs/features/audit-log.md:33`'s **Meaning** column already lists the extra cases for `boot_enforce_failed`. Only the label and the *Reads as* column are wrong.

**Files:**
- Modify: `locales/en.json:244-250`, `locales/de.json:244-250` — `audit_boot_enforce_failed`, `audit_rollback_skipped`
- Modify: `docs/_docs/features/audit-log.md:33` (the *Reads as* cell), `:38` (the *everything else* row quotes the rollback label verbatim), and the block quote at `:42-45` if the new label makes a sentence redundant
- Read for context: `internal/core/restore.go:72`, `:133`, `:145`; `internal/core/firewall.go:612-621`, `:660-700`

**Interfaces:**
- Consumes: nothing
- Produces: nothing. The action **strings** do not change — only their translations. Changing an action string would orphan every entry already in an operator's log

- [ ] **Step 1: Read both write sites before choosing words**

```bash
sed -n '66,80p;126,150p' internal/core/restore.go
sed -n '608,624p;690,700p' internal/core/firewall.go
```

The wording has to be true of **every** site that writes the action. This is the step that makes the difference between a fix and a second wrong label.

- [ ] **Step 2: Change the four strings**

`locales/en.json`:

```json
  {"id": "audit_boot_enforce_failed", "translation": "Rules could not be restored"},
  {"id": "audit_rollback_skipped", "translation": "Rollback stored, not written — panic mode is engaged"},
```

`locales/de.json`:

```json
  {"id": "audit_boot_enforce_failed", "translation": "Regeln konnten nicht wiederhergestellt werden"},
  {"id": "audit_rollback_skipped", "translation": "Rücknahme gespeichert, nicht geschrieben — Notfallmodus ist aktiv"},
```

`boot_enforce_failed` loses *at startup* and gains nothing: the action covers a boot restore and a mid-apply teardown, and the detail already says which. Note that `audit_boot_enforced` — the green one at `:244` — **stays** *"Rules restored at startup"*, because that one really is only the boot path.

`rollback_skipped` says what both sites do: the stored rules went back, the kernel write did not happen.

These are proposals with the reasoning attached, not fixed text. If reading Step 1 shows a site the words are not true of, change the words and say which site in the commit message.

- [ ] **Step 3: Follow both strings into the published table**

In `docs/_docs/features/audit-log.md`:
- `:33` — the *Reads as* cell becomes `Rules could not be restored`
- `:38` — the *everything else* row lists `Rollback skipped — panic mode is engaged` verbatim; update it
- `:42-45` — the block quote explains that `rollback_skipped` does revert the stored rules. With the label now saying so, cut whichever sentence has become a restatement. Do not leave both

- [ ] **Step 4: Run every gate this touches**

```bash
go test ./internal/web/ -run 'Locale|Translat|ActionLabel' -v 2>&1 | tail -8
go test ./internal/... 2>&1 | tail -4
npm run check:prose 2>&1 | tail -3
npm run check:docs 2>&1 | tail -3
codespell
```

`TestLocaleFilesAreAtParity` and `TestTemplatesOnlyUseTranslatedKeys` must both stay green. Check whether any test asserts the old English text:

```bash
grep -rn 'could not be restored at startup\|Rollback skipped' --include='*.go' --include='*.mjs' . | grep -v node_modules
```

Every hit is part of this task.

- [ ] **Step 5: Render the log page in both languages**

The audit log page is where these strings appear. Start the local review server, log in, and load `/log` with entries of both kinds present — in **English and German**, both themes, at 1600 / 900 / 390 px. A longer label wraps differently in a table cell, and `Rücknahme gespeichert, nicht geschrieben — Notfallmodus ist aktiv` is 58 characters. If it breaks the column, shorten the German rather than widening the column.

- [ ] **Step 6: Commit**

```bash
git add locales/en.json locales/de.json docs/_docs/features/audit-log.md
git commit -m "$(cat <<'MSG'
fix(i18n): two audit labels say what their action actually covers

boot_enforce_failed read "at startup" and is also written when a mid-apply
panic teardown fails, so somebody hunting a 15:00 teardown failure had to
search for "startup" — actionLabel is what the audit filter searches.
rollback_skipped read "Rollback skipped" while both of its write sites leave
the stored rules reverted and only the kernel write undone; the published table
already explained that, and the label contradicted it.

Both locales, the Reads as column and the one sentence that became a
restatement. The action strings themselves are unchanged: renaming one would
orphan every entry already in an operator's log.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

---

### Task 13: G115 leaves the global excludes

Closes *Two gosec runs, one rule, different answers*, under ruling #11.

`.golangci.yml`'s gosec `excludes` list carries `G115  # int→uint conversions safe for rate-limit and length values`, so `Lint` is green on every integer narrowing in the tree. `security.yml:101` runs **standalone** gosec and uploads SARIF, applying none of golangci-lint's exclusions — so code scanning is the only path on which a G115 ever reaches a human, on a check-run named `gosec`, after the branch is already pushed. On 2.18 that let `netns.go:382` convert `cmd.Process.Pid` to a `uint32` with no bound — `-1` becomes `4294967295` — through a task review, a scoped re-review, a whole-branch review and a push.

**Measured while planning, which makes this ruling cheap:**

```
gosec -include=G115 ./...                        → 0 issues
gosec -include=G115 -tests -tags integration ./...  → 10 issues at 6 distinct sites
    internal/core/nftables_log_test.go:66
    internal/web/totp_test.go:84, :92, :93, :97, :111
```

Every one is in a `_test.go` file, which `security.yml`'s filter step already keeps out of the alert list. And all four sites the exclusion's own comment defends — `nftables.go:2389`, `:2440`, `totp.go:75`, `:118` — **already carry `#nosec G115` with their bound written beside them**. The exclusion protects nothing but six lines in two test files.

**Files:**
- Modify: `.golangci.yml` — delete the `G115` line from the gosec `excludes`
- Modify: `internal/core/nftables_log_test.go:66`, `internal/web/totp_test.go:84, 92, 93, 97, 111` — add `#nosec G115` with a reason
- Create or modify: a guard test in `internal/shared` asserting G115 is not globally excluded
- Read for context: `.golangci.yml:36-50` (the gosec block and the reasoning already beside each exclusion), `.github/workflows/security.yml:95-110`

**Interfaces:**
- Produces: nothing

- [ ] **Step 1: Take the baseline yourself**

```bash
~/go/bin/gosec -include=G115 -fmt=json ./... 2>/dev/null | python3 -c "import json,sys; print('prod:', len(json.load(sys.stdin).get('Issues',[])))"
~/go/bin/gosec -include=G115 -tests -tags integration -fmt=json ./... 2>/dev/null | python3 -c "
import json,sys,collections
iss=json.load(sys.stdin)['Issues']
c=collections.Counter((i['file'].split('easywall/')[-1], i['line']) for i in iss)
print('with tests:', len(iss), 'at', len(c), 'sites')
for (f,l),n in sorted(c.items()): print(f'  {n}x {f}:{l}')
"
```

Expected: `prod: 0`, and six sites with tests. **If a production site appears, stop.** That is a real finding this exclusion was hiding, and it is fixed before the exclusion is removed — not after.

- [ ] **Step 2: Annotate the six test sites**

Read each line, then put a `#nosec G115` comment on the line above with the reason that line is safe. Do not paste one generic reason six times — `totp_test.go`'s five are about a step counter derived from a fixed timestamp, and `nftables_log_test.go:66` is about something else. Example shape:

```go
	// #nosec G115 -- a fixed test timestamp divided by 30; the value is known at
	// compile time and nowhere near the uint64 ceiling.
	step := uint64(ts / 30)
```

- [ ] **Step 3: Delete the exclusion**

In `.golangci.yml`, remove:

```yaml
        - G115  # int→uint conversions safe for rate-limit and length values
```

and add a line above the `excludes` list recording why it is gone:

```yaml
      # G115 is deliberately NOT excluded. It was, and the exclusion meant the
      # only path on which an integer narrowing reached a human was standalone
      # gosec in security.yml — which applies none of these exclusions — on a
      # check run after the branch was already pushed. 2.18's netns.go:382
      # converted a pid to uint32 with no bound and passed Lint from the line
      # being written through three reviews. Measured when it was removed: zero
      # findings in production code, because all four sites this exclusion's
      # comment defended already carry their own #nosec with a bound beside it.
      excludes:
```

- [ ] **Step 4: Prove `Lint` is green and can now see the class**

```bash
~/go/bin/golangci-lint run ./... 2>&1 | tail -20
```

Expected: clean. If `golangci-lint` refuses the target with *"the Go language version used to build golangci-lint is lower than the targeted Go version"*, that is the wrong-binary symptom from `docs-tech/local-review.md` — it is not this task failing. Then prove the rule is live:

```bash
# A deliberately unbounded narrowing, in production code
sed -i 's|^func harnessCollision|func g115Mutation(n int) uint32 { return uint32(n) }\n\nfunc harnessCollision|' internal/core/netns.go
~/go/bin/golangci-lint run ./internal/core/ 2>&1 | grep -i 'G115\|integer overflow' | head -3
# Expected: a G115 finding. Before this task it would have been silent.
git checkout internal/core/netns.go
```

Do this **after** Task 10 is committed, or on a scratch file, so the checkout does not discard Task 10's work.

- [ ] **Step 5: Add the guard so it cannot drift back**

In `internal/shared`, add a test beside the other workflow and config guards (find them with `grep -rln 'repoFile' internal/shared/*_test.go`):

```go
// TestG115IsNotGloballyExcluded keeps the two gosec runs telling one story.
//
// .golangci.yml's exclusions do not apply to the standalone gosec in
// security.yml, so a rule excluded here is a rule that only ever reaches a
// human through code scanning, after a push, on a check run named gosec. That
// divergence let 2.18 convert a pid to uint32 with no bound and pass Lint from
// the line being written through three reviews.
//
// Measured when the exclusion was removed: zero G115 findings in production
// code. All four sites the exclusion's comment defended carry their own
// #nosec with the bound beside it, which is the form that survives both runs.
func TestG115IsNotGloballyExcluded(t *testing.T) {
	cfg := repoFile(t, ".golangci.yml")
	for _, line := range strings.Split(cfg, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- G115") {
			t.Errorf("G115 is excluded in .golangci.yml (%q) — Lint then cannot see an "+
				"integer narrowing that standalone gosec in security.yml will report "+
				"after the push. Use a #nosec G115 comment with the bound beside the "+
				"line instead, the way nftables.go:2389 and totp.go:75 do", trimmed)
		}
	}
}
```

Check `repoFile`'s exact signature in that package before using it — it may take path segments rather than one string.

- [ ] **Step 6: Commit**

```bash
go test ./internal/shared/ -run TestG115IsNotGloballyExcluded -v
go test ./internal/... 2>&1 | tail -3
git add .golangci.yml internal/core/nftables_log_test.go internal/web/totp_test.go internal/shared/
git commit -m "$(cat <<'MSG'
build: G115 leaves the global gosec excludes

.golangci.yml's exclusions do not reach the standalone gosec in security.yml,
so an excluded rule only ever reached a human through code scanning, after the
push, on a check run named gosec. That is how 2.18's netns.go:382 converted a
pid to uint32 with no bound and passed Lint from the line being written through
three reviews.

Measured: zero G115 findings in production code, and ten at six sites under
CI's flags — every one in a _test.go file, which security.yml's filter already
drops. All four sites the exclusion's comment defended already carry their own
#nosec with the bound beside them. The exclusion protected six test lines,
which now carry their own reasons.

A guard in internal/shared keeps it from drifting back.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

---

### Task 14: The spelling gate covers what it is configured for

Closes *The spelling gate's scope does not match its configuration*.

`.codespellrc` is written repo-wide and `codespell` runs repo-wide, but the only job that runs it is `docs.yml`'s *Site builds*, which triggers on `docs/**`, `.github/workflows/docs.yml`, `.github/actions/**`, `CHANGELOG.md` and `.codespellrc` (`docs.yml:22-35`). A typo in `README.md`, `CONTRIBUTING.md`, `DESIGN.md`, `locales/en.json`, `internal/**/*.go`, `config/`, `debian/`, `docker/` or `systemd/` is never checked.

`test.yml` triggers on every `pull_request` with **no path filter** (`test.yml:3-6`), which is what makes it the right home. **Measured 2026-09-09 and re-confirmed while planning: `codespell` from the repository root with no paths — the whole tree, config honoured — produces zero findings and exits 0.** The trap worth preserving: passing explicit paths on the command line bypasses the skip list's `./`-prefixed entries, which is what makes the repo-wide invocation both the correct one and the cheaper one.

**Files:**
- Modify: `.github/workflows/test.yml` — add the step to the `assets` job (`:181`), first, before `setup-node`
- Modify: `.github/workflows/docs.yml:51-56` — remove the step, leave a comment saying where it went and why
- Create or modify: a guard test in `internal/shared` pinning the home
- Read for context: `.codespellrc`, `.github/workflows/docs.yml:22-56`, `.github/workflows/test.yml:181-200`

**Interfaces:**
- Produces: nothing

- [ ] **Step 1: Re-measure before moving anything**

```bash
codespell; echo "exit: $?"
```

Expected: no output, exit 0. If there are findings, **fix them in this task** — moving a gate that is red is how a branch turns red for a reason nobody connects to the change.

- [ ] **Step 2: Add the step to `test.yml`'s `assets` job**

Insert immediately after `- uses: actions/checkout@v7` in the `assets` job:

```yaml
      # codespell is a typo-only dictionary — near-zero false positives on
      # nftables, conntrack and argon2id, where hunspell floods. Configuration,
      # and the reasoning for every skip and every ignored word, is in
      # .codespellrc. First, because it needs no Ruby, no Node and no build.
      #
      # Here and not in docs.yml, where it lived until 2.19: .codespellrc is
      # written repo-wide and codespell runs repo-wide, but docs.yml triggers on
      # docs/**, CHANGELOG.md and three other paths — so a typo in README.md,
      # DESIGN.md, locales/en.json or internal/**/*.go was never checked. This
      # workflow has no path filter, which is the whole reason it is the right
      # home.
      #
      # No paths on the command line, deliberately: explicit paths bypass the
      # skip list's ./-prefixed entries in .codespellrc, so the repo-wide
      # invocation is the correct one as well as the shorter one.
      - name: Spelling
        run: pipx run codespell
```

- [ ] **Step 3: Take it out of `docs.yml` and say where it went**

Replace `docs.yml:51-56` with:

```yaml
      # Spelling moved to test.yml's `assets` job in 2.19. It ran here and this
      # workflow is path-filtered, so a repo-wide dictionary was only ever
      # applied to docs/**, CHANGELOG.md, .github/actions/** and this file.
      # Do not add it back: two copies would disagree the moment one is
      # changed, and TestTheSpellingGateRunsWhereItIsConfigured is what keeps
      # this comment honest.
```

- [ ] **Step 4: Add the guard**

In `internal/shared`, beside the other workflow guards:

```go
// TestTheSpellingGateRunsWhereItIsConfigured keeps the gate's scope and its
// configuration in agreement.
//
// .codespellrc is written repo-wide. For four releases the only job that ran
// codespell was docs.yml's, which triggers on docs/**, CHANGELOG.md,
// .github/actions/** and itself — so a typo in README.md, DESIGN.md,
// locales/en.json or any Go file was never checked, and nothing said so.
//
// It asserts two things, because either one alone permits the old state: the
// unfiltered workflow runs it, and the path-filtered one does not.
func TestTheSpellingGateRunsWhereItIsConfigured(t *testing.T) {
	testWF := repoFile(t, ".github", "workflows", "test.yml")
	if !strings.Contains(testWF, "codespell") {
		t.Error("test.yml does not run codespell. It is the workflow with no path filter, " +
			"which is what makes a repo-wide dictionary actually cover the repository")
	}
	docsWF := repoFile(t, ".github", "workflows", "docs.yml")
	for _, line := range strings.Split(docsWF, "\n") {
		if strings.Contains(line, "codespell") && !strings.HasPrefix(strings.TrimSpace(line), "#") {
			t.Errorf("docs.yml runs codespell again (%q) — it is path-filtered, so this copy "+
				"covers less than the configuration claims, and two copies disagree the "+
				"moment one is changed", strings.TrimSpace(line))
		}
	}
}
```

Match `repoFile`'s real signature — `internal/shared/docs_coverage_test.go:146` uses `repoFile(t, ".github", "workflows", "docs.yml")`.

- [ ] **Step 5: Prove the guard fires both ways**

```bash
go test ./internal/shared/ -run TestTheSpellingGateRunsWhereItIsConfigured -v
# Expected: PASS

sed -i 's|^      - name: Spelling$|      - name: Spelling DISABLED # MUTATION|; s|        run: pipx run codespell|        run: true # MUTATION|' .github/workflows/test.yml
go test ./internal/shared/ -run TestTheSpellingGateRunsWhereItIsConfigured 2>&1 | tail -4
# Expected: FAIL — "test.yml does not run codespell"
git checkout .github/workflows/test.yml
```

Re-apply Step 2 after the checkout, or stash instead.

- [ ] **Step 6: Commit**

```bash
git add .github/workflows/test.yml .github/workflows/docs.yml internal/shared/
git commit -m "$(cat <<'MSG'
ci: the spelling gate moves to the workflow with no path filter

.codespellrc is repo-wide and codespell ran repo-wide, but its only job was in
docs.yml, which triggers on docs/**, CHANGELOG.md, .github/actions/** and
itself. A typo in README.md, DESIGN.md, locales/en.json or any Go file was
never checked, and nothing said so.

Measured before the move, twice: codespell from the repository root with no
paths produces zero findings and exits 0. No paths on the command line
deliberately — explicit paths bypass the ./-prefixed entries in the skip list,
so the repo-wide invocation is the correct one as well as the shorter one.

A guard asserts both halves: the unfiltered workflow runs it, the filtered one
does not.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

---

# Phase 3 · The checks under `scripts/`

**T15 must land before T16** — both edit `scripts/ui-check.mjs`. T17, T18 and T19 are independent of everything.

---

### Task 15: `check:ui` drives the server it was told about

Closes *`ui-check.mjs` does not derive its URL from the config it already reads*. Byte-identical to `765d615`.

`scripts/ui-check.mjs:48` is `const BASE = process.env.EASYWALL_URL || 'https://127.0.0.1:12227'`. The script already resolves and parses a `web.toml` — `webConfigPath()` at `:111`, `readPasswordHash()` at `:120`, `readTOTPSecret()` at `:136` — for the password hash and the TOTP secret. It does not read `bind_addr` from the same file. So moving the demo server moves the server and leaves `check:ui` driving 12227.

**The incident this closes:** Task 14 of 2.18 reported *"UI checks passed"* against a pre-existing `easywall-web` on that port, having never loaded the stylesheet it was checking — in the check this repository trusts most.

**Files:**
- Modify: `scripts/ui-check.mjs:48`, and add one reader beside `readTOTPSecret`
- Read for context: `:96-152` (the resolver and the two existing readers, which this copies exactly), `:315` and `:410` (two sub-checks that write their own `web.toml` and spawn their own server on a chosen port — **they build their own URLs and must not be touched**)

**Interfaces:**
- Produces: `readBindAddr(configPath)` → a base URL string like `https://127.0.0.1:12227`, or `null` when the file cannot be read or has no `bind_addr`

- [ ] **Step 1: Add the reader**

Insert after `readTOTPSecret` in `scripts/ui-check.mjs`:

```js
/**
 * The base URL the running instance is actually listening on.
 *
 * Read from the same web.toml readPasswordHash and readTOTPSecret already
 * parse, because the alternative is what shipped: a hard-coded 12227 beside a
 * config the script was holding open. EASYWALL_DEMO_ADDR moves the demo
 * server, and check:ui went on driving the old port — which reported "UI
 * checks passed" against a pre-existing easywall-web, having never loaded the
 * stylesheet under test, in the check this repository trusts most.
 *
 * A bind_addr with no host means every interface; 127.0.0.1 is the address to
 * dial then, not an empty host. "[::]:12227" is the same case with brackets.
 */
function readBindAddr(configPath) {
  let text;
  try {
    text = readFileSync(configPath, 'utf8');
  } catch {
    return null;
  }
  const m = text.match(/^\s*bind_addr\s*=\s*"([^"]*)"/m);
  if (!m || !m[1]) return null;
  const raw = m[1].trim();
  const port = raw.slice(raw.lastIndexOf(':') + 1);
  if (!/^\d+$/.test(port)) return null;
  let host = raw.slice(0, raw.lastIndexOf(':'));
  if (host === '' || host === '[::]' || host === '0.0.0.0' || host === '::') host = '127.0.0.1';
  return `https://${host}:${port}`;
}
```

- [ ] **Step 2: Use it, with the override still winning**

Replace `:48`:

```js
const CONFIG_PATH = webConfigPath();
// EASYWALL_URL first — a run against something the config does not describe is
// a legitimate thing to want. Then the config, which is the fix for this line's
// own history. The literal last, so a tree with no config still runs.
const BASE = process.env.EASYWALL_URL || readBindAddr(CONFIG_PATH) || 'https://127.0.0.1:12227';
```

`webConfigPath`, `readBindAddr` and `readFileSync` must be declared before this line runs — in an ES module, `function` declarations hoist and `const` does not, so move the `const BASE` line **below** the function declarations rather than moving the functions up. Then replace every later `webConfigPath()` call with `CONFIG_PATH` so the file is read from one resolved path.

- [ ] **Step 3: Print what it decided**

Right after `BASE` is computed, add one line to the script's startup output:

```js
console.log(`check:ui → ${BASE}  (config: ${CONFIG_PATH})`);
```

The incident was a check that did not say what it was checking. One line makes the next occurrence visible in the log instead of invisible in a pass.

- [ ] **Step 4: Prove it, three ways**

```bash
# 1. The demo's own config decides the port
scripts/demo-server.sh &
sleep 3
npm run check:ui 2>&1 | head -3
# Expected: the first line names the demo's port and its web.toml

# 2. The override still wins
EASYWALL_URL=https://127.0.0.1:9999 npm run check:ui 2>&1 | head -3
# Expected: names 9999, then fails to connect — which is correct

# 3. A moved address is followed
EASYWALL_DEMO_ADDR=127.0.0.1:12345 scripts/demo-server.sh &
sleep 3
npm run check:ui 2>&1 | head -3
# Expected: names 12345. Before this task it named 12227.
```

Read `scripts/demo-server.sh` for the actual variable name and how it writes `bind_addr` — if it does not write `bind_addr` into the `web.toml` it generates, that is the other half of this task and it is fixed here.

- [ ] **Step 5: Commit**

```bash
git add scripts/ui-check.mjs
git commit -m "$(cat <<'MSG'
fix(check:ui): the URL comes from the config the script already reads

ui-check.mjs resolved and parsed a web.toml for the password hash and the TOTP
secret, and kept a hard-coded https://127.0.0.1:12227 beside it. Moving the
demo server left the check driving the old port — which is how 2.18's Task 14
reported "UI checks passed" against a pre-existing easywall-web, having never
loaded the stylesheet it was checking.

EASYWALL_URL still wins, the literal is still the last resort, and the script
now prints the URL and the config it chose: the incident was a check that did
not say what it was checking.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

---

### Task 16: `check:ui` survives a second run

Closes *`check:ui` is not re-runnable against a live demo server* and *`checkPortsCatalogue` is not idempotent* — **one finding, recorded twice**: once under 2.17's *Measured, and left alone* and once under *From the check:ui race fix*, independently rediscovered. Both rows close here.

`scripts/ui-check.mjs:698-713` picks Pi-hole from the catalogue and asserts exactly two TCP rows were added. On a second run against the same demo server the first run's rows are still there, so it reports *"picking Pi-hole added 4 TCP rows, expected 2"* — which reads as a real regression and costs a bisect. Invisible in CI, which starts a fresh `easywall-web` for every run; it bites only a maintainer whose `scripts/demo-server.sh` has been up across two invocations.

The decision, taken in the spec: **tolerate**, rather than have the check reset a maintainer's running session. A check that deletes rules somebody was looking at is the larger surprise.

**Files:**
- Modify: `scripts/ui-check.mjs:698-717`
- Read for context: the whole function, including the `#rules-json` payload assertion at `:718`

**Interfaces:**
- Consumes: `BASE` and `CONFIG_PATH` from Task 15
- Produces: nothing

- [ ] **Step 1: Count before, assert the delta**

Replace the opening of `checkPortsCatalogue`:

```js
async function checkPortsCatalogue(page) {
  await page.goto(`${BASE}/ports?type=tcp`, { waitUntil: 'networkidle' });

  // What is already there. A demo server that has been up across two
  // invocations still holds run 1's rows, and asserting an absolute count then
  // reported "picking Pi-hole added 4 TCP rows, expected 2" — which reads as a
  // real regression and costs a bisect. CI never sees it: every run there
  // starts a fresh easywall-web.
  //
  // Tolerated rather than reset. A check that deletes rules a maintainer was
  // looking at is a bigger surprise than one that counts the difference.
  const countPihole = () => page.$$eval('#rules-tbody tr[data-idx]',
    trs => trs.filter(tr => tr.dataset.service === 'pihole').length);
  const before = await countPihole();

  await page.click('#catalogue-btn');
  await page.click('.catalogue-item[data-service="pihole"]');

  const rows = await page.$$eval('#rules-tbody tr[data-idx]', trs =>
    trs.map(tr => ({
      port: tr.querySelector('.f-port')?.value,
      sources: tr.querySelector('.f-sources')?.value,
      service: tr.dataset.service,
    })));
  const added = rows.filter(r => r.service === 'pihole');
  if (added.length - before !== 2) {
    fail('ports catalogue',
      `picking Pi-hole added ${added.length - before} TCP rows, expected 2 (80, 53)` +
      (before ? ` — ${before} were already there from an earlier run` : ''));
    return;
  }
  // The rows this run added are the last two, not the first two: an earlier
  // run's rows sit ahead of them.
  const fresh = added.slice(before);
  if (!fresh[0].sources.includes('fc00::/7')) {
    fail('ports catalogue', `the private suggestion did not reach the field: "${fresh[0].sources}"`);
  }
```

Then read the rest of the function from `:718` and give the `#rules-json` assertion the same treatment — if it also asserts an absolute count or indexes from zero, it has the same defect.

- [ ] **Step 2: Prove it by running twice against one server**

```bash
scripts/demo-server.sh &
sleep 3
npm run check:ui 2>&1 | tail -5   # run 1
npm run check:ui 2>&1 | tail -5   # run 2 — this is the one that used to fail
```

Both must pass, and run 2's output must not mention four rows. Before this task, run 2 failed.

- [ ] **Step 3: Prove the check still catches a real break**

```bash
# Make the catalogue add one row instead of two
grep -n 'pihole' internal/shared/catalogue.go | head -5
# …then delete one of Pi-hole's TCP entries, re-run, and watch the check fail
# with "added 1 TCP rows, expected 2". Revert afterwards.
```

A tolerant check that tolerates everything is worse than a brittle one. This step is what proves it did not become that.

- [ ] **Step 4: Name it in the traps table**

`docs-tech/local-review.md` has a traps table (the rate limiter is in it). Add one row: a demo server kept up across two `check:ui` runs used to fail the ports catalogue, it no longer does, and the reason the check counts a delta rather than a total. This is the entry both carried rows argued for.

- [ ] **Step 5: Commit**

```bash
git add scripts/ui-check.mjs docs-tech/local-review.md
git commit -m "$(cat <<'MSG'
fix(check:ui): the ports catalogue counts what this run added

checkPortsCatalogue asserted an absolute count, so a demo server kept up across
two invocations reported "picking Pi-hole added 4 TCP rows, expected 2" — a
real-looking regression worth a bisect. CI never saw it: every run there starts
a fresh easywall-web.

It counts the delta now and names the rows an earlier run left. Tolerated
rather than reset: a check that deletes rules a maintainer was looking at is
the bigger surprise. One finding that was carried twice — 2.17 measured it and
the check:ui race fix rediscovered it — and both rows close here.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

---

### Task 17: `check:changelog` gets a second opinion

Closes *`render-changelog.mjs` is both the renderer and the checker* — the fourth approved §5 contract change.

`check:changelog` is `render-changelog.mjs --check` (`:22`), and one regex — `HEADER` at `:30` — decides both what gets written and whether what was written is right. **Measured at `b723422`: `CHANGELOG.md:108` read `## [2.15.1] —## [2.15.1] — 2026-09-07`, the generated page carried 32 `<details>` sections rather than 34, and `check:changelog` reported it current.** So 2.15.1 had no section of its own and no compare link on the site, and its entry read as part of 2.16.0's. The heading was fixed in 2.17; the sharing was not.

The closing move is a **second, independent list of versions** — derived from something other than the renderer's regex — that the check compares against.

**Files:**
- Create: `scripts/check-changelog-versions.mjs` — the independent parser
- Modify: `package.json` — `check:changelog` runs both
- Read for context: `scripts/render-changelog.mjs:22-40` (`CHECK`, `SRC`, `OUT`, `HEADER`, `LINK_DEF`), `CHANGELOG.md`'s link-definition block at the bottom

**Interfaces:**
- Produces: a script that exits non-zero with a named discrepancy. It shares **no code** with `render-changelog.mjs` — importing a helper from it would recreate the single source this task exists to break

- [ ] **Step 1: Write the independent parser**

Create `scripts/check-changelog-versions.mjs`:

```js
#!/usr/bin/env node
/**
 * A second opinion on which versions CHANGELOG.md contains.
 *
 * render-changelog.mjs is both the renderer and the checker: one regex decides
 * what gets written and whether what was written is right, so a heading
 * neither of them parses is invisible to both. Measured at b723422 —
 * CHANGELOG.md:108 read "## [2.15.1] —## [2.15.1] — 2026-09-07", the page
 * carried 32 <details> sections rather than 34, and check:changelog reported
 * it current. 2.15.1 had no section and no compare link on the site.
 *
 * This deliberately shares no code with the renderer. Three sources are
 * compared, and they are independent of each other:
 *
 *   1. every version that appears in a `## [x.y.z]` heading, found by counting
 *      bracketed tokens rather than by matching the renderer's line shape
 *   2. every version with a link definition at the bottom of the file
 *   3. every <details> section in the generated page
 *
 * Any disagreement is a version that is in the file and not on the site, or on
 * the site twice, or missing a compare link. Importing the renderer's HEADER
 * regex would put this back to one source of truth, which is the defect.
 */
import { readFileSync } from 'node:fs';

const changelog = readFileSync(new URL('../CHANGELOG.md', import.meta.url), 'utf8');
const page = readFileSync(new URL('../docs/_docs/changelog.md', import.meta.url), 'utf8');

const fail = [];

// 1. Versions in headings. A heading line is any line starting "## ", and every
//    bracketed token on it is a version claim — which is what catches a line
//    holding two of them.
const headingVersions = [];
for (const line of changelog.split('\n')) {
  if (!line.startsWith('## ')) continue;
  const tokens = [...line.matchAll(/\[([^\]]+)\]/g)].map(m => m[1]);
  if (tokens.length === 0) {
    fail.push(`a "## " heading carries no [version]: ${line.trim()}`);
    continue;
  }
  if (tokens.length > 1) {
    fail.push(`one heading line carries ${tokens.length} versions (${tokens.join(', ')}) — ` +
      `two headings ran together, and the renderer's regex reads only the first: ${line.trim()}`);
  }
  headingVersions.push(...tokens);
}

// 2. Link definitions.
const linked = new Set(
  [...changelog.matchAll(/^\[([^\]]+)\]:\s*https?:\S+/gm)].map(m => m[1]),
);

// 3. Sections on the generated page.
const sections = (page.match(/<details/g) || []).length;

const seen = new Set();
for (const v of headingVersions) {
  if (seen.has(v)) fail.push(`version ${v} appears in two headings`);
  seen.add(v);
  if (v !== 'Unreleased' && !linked.has(v)) {
    fail.push(`version ${v} has a heading and no link definition — no compare link on the site`);
  }
}

if (sections !== headingVersions.length) {
  fail.push(`CHANGELOG.md has ${headingVersions.length} version headings and the generated page ` +
    `has ${sections} <details> sections. A heading the renderer did not parse is a version with ` +
    `no section of its own, which is what 2.15.1 got.`);
}

if (fail.length) {
  console.error('check:changelog-versions failed:');
  for (const f of fail) console.error(`  - ${f}`);
  console.error('\nThis check does not share a parser with render-changelog.mjs, deliberately.');
  process.exit(1);
}
console.log(`check:changelog-versions: ${headingVersions.length} versions, ` +
  `${sections} sections, ${linked.size} link definitions — agreed`);
```

Count the `Unreleased` heading's treatment against the real file before finishing: if `CHANGELOG.md` has no `## [Unreleased]` section, drop that special case rather than leaving a branch nothing reaches.

- [ ] **Step 2: Run it on the tree as it stands**

```bash
node scripts/check-changelog-versions.mjs
```

Expected: it agrees. If it does not, **read the discrepancy before changing the script** — a real mismatch here is exactly what this task exists to surface, and adjusting the checker to match a broken file is the failure mode it is guarding against.

- [ ] **Step 3: Prove it against the historical defect**

```bash
cp CHANGELOG.md /tmp/CHANGELOG.md.bak
# Reproduce b723422's line 108: two headings run together
python3 - <<'PY'
import re
p='CHANGELOG.md'
s=open(p).read()
lines=s.split('\n')
for i,l in enumerate(lines):
    if l.startswith('## [') and i > 2:
        lines[i] = l + l   # the exact shape: "## [x] — date## [x] — date"
        print('mutated line', i+1, ':', lines[i][:70])
        break
open(p,'w').write('\n'.join(lines))
PY
node scripts/check-changelog-versions.mjs; echo "exit: $?"
# Expected: exit 1 — "one heading line carries 2 versions"
npm run check:changelog; echo "renderer's own check exit: $?"
# Expected: the old check may well still pass, which is the finding
cp /tmp/CHANGELOG.md.bak CHANGELOG.md
```

Record both exit codes in the commit message. The point of this task is the case where the two disagree.

- [ ] **Step 4: Wire it into `package.json`**

```bash
grep -n 'check:changelog' package.json
```

Change the `check:changelog` script so it runs the renderer's check **and** the new one:

```json
    "check:changelog": "node scripts/render-changelog.mjs --check && node scripts/check-changelog-versions.mjs",
```

Then check every caller — CI and any other script — still gets both:

```bash
grep -rn 'check:changelog' .github/ package.json scripts/ | grep -v node_modules
```

- [ ] **Step 5: Commit**

```bash
npm run check:changelog
git add scripts/check-changelog-versions.mjs package.json
git commit -m "$(cat <<'MSG'
feat(check): check:changelog gets a parser it does not share with the renderer

render-changelog.mjs was both the renderer and the checker: one regex decided
what gets written and whether what was written is right, so a heading neither
of them parses was invisible to both. Measured at b723422 — CHANGELOG.md:108
read "## [2.15.1] —## [2.15.1] — 2026-09-07", the page carried 32 <details>
sections rather than 34, and check:changelog reported it current. 2.15.1 had no
section of its own and no compare link on the site.

The new check compares three independent sources: bracketed tokens on every
"## " line, the link definitions at the bottom of the file, and the <details>
count on the generated page. It imports nothing from the renderer, which is the
whole point.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

---

### Task 18: The changelog page gets a version list

Closes *The changelog page has no on-page contents*, under ruling #9.

`docs/_layouts/default.html:248-258` filters headings inside `<details>` — correctly, and its own comment says why: *"A heading inside a collapsed `<details>` is not on the page as far as the reader is concerned, and an entry that scrolls to something invisible is worse than no entry."* All 30 version headings are inside one. `heads.length < 3` then drops the column. Correct by that rule's logic, and the longest page on the site is the one with no jump list.

The ruling: the changelog page gets **its own** version list, built by the renderer that already knows every version — rather than loosening a rule that is right everywhere else.

**Files:**
- Modify: `scripts/render-changelog.mjs` — emit the list
- Modify: `web/src/docs.css` — style it (and therefore `docs/assets/css/style.css` is rebuilt in Task 25)
- Read for context: `docs/_layouts/default.html:240-275` (the TOC builder, which must keep ignoring this page's headings)

**Interfaces:**
- Consumes: the version list `render-changelog.mjs` already walks to emit the sections
- Produces: a `<nav class="changelog-versions">` in the generated page

- [ ] **Step 1: Check whether a fragment opens a closed `<details>` natively**

The HTML specification requires a browser to open ancestor `<details>` elements when navigating to a fragment inside them. If that holds in the browsers this site targets, the list needs no JavaScript at all — which is the whole difference between this task being ten lines and forty.

```bash
scripts/demo-server.sh &   # any local server that serves docs/_site
# Build the site and load a version fragment directly
bundle exec jekyll build --source docs --destination docs/_site
```

Then, in the browser: open `docs/_site/docs/changelog/#2.17.0` (use the real id — read the generated page for the id the renderer puts on each section) with every section collapsed, and see whether the target section opens and scrolls. **Verify this by rendering it, in Chrome, at 1600px.** Record the answer in the commit message either way.

- [ ] **Step 2: Emit the list**

In `scripts/render-changelog.mjs`, before the sections are written, emit:

```html
<nav class="changelog-versions" aria-label="Versions">
  <a href="#2.18.0">2.18.0</a>
  <a href="#2.17.0">2.17.0</a>
  …
</nav>
```

Use the same id the renderer already puts on each `<details>` (read the file — do not invent an id scheme; an anchor to an id that does not exist is worse than no list). Keep it to version numbers only: a date beside each would double the width and the dates are visible in the sections.

If Step 1 showed that a fragment does **not** open a closed section, add a three-line inline script beside the nav — not in `default.html`, which must stay generic:

```html
<script>
  // A fragment is required to open its ancestor <details>, and where that is
  // not honoured the link scrolls to something invisible — which is the exact
  // reason default.html filters these headings out of the site-wide contents.
  addEventListener('hashchange', function () {
    var t = document.getElementById(location.hash.slice(1));
    if (t && t.tagName === 'DETAILS') t.open = true;
  });
</script>
```

- [ ] **Step 3: Style it**

In `web/src/docs.css`, add a rule for `.changelog-versions`: a wrapping row of small links, using the site's existing muted-text and border tokens — **no new colour literals**, and nothing that only works in one theme. Look at how `.docs-toc-item` is styled and stay in that vocabulary.

- [ ] **Step 4: Confirm the site-wide contents column is still absent here**

`default.html`'s `heads.length < 3` branch removes the nav **and** adds `content-full` to release the 340px gutter. That must still happen on this page — the new list is inline content, not the sidebar column. If both appear, the page has two contents lists and a wasted gutter.

Render `docs/_site/docs/changelog/` and check at **1600 / 900 / 390 px in both themes**:
- the version list wraps rather than scrolling sideways at 390px
- no empty right-hand gutter
- clicking a version opens and reaches that section

- [ ] **Step 5: Rebuild, check, commit**

```bash
npm run build:changelog
npm run check:changelog     # includes Task 17's second opinion
npm run check:docs 2>&1 | tail -5
git add scripts/render-changelog.mjs web/src/docs.css docs/_docs/changelog.md
git commit -m "$(cat <<'MSG'
feat(docs): the changelog page gets its own version list

default.html filters headings inside <details> because an entry that scrolls to
something invisible is worse than no entry, and all 30 version headings are
inside one — so heads.length < 3 dropped the column and the longest page on the
site had no jump list. The filter is right everywhere else and is not loosened.

The renderer already walks every version, so the list is generated beside the
sections it points at.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

The generated stylesheet is **not** committed here — Task 25 rebuilds both stylesheets once, at the end of Phase 4, so one task owns that diff.

---

### Task 19: The compose healthcheck stops depending on the image format

Closes *`podman build` silently drops `HEALTHCHECK`* — the fifth approved §5 contract change.

OCI is podman's default image format and has no healthcheck field, so a build **exits 0** while producing `HealthCheck: null`. `docker-compose.yml` carries a `build:` section and the documented path is `docker compose up -d`, so a podman operator following the documentation gets no health check and no error. `docs/_docs/installation/docker.md:82-92` names `--format docker`, which is a workaround rather than a fix.

The decision, taken in the spec: **accept a compose-level `healthcheck:` and give up the single-definition rule**, rather than publish an image as the documented path — which is a distribution decision this sweep should not take.

`internal/shared/healthcheck_definition_test.go:25-48` currently forbids exactly this, and its own message names *"a locally built OCI image on podman"* as one of the two cases that legitimately need one, *"a decision to make in review rather than a line to add."* This is that decision, so the guard changes shape rather than being deleted: it stops asserting **one** definition and starts asserting the **two agree**, which is the anti-drift property it was really protecting.

**Files:**
- Modify: `docker-compose.yml:60-80` — add the block, rewrite the comment
- Modify: `internal/shared/healthcheck_definition_test.go` — invert the first assertion into an agreement check
- Modify: `docs/_docs/installation/docker.md:78-93` — `--format docker` stops being required for the compose path
- Read for context: `Dockerfile:150-157` (the values to mirror), `.github/workflows/build.yml`'s health-check job (its coverage narrows — see Step 4)

**Interfaces:**
- Produces: nothing in code. The new contract is *two definitions that a test keeps identical*

- [ ] **Step 1: Add the block, mirroring the Dockerfile exactly**

`Dockerfile:156` is:

```
HEALTHCHECK --interval=10s --timeout=5s --start-period=15s --retries=3 \
  CMD wget -q --no-check-certificate -O /dev/null https://127.0.0.1:12227/healthz
```

In `docker-compose.yml`, replace the *"There is deliberately no `healthcheck:` here"* comment block with:

```yaml
    # Two definitions of one check, deliberately, since 2.19 — and a test keeps
    # them identical.
    #
    # The single definition was the better rule until it met podman. OCI is
    # podman's default image format and has no healthcheck field, so
    # `podman build` exits 0 and produces HealthCheck: null. This file carries a
    # `build:` section and the documented path is `docker compose up -d`, so an
    # operator following the documentation got no health check and no error —
    # and `--format docker` is a workaround that only helps whoever reads that
    # paragraph.
    #
    # Anything changed here changes in the Dockerfile too:
    # TestTheContainerHealthCheckHasOneDefinition asserts the two agree, field
    # by field, which is the drift the old rule was really guarding against.
    healthcheck:
      test: ["CMD", "wget", "-q", "--no-check-certificate", "-O", "/dev/null", "https://127.0.0.1:12227/healthz"]
      interval: 10s
      timeout: 5s
      start_period: 15s
      retries: 3
```

- [ ] **Step 2: Turn the guard into an agreement check**

Rewrite `internal/shared/healthcheck_definition_test.go`. Keep the name — it is in `docs-tech/invariants.md` and in the compose comment — and change what it holds:

```go
// TestTheContainerHealthCheckHasOneDefinition asserts the image's HEALTHCHECK
// and compose's healthcheck describe the same check.
//
// It used to forbid the compose block outright, and that was right until it met
// podman: OCI is podman's default image format and has no healthcheck field, so
// `podman build` exits 0 with HealthCheck: null, and docker-compose.yml carries
// a build: section while the documented path is `docker compose up -d`. An
// operator following the documentation got no check and no error. That case was
// named in this test's own old message as one requiring a decision in review;
// 2.19 took it.
//
// So there are two definitions now, and the property that matters is not that
// there is one — it is that they do not drift. Every field is compared. The
// name is unchanged because invariants.md and the compose comment both point
// at it.
func TestTheContainerHealthCheckHasOneDefinition(t *testing.T) {
	dockerfile := repoFile(t, "Dockerfile")
	compose := repoFile(t, "docker-compose.yml")

	if !namedOutsideAComment(dockerfile, "HEALTHCHECK") {
		t.Fatal("the Dockerfile declares no HEALTHCHECK instruction.\n" +
			"  A plain `docker run` is then checked by nothing. That is the state a " +
			"container was in when it stayed \"Up\" for hours with a live core and a dead " +
			"web process.")
	}
	if !namedOutsideAComment(compose, "healthcheck:") {
		t.Fatal("docker-compose.yml declares no `healthcheck:` block.\n" +
			"  It needs one: podman's default OCI format has no healthcheck field, so a " +
			"locally built image carries none and `podman build` exits 0 anyway. Compose " +
			"inheriting the image's check only works where the image has one.")
	}

	for _, f := range []struct{ name, dockerFlag, composeKey string }{
		{"interval", "--interval=10s", "interval: 10s"},
		{"timeout", "--timeout=5s", "timeout: 5s"},
		{"start period", "--start-period=15s", "start_period: 15s"},
		{"retries", "--retries=3", "retries: 3"},
	} {
		if !strings.Contains(dockerfile, f.dockerFlag) {
			t.Errorf("the Dockerfile's HEALTHCHECK %s is not %q — and compose still says %q",
				f.name, f.dockerFlag, f.composeKey)
		}
		if !strings.Contains(compose, f.composeKey) {
			t.Errorf("compose's healthcheck %s is not %q — and the Dockerfile still says %q",
				f.name, f.composeKey, f.dockerFlag)
		}
	}

	// The probe itself: same endpoint, same flags, however the two files spell
	// the argument list.
	for _, fragment := range []string{"--no-check-certificate", "https://127.0.0.1:12227/healthz"} {
		if !strings.Contains(dockerfile, fragment) {
			t.Errorf("the Dockerfile's probe does not contain %q", fragment)
		}
		if !strings.Contains(compose, fragment) {
			t.Errorf("compose's probe does not contain %q", fragment)
		}
	}
}
```

- [ ] **Step 3: Prove the agreement check fires**

```bash
go test ./internal/shared/ -run TestTheContainerHealthCheckHasOneDefinition -v
# Expected: PASS

sed -i 's|      interval: 10s|      interval: 30s # MUTATION|' docker-compose.yml
go test ./internal/shared/ -run TestTheContainerHealthCheckHasOneDefinition 2>&1 | tail -4
# Expected: FAIL — compose's interval is not "interval: 10s"
git checkout docker-compose.yml
```

Re-apply Step 1 after the checkout, or stash instead.

- [ ] **Step 4: Say what CI no longer covers**

The old guard's message warned that a compose block *"silently narrows CI: build.yml's health-check job measures the image's check, and covers compose only while compose has no check of its own."* That is now true, and it must not be silent. Read `.github/workflows/build.yml`'s health-check job and either:
- extend it to bring the container up through compose and assert the container reaches `healthy`, or
- record in `docs-tech/ci-and-release.md` that the job measures the **image's** check only, and that the compose path is covered by the agreement test rather than by a run.

Whichever is chosen, write it down. Do not leave the narrowing undocumented — the old test's message is the only place it is currently stated, and this task rewrites that message.

- [ ] **Step 5: Fix the documentation**

`docs/_docs/installation/docker.md:78-93` tells a podman operator that `--format docker` is not optional. For `podman compose up -d` that is no longer true — compose now carries its own check. For a plain `podman build` + `podman run` it still is. Rewrite the paragraph to say which path needs which, and keep the `podman image inspect` verification command: an operator who wants to know what they got should still be able to ask.

```bash
npm run check:prose 2>&1 | tail -3
npm run check:docs 2>&1 | tail -3
codespell
```

- [ ] **Step 6: Verify it with a real build, if podman is here**

```bash
which podman && podman compose version
# If present:
podman compose build 2>&1 | tail -3
podman compose up -d
sleep 20
podman ps --format '{{.Names}} {{.Status}}'
# Expected: "(healthy)" — which it was not before this task
podman compose down
```

If podman is not installed, say so in the commit message rather than claiming the path was verified.

- [ ] **Step 7: Commit**

```bash
go test ./internal/... 2>&1 | tail -3
git add docker-compose.yml internal/shared/healthcheck_definition_test.go docs/_docs/installation/docker.md docs-tech/ci-and-release.md .github/workflows/build.yml
git commit -m "$(cat <<'MSG'
fix(docker): compose declares its own health check, and a test keeps the two identical

OCI is podman's default image format and has no healthcheck field, so
`podman build` exits 0 producing HealthCheck: null. docker-compose.yml carries
a build: section and the documented path is `docker compose up -d`, so an
operator following the documentation got no health check and no error;
--format docker only helped whoever read that paragraph.

The single-definition rule is given up deliberately — the alternative is
publishing an image as the documented path, which is a distribution decision.
TestTheContainerHealthCheckHasOneDefinition keeps its name and changes what it
holds: the two definitions must agree field by field, which is the drift the
old rule was really protecting.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

---

# Phase 4 · CSS and design

T20–T24 and T26 are independent. **T25 runs last in this phase**: it rebuilds both stylesheets, so one task owns that diff instead of five tasks fighting over it.

**Nothing in this phase is verified by reading CSS.** Every task ends in a browser at **1600 / 900 / 390 px in both themes**, and the pages Phase 4 changes are the pages Task 32 re-photographs.

---

### Task 20: The callout washes become tokens

Closes *`.callout-info` is the last blue on the documentation site*, under ruling #4 — **and corrects the entry's premise**, which did not survive measurement.

The entry says info is the exception: *"hard-codes `rgba(56,189,248,…)` dark and `rgba(2,132,199,…)` light … no token"*. Measured in `web/src/docs.css`:

| | Dark wash | Light wash | Text |
|---|---|---|---|
| `.callout-info` `:1113` | `rgba(56,189,248,…)` | `rgba(2,132,199,…)` | `var(--code-builtin)` |
| `.callout-warning` `:1128` | `rgba(245,158,11,…)` | — | `var(--code-number)` |
| `.callout-success` `:1137` | `rgba(16,185,129,…)` | `rgba(5,150,105,…)` | `var(--code-string)` |

All three hard-code their wash and edge; all three already tokenise their text. The pattern is uniform, and there is no info-only exception to remove. **The blue stays** — *info* is a state, and *colour means state* does not forbid a state from having a colour. What is worth fixing is the three pairs of literals.

**Files:**
- Modify: `web/src/docs.css:1113-1148` — the three callouts, and wherever the file's `:root` tokens are declared
- Read for context: the token block at the top of `docs.css` (find it with `grep -n '^  --' web/src/docs.css | head -30`), and both `[data-theme="easywall-light"]` overrides

**Interfaces:**
- Produces: six tokens — `--callout-info-wash` / `--callout-info-edge` and the same for `warning` and `success` — declared in the light palette on bare `:root` and overridden in the dark block, matching however `docs.css` already orders its two themes

- [ ] **Step 1: Read how `docs.css` declares a themed token pair**

```bash
grep -n 'data-theme="easywall-light"' web/src/docs.css | head -5
grep -n '^:root\|^\[data-theme' web/src/docs.css | head -6
```

Follow that structure exactly. A token declared only inside a theme block is the defect `DESIGN.md` warns about — the other theme then has no value at all.

- [ ] **Step 2: Declare the six tokens and use them**

Add to the token block, keeping the existing literals as the values so **nothing changes visually**:

```css
  /* Callout washes. Three semantic states, each a wash and an edge, and each
     was a pair of rgba() literals in the rule itself until 2.19. The blue is
     not an exception to "colour means state" — info *is* a state — but a
     literal is invisible to every guard that greps for token names, which is
     what TestNoRetiredHueSurvives exists to catch one palette later. */
  --callout-info-wash: rgba(2,132,199,0.06);
  --callout-info-edge: rgba(2,132,199,0.30);
  --callout-warning-wash: rgba(245,158,11,0.08);
  --callout-warning-edge: rgba(245,158,11,0.30);
  --callout-success-wash: rgba(5,150,105,0.06);
  --callout-success-edge: rgba(5,150,105,0.30);
```

…in the light palette, and the dark values in the dark block. Then the three rules become:

```css
  .callout-info {
    background: var(--callout-info-wash);
    border-color: var(--callout-info-edge);
    color: var(--code-builtin);
  }
```

and the two `[data-theme="easywall-light"]` callout overrides **are deleted** — the tokens carry the theme now, which is the point.

Read the current values carefully: `.callout-warning` has no light override in the source, so check whether that is deliberate before inventing one. If it has none, give warning the same value in both palettes and say so in a comment rather than quietly changing it.

- [ ] **Step 3: Rebuild and prove nothing moved**

```bash
npm run build:docs-css
git diff --stat -- docs/assets/css/style.css
grep -o 'rgba(2,132,199,[0-9.]*)' docs/assets/css/style.css | sort -u
grep -o 'rgba(56,189,248,[0-9.]*)' docs/assets/css/style.css | sort -u
```

The literals must still appear in the **built** file — they are the token values. What must have changed is that they appear once each, in the token declarations, rather than in the rules. A green build is not proof: grep the built file.

Then revert the built file — Task 25 owns that commit:

```bash
git checkout docs/assets/css/style.css
```

- [ ] **Step 4: Render all three callouts, both themes**

Find a page with all three (`grep -rln 'callout-info\|callout-warning\|callout-success' docs/_docs/`) and load it at **1600 / 900 / 390 px in both themes**. The three washes must look exactly as they did before this task. If any of them shifted, a value was transcribed wrong — compare against `git show HEAD:web/src/docs.css`.

- [ ] **Step 5: Commit (source only)**

```bash
git add web/src/docs.css
git commit -m "$(cat <<'MSG'
refactor(docs-css): the three callout washes become tokens

The carried entry called .callout-info the last blue and said it carried no
token. Measured: all three callouts hard-code their wash and edge — info
rgba(56,189,248), warning rgba(245,158,11), success rgba(16,185,129) — and all
three already tokenise their text colour. The pattern was uniform; there was no
info-only exception.

The blue stays: info is a state, and colour means state does not forbid a state
from having one. What changes is that six literals become six tokens, so the
next palette guard can see them, and the two theme-scoped overrides go away
because the tokens carry the theme.

No visual change — the token values are the literals they replaced.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

---

### Task 21: `security.md` stops overflowing at 390px

Closes *`security.md` overflows at 390px*, under ruling #7 — **and the entry's stated reason for rejecting the obvious fix does not hold.**

482px against a 390px viewport, in both themes, caused by one `<code>` holding `TestLoginVerify_TheSixteenthCodeAttemptDoesNotGetThrough`: it renders 462px wide and cannot wrap. Present before the branch.

The entry rejects `overflow-wrap` because it *"would break **every** long identifier on the site at an arbitrary point, and a mid-token `proxy_set_header` may serve a firewall's reader worse than one page that scrolls sideways."* That is true of `overflow-wrap: anywhere` and of `word-break: break-all`. It is **not** true of `overflow-wrap: break-word`, which breaks a word only when it cannot fit on a line by itself. `proxy_set_header` is 16 characters and fits inside 390px; the 462px test name does not. The rejected fix was rejected for a property it does not have — **to be confirmed in the render, not on this reasoning**.

**Files:**
- Modify: `web/src/docs.css:1701-1709` (`.content-body code`) and `:1768-1771` (`.content-body pre code`, the reset that must not inherit it)
- Modify: `DESIGN.md` — it is silent on this, and ruling #7 is a rule worth writing down
- Read for context: `docs/_docs/features/security.md` (the page), `:1762-1767`'s comment about a theme-scoped override that must not come back

**Interfaces:**
- Produces: nothing

- [ ] **Step 1: Measure the overflow before touching anything**

```bash
bundle exec jekyll build --source docs --destination docs/_site
```

Load `docs/_site/docs/features/security/` at **390px in both themes** and measure the document width against the viewport. Use the page-level overflow check the repository already has if it covers this page:

```bash
grep -rn 'checkContainersDoNotOverflow\|documentDoesNotOverflow' scripts/*.mjs | head -5
```

Record the number. *482px against 390* is the carried measurement; confirm it still holds or record what it is now.

- [ ] **Step 2: Add `break-word`, and keep it out of `pre`**

```css
.content-body code {
  font-family: var(--font-mono);
  font-size: 0.85em;
  background: var(--surface-2);
  color: var(--text);
  padding: 1px 6px;
  border-radius: 4px;
  border: 1px solid var(--border);
  /* break-word and not anywhere: a word breaks only when it cannot fit on a
     line by itself. proxy_set_header is 16 characters and never breaks;
     TestLoginVerify_TheSixteenthCodeAttemptDoesNotGetThrough is 462px at this
     size and overflowed features/security.md by 92px at a 390px viewport, in
     both themes. The carried entry rejected overflow-wrap on the grounds that
     it would break every long identifier at an arbitrary point — which is
     anywhere's behaviour and break-all's, not this one's. Verified rendered,
     not reasoned. */
  overflow-wrap: break-word;
}
```

And in the `pre code` reset:

```css
.content-body pre code {
  background: none;
  border: none;
  color: inherit;
  /* A code block scrolls; it does not wrap. A broken command line is a command
     somebody pastes wrong. */
  overflow-wrap: normal;
}
```

- [ ] **Step 3: Verify by rendering, which is the whole point**

```bash
npm run build:docs-css
bundle exec jekyll build --source docs --destination docs/_site
```

At **390px, both themes**, check all four of these and record each:

| Page | What must be true |
|---|---|
| `docs/features/security/` | the document no longer exceeds the viewport; the test name wraps |
| any page with `proxy_set_header` (`grep -rln proxy_set_header docs/_docs/`) | it does **not** break mid-token |
| any page with a long `<pre>` command | it still scrolls sideways inside its own block and does not wrap |
| one page at 1600px | nothing changed at all |

If `proxy_set_header` breaks, `break-word` is not behaving as this task claims — stop, report the render, and take the entry's original position instead. Do not widen the fix.

Then revert the built stylesheet for Task 25:

```bash
git checkout docs/assets/css/style.css
```

- [ ] **Step 4: Write the rule down**

`DESIGN.md` is silent on this, which is why the entry sat under *Design decisions nobody has made yet* for five releases. Add a short entry in the typography or code section:

> An inline `<code>` may break, and only when it cannot fit a line on its own:
> `overflow-wrap: break-word`. A code **block** never wraps — it scrolls. The
> distinction is that an identifier a reader retypes must not gain a line break
> they cannot see, and a 56-character Go test name must not push a phone-width
> page sideways.

- [ ] **Step 5: Commit (source only)**

```bash
npm run check:docs 2>&1 | tail -3
git add web/src/docs.css DESIGN.md
git commit -m "$(cat <<'MSG'
fix(docs-css): an inline code chip may break when it cannot fit

features/security.md was 482px wide at a 390px viewport in both themes, on one
<code> holding a 56-character Go test name that renders 462px and cannot wrap.

The carried entry rejected overflow-wrap because it "would break every long
identifier at an arbitrary point" — which is the behaviour of anywhere and
break-all, not of break-word, where a word breaks only if it cannot fit a line
alone. Verified rendered: the test name wraps, proxy_set_header does not, and a
<pre> still scrolls rather than wrapping, which the reset now states.

DESIGN.md was silent on it, which is why the entry sat under "design decisions
nobody has made yet" for five releases. It says so now.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

---

### Task 22: The configuration reference colours its keys

Closes *The rouge theme leaves nine emitted token classes unstyled*, under ruling #8.

`web/src/docs.css:1776-1786` is the theme. Nine emitted classes have no rule, so they fall to body-text colour: `.p` (162 occurrences), `.nt` (68 — nginx directives, YAML tags), `.nl` (58 — **TOML and YAML keys**), plus `.na`, `.kc`, `.no`, `.se`, `.si`, `.sh`. In the configuration reference a key therefore renders in body-text colour while its value is coloured — the reference is wrong about its own subject.

Highlighting is otherwise used correctly: 52 fences carry a language, and the 11 that do not are output, paths and a CSP header, rightly plain. **Labelling the `$ easywall-core …` blocks `console` stays forbidden** — the copy button would take the `$` prompt with it.

Every new rule uses an existing token. No new colour literal enters the file.

**Files:**
- Modify: `web/src/docs.css:1776-1786`
- Read for context: the `--code-*` tokens already in use (`--code-keyword`, `--code-string`, `--code-builtin`, `--code-number`, `--code-error`, `--text`, `--text-muted`, `--text-subtle`)

**Interfaces:**
- Produces: nothing

- [ ] **Step 1: Confirm the nine are really emitted, and count them**

```bash
bundle exec jekyll build --source docs --destination docs/_site
for c in p nt nl na kc no se si sh; do printf '%-4s ' ".$c"; grep -o "class=\"$c\"" -r docs/_site/docs/ | wc -l; done
```

If a class emits zero, leave it out — a rule for a class nothing emits is a rule nobody can check.

- [ ] **Step 2: Add the rules, grouped by role**

```css
/* Rouge syntax highlighting — minimal token theme */
.highlight .c, .highlight .c1, .highlight .cm { color: var(--text-subtle); font-style: italic; }
.highlight .k, .highlight .kd, .highlight .kn, .highlight .kp, .highlight .kr,
.highlight .kc { color: var(--code-keyword); font-weight: 500; }
.highlight .s, .highlight .s1, .highlight .s2, .highlight .sb,
.highlight .se, .highlight .si, .highlight .sh { color: var(--code-string); }
.highlight .n, .highlight .nx, .highlight .nv { color: var(--text); }
.highlight .nf, .highlight .nc { color: var(--code-keyword); }
.highlight .nb, .highlight .bp, .highlight .no { color: var(--code-builtin); }
.highlight .o, .highlight .ow, .highlight .p { color: var(--text-muted); }
.highlight .mi, .highlight .mf, .highlight .il { color: var(--code-number); }
.highlight .err { color: var(--code-error); }

/* The name on the left of a configuration line: a TOML or YAML key (.nl, 58
   occurrences), an nginx directive or YAML tag (.nt, 68), an attribute (.na).
   All three were unstyled, so in the configuration reference a key rendered in
   body-text colour beside a coloured value — the page disagreeing with its own
   subject. One role, one token, and the same token a function name already
   uses, because "the thing being named" is what they have in common. */
.highlight .nl, .highlight .nt, .highlight .na { color: var(--code-keyword); }
```

`.p` joins the operators at `--text-muted` — punctuation should recede, and at 162 occurrences anything louder would make every block busier than its content.

- [ ] **Step 3: Render the configuration reference in both themes**

```bash
npm run build:docs-css
bundle exec jekyll build --source docs --destination docs/_site
```

Load the configuration reference (`ls docs/_docs/reference/` to find it) at **1600 / 900 / 390 px in both themes**. Check:

- a TOML key is now distinguishable from its value, in **both** themes
- a YAML block reads as key/value and not as one colour
- an nginx snippet's directives are coloured
- **no block became harder to read.** Nine new rules at once can turn a quiet block into a rainbow; if any does, cut back to `.nl`/`.nt`/`.na`/`.p` and say in the commit message which were dropped and why
- the `$ easywall-core …` blocks are **unchanged** and still plain

Then:

```bash
git checkout docs/assets/css/style.css
```

- [ ] **Step 4: Commit (source only)**

```bash
git add web/src/docs.css
git commit -m "$(cat <<'MSG'
fix(docs-css): the rouge theme colours the keys it was leaving plain

Nine emitted token classes had no rule and fell to body-text colour: .p (162
occurrences), .nt (68 — nginx directives, YAML tags), .nl (58 — TOML and YAML
keys), plus .na, .kc, .no, .se, .si and .sh. In the configuration reference a
key therefore rendered in body-text colour beside a coloured value, which is
the page being wrong about its own subject.

Every rule uses a token already in the file; no new colour literal. Verified
rendered in both themes at three widths. The $ easywall-core blocks stay
unlabelled: `console` would put the prompt inside the copy button.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

---

### Task 23: `/ports` gets its aside back above a threshold

Closes *`/ports` collapses its aside at every width, and the reasoning is width-blind*, under ruling #3.

`web/src/app.css:790-802` records the decision: the aside is *"worth having, but not worth this table's row width"*, so `.page-grid.page-grid-ports` collapses **unconditionally** rather than below the shared 1570px every other page uses. From **2.15**, when *Last used* made it a six-column table. `ports.html` is byte-identical to `765d615`.

**Measured at 1920px: the five fixed columns need 796px and `Description` absorbs 830px of slack.** A 320px rail would leave `Description` 510px, against a longest real value of *"PostgreSQL — replication peer"*. The budget was genuinely tight at 1570 and is not tight at 1920. The column widths are `.col-port` 128px (`:1757`), `.col-toggle` 94px (`:1768`), `.col-sources` 25rem (`:1775`), and `.col-flex` is `width: auto` and absorbs the remainder (`:1758`).

The ruling: **the same rule as every other page, at a higher threshold, determined by rendering.**

**Files:**
- Modify: `web/src/app.css:790-802` — replace the unconditional collapse with a media query, and rewrite the comment to record the measurement rather than the old conclusion
- Read for context: `:775-786` (why the shared breakpoint is 1570 and not ~1500 — *"measured, not estimated"*, which is the standard this task has to meet), `web/templates/ports.html:106-125` (the three aside cards)

**Interfaces:**
- Produces: nothing

- [ ] **Step 1: Find the threshold by rendering, not by adding**

Start the local review server with demo data so `/ports` has realistic rows, then widen the viewport in steps and record where the table stops being comfortable **with** the aside present. Temporarily give the page the two-column grid:

```bash
# In web/src/app.css, comment out the .page-grid-ports override, rebuild, and look.
npm run build:css
```

Check at **1920, 1800, 1700, 1650, 1600px**, both themes, with the longest real description in a row (*"PostgreSQL — replication peer"*). What you are looking for is the width at which `.col-flex` stops being able to show a full description on one line. Record the numbers — the next person needs them as much as `:775`'s comment records 1570.

- [ ] **Step 2: Write the rule at the width you measured**

```css
  /* /ports only, and no longer unconditional.
     …
     The 2.15 decision was that ports' own aside (port syntax, SSH protection,
     sources) is "worth having, but not worth this table's row width", and it
     collapsed at every width. That reasoning was width-blind. Measured at
     1920px: the five fixed columns need 796px — .col-port 128, .col-toggle 94,
     .col-sources 25rem — and .col-flex absorbs 830px of slack. A 320px rail
     leaves the description 510px, against a longest real value of
     "PostgreSQL — replication peer". The budget was genuinely tight at 1570
     and is not tight at 1920.
     So: two columns above <MEASURED>px, one below. Not the shared 1570,
     because this table needs more than the others before an aside is free;
     and not "never", because above <MEASURED>px it is free. <MEASURED> was
     found by rendering at 1920/1800/1700/1650/1600 with the longest real
     description in a row, the same way 1570 was found and for the same
     reason: a box-model estimate said ~1500 and was wrong. */
  @media (max-width: <MEASURED>px) {
    .page-grid.page-grid-ports { grid-template-columns: minmax(0, 1fr); }
  }
```

> **`<MEASURED>` is not a placeholder to leave in.** It is the one number in
> this plan that only a render can produce — the same way 1570 was produced,
> and the reason `app.css:775` says *"1570, not the ~1500 a naive box-model
> estimate suggests: measured"*. Step 1 is how you get it. A commit containing
> the literal string `<MEASURED>` is a broken stylesheet.

Replace `<MEASURED>` with the number from Step 1 in all three places. **The specificity idiom matters:** `.page-grid.page-grid-ports` is `(0,0,2,0)` and beats the shared `.page-grid` rule at `(0,0,1,0)` at every width, which is why the original needed no media query. Inside a `max-width` query it now only applies below the threshold — above it, the shared two-column rule wins. Verify that in the render rather than trusting the cascade arithmetic.

- [ ] **Step 3: Render it at every width that matters**

```bash
npm run build:css
```

With demo mode up, load `/ports?type=tcp` at **1920, the threshold + 1, the threshold − 1, 1600, 1280, 900 and 390px**, in **both themes**:

| Width | Expected |
|---|---|
| 1920 | two columns, aside on the right, every description on one line |
| threshold + 1 | two columns, descriptions still not clipped |
| threshold − 1 | one column, aside below the table |
| 1570 and below | unchanged from today |
| 390 | unchanged from today; card mode |

Then run the overflow check, which is the thing that caught the 10px card-mode overflow in 2.15:

```bash
npm run check:ui 2>&1 | tail -6
```

- [ ] **Step 4: Commit (source only)**

```bash
git add web/src/app.css
git commit -m "$(cat <<'MSG'
fix(app-css): /ports gets its aside back above a measured width

The 2.15 decision collapsed it at every width, on the reasoning that the aside
is "worth having, but not worth this table's row width". That was width-blind.
Measured at 1920px: the five fixed columns need 796px and .col-flex absorbs
830px of slack; a 320px rail leaves the description 510px against a longest
real value of "PostgreSQL — replication peer". Tight at 1570, not tight at
1920.

The threshold was found by rendering at 1920/1800/1700/1650/1600 with the
longest real description in a row — the same way 1570 was found, and for the
same reason: the box-model estimate said ~1500 and was wrong.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

---

### Task 24: A retired hue may be named in a comment

Closes *`TestNoRetiredHueSurvives` cannot be satisfied by a comment*, under ruling #10.

`internal/web/accent_guard_test.go:84-99` flattens the whole file (`strings.ReplaceAll(string(raw), " ", "")`) and does a plain `strings.Contains` for three literals. Its sibling `TestNoAccentTokenSurvives` at `:43-68` walks lines and skips comments, with the reason written down: *"A comment may name the token it replaced — that is how the reasoning survives. A declaration may not."* The same reason applies to a literal, and the second guard does not honour it — so the two comments that hit it were reworded to describe the hue instead of spelling it, which loses the grep-ability that made the note useful.

The two reworded comments are `web/src/docs.css:537` (*"The edge was a hard-coded Aurora cyan, retired two palettes ago…"*) and `:1037` (*"Was a hard-coded Aurora cyan, retired with that palette…"*).

**Files:**
- Modify: `internal/web/accent_guard_test.go:77-99`
- Modify: `web/src/docs.css:537`, `:1037` — restore the literal
- Read for context: `:43-68` (the sibling's comment-skipping, which this copies)

**Interfaces:**
- Produces: nothing

- [ ] **Step 1: Make the guard skip comments, the same way its sibling does**

```go
func TestNoRetiredHueSurvives(t *testing.T) {
	retired := []struct{ literal, was string }{
		{"34,211,238", "the retired Aurora cyan"},
		{"8,145,178", "its light-mode twin"},
		{"45,212,191", "the teal in the hero glows"},
	}

	for _, path := range stylesheetPaths(t) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		name := filepath.Base(filepath.Dir(path)) + "/" + filepath.Base(path)

		for i, line := range strings.Split(string(raw), "\n") {
			// A comment may name the hue it replaced — that is how the
			// reasoning survives, and it is the rule TestNoAccentTokenSurvives
			// already states one function up. This test did not honour it, so
			// the two comments that recorded *which* cyan went were reworded to
			// describe it instead of spelling it, and stopped being greppable.
			// A declaration still may not.
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") ||
				strings.HasPrefix(trimmed, "//") {
				continue
			}
			flat := strings.ReplaceAll(line, " ", "")
			for _, hue := range retired {
				if strings.Contains(flat, hue.literal) {
					t.Errorf("%s:%d still carries rgba(%s,…) — %s\n"+
						"  a grep over token names cannot see a literal; this is the half "+
						"that keeps the palette honest", name, i+1, hue.literal, hue.was)
				}
			}
		}
	}
}
```

Note two changes beyond the skip: the flattening is now per line (so a reported line number is meaningful), and the error message carries that line number, which the whole-file version could not.

**A single-line `/* … */` comment is caught by the `/*` prefix. A trailing comment on a declaration line is not** — and must not be: a literal in a declaration with a comment after it is still a declaration. Say that in the comment if it is not obvious from the code.

- [ ] **Step 2: Restore the two comments**

`web/src/docs.css:537` and `:1037`. Put the literal back into the prose, so the next person grepping for `34,211,238` finds the note explaining where it went:

```css
  /* The edge was a hard-coded rgba(34,211,238,…) — the Aurora cyan, retired
     two palettes ago — and is now var(--border). */
```

Read both comments first and keep their existing sentence; only the hue's name becomes its literal again.

- [ ] **Step 3: Prove both halves**

```bash
go test ./internal/web/ -run 'TestNoRetiredHueSurvives|TestNoAccentTokenSurvives' -v
# Expected: PASS — the comments now name the literals and the guard tolerates that

# A declaration must still fail
sed -i 's|^  .callout-info {|  .callout-info { border-color: rgba(34,211,238,0.25); /* MUTATION */|' web/src/docs.css
go test ./internal/web/ -run TestNoRetiredHueSurvives 2>&1 | tail -4
# Expected: FAIL, with the line number
git checkout web/src/docs.css
```

Re-apply Step 2 after the checkout, or stash instead.

- [ ] **Step 4: Commit**

```bash
git add internal/web/accent_guard_test.go web/src/docs.css
git commit -m "$(cat <<'MSG'
test(web): a retired hue may be named in a comment, not in a declaration

TestNoAccentTokenSurvives skips comment lines and says why: a comment may name
the token it replaced, because that is how the reasoning survives.
TestNoRetiredHueSurvives did a whole-file Contains and did not, so the two
comments recording which cyan went were reworded to describe it instead of
spelling it — losing exactly the grep-ability that made them useful.

It now walks lines, skips comments, and reports a line number, which the
whole-file version could not. Both comments have their literals back.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

---

### Task 25: Prose stops leaking into the stylesheets — and both are rebuilt

Closes two carried entries with one cause: *Prose in a template leaks into the stylesheet* and *Prose in `docs/` leaks into the published stylesheet, not just prose in templates*.

Tailwind scans templates and Markdown for class-like tokens and does not understand HTML comments, so an ordinary English word inside one is compiled into a utility rule. **All three leaks are live right now, measured:**

| Rule | In | Caused by |
|---|---|---|
| `.absolute` | `web/static/style.css` **and** `docs/assets/css/style.css` | `web/templates/password.html:331` — *"It has to be absolute."* |
| `.rounded` | both | `web/templates/ports.html:38` — *"the outer rounded rectangle"* |
| `.ring` | `docs/assets/css/style.css` | `CHANGELOG.md:406` → `docs/_docs/changelog.md:421` — *"a ring with no border change"* |

The stylesheets are still correct and CI reproduces them exactly; what they are not is minimal, and the cause is invisible in the diff. Worse, a comment written in one pull request turns *Generated assets are current* red in the next.

Tailwind 4.3.3 is installed and supports `@source not`. The scan is `@source "../templates/**/*.html"` (`app.css:25`) and `@source "../../docs/**/*.{html,md}"` (`docs.css:20`).

**This task runs last in Phase 4 and owns the rebuild of both stylesheets.** Every earlier Phase 4 task reverted its built file for exactly this reason.

**Files:**
- Modify: `web/src/app.css:25-26`, `web/src/docs.css:20`
- Commit: `web/static/style.css`, `docs/assets/css/style.css` — the rebuild for all of Phase 4
- Modify: `docs-tech/invariants.md` or `CONTRIBUTING.md` — whichever an author meets first, if the exclusion cannot cover it

**Interfaces:**
- Produces: nothing

- [ ] **Step 1: Try the exclusion, and measure whether it works**

`docs.css` first, because the changelog page is generated and nobody can be asked to watch their wording in it:

```css
@source "../../docs/**/*.{html,md}";
/* Not the changelog: it is generated from CHANGELOG.md, its prose may not be
   edited to suit a scanner, and Tailwind reads class-like tokens out of
   ordinary English. Measured — "a ring with no border change" in a changelog
   entry compiled `.ring` and 1,650 bytes into the published stylesheet. */
@source not "../../docs/_docs/changelog.md";
```

```bash
npm run build:docs-css
grep -c '\.ring[{,: ]' docs/assets/css/style.css   # expected: 0
git diff --stat -- docs/assets/css/style.css
```

If the count is still 1, `@source not` is not doing what this step assumes — record what happened and go to Step 3.

- [ ] **Step 2: Try it for the template comments**

Template comments cannot be excluded by file — the file is the template. Two things to measure, in this order:

```bash
# a) Does Tailwind ignore an HTML comment if the word is not a bare token?
#    Try wrapping the words in the two comments: "absolute" -> "absolute-position"
#    is NOT the fix (it changes the prose). Instead measure whether Tailwind
#    skips <!-- --> content at all in 4.3.3:
printf '<!-- absolute rounded ring -->\n<div class="p-4"></div>\n' > /tmp/probe.html
npx tailwindcss -i web/src/app.css -o /tmp/probe.css --content /tmp/probe.html 2>/dev/null
grep -cE '\.(absolute|rounded|ring)[{,: ]' /tmp/probe.css
```

Read `npx tailwindcss --help` for the flag this version uses to point at a single content file; `--content` may not exist in v4, in which case write a throwaway `probe.css` with its own `@source "/tmp/probe.html"` instead.

The answer decides the shape of the fix. **Record it either way** — this is the measurement both carried entries were missing.

- [ ] **Step 3: If it cannot be excluded, write the rule where an author meets it**

If Tailwind scans comments regardless, the entries close as a **documented constraint** rather than a silent surprise. Add to `CONTRIBUTING.md`, beside whatever it already says about generated assets:

> **A word in an HTML comment can become a CSS rule.** Tailwind scans templates
> for class-like tokens and does not understand `<!-- -->`, so writing *"it has
> to be absolute"* in `password.html` compiles `.absolute` into
> `web/static/style.css`. The bytes are harmless; the diff is not — *Generated
> assets are current* then fails on the **next** pull request, which did not
> write the comment. Rebuild and commit both stylesheets whenever you change a
> template comment, or keep utility-shaped words out of comments.

And add the pair to `docs-tech/invariants.md`'s list, since *Generated assets are current* is the check that goes red.

- [ ] **Step 4: Rebuild both stylesheets — the whole of Phase 4 lands here**

```bash
npm run build:css
npm run build:docs-css
git diff --stat -- web/static/style.css docs/assets/css/style.css
```

Then **grep the built files for everything Phase 4 changed** — a green build is not proof the rule shipped:

```bash
# Task 20's tokens
grep -c 'callout-info-wash' docs/assets/css/style.css
# Task 21's wrap
grep -o 'overflow-wrap:break-word' docs/assets/css/style.css | head -2
# Task 22's key colour
grep -o '\.nl{[^}]*}' docs/assets/css/style.css | head -2
# Task 23's threshold
grep -o 'page-grid-ports' web/static/style.css | head -2
# and the leaks
for c in ring absolute rounded; do printf '%-10s ' ".$c"; grep -c "\.$c[{,: ]" docs/assets/css/style.css; done
```

Every one must be present (or absent, for the leaks) as the tasks claim. A missing rule here is Tailwind having dropped it silently, which is the failure mode `CLAUDE.md` names.

- [ ] **Step 5: Run the check CI runs**

```bash
npm run build:css && npm run build:docs-css
git diff --exit-code --stat -- web/static/style.css docs/assets/css/style.css && echo "reproducible"
npm run check:diagrams 2>&1 | tail -3
go test ./internal/web/ -run 'TestNoAccentTokenSurvives|TestNoRetiredHueSurvives' 2>&1 | tail -3
```

The first must report *reproducible* — a second build of the same sources must produce no diff, or the committed file is not what the sources say.

- [ ] **Step 6: Commit**

```bash
git add web/src/app.css web/src/docs.css web/static/style.css docs/assets/css/style.css CONTRIBUTING.md docs-tech/invariants.md
git commit -m "$(cat <<'MSG'
build(css): prose stops leaking into the stylesheets, and Phase 4 is rebuilt

Tailwind scans templates and Markdown for class-like tokens and does not
understand HTML comments. Measured, all three live before this commit:
.absolute in both stylesheets from password.html's "it has to be absolute",
.rounded from ports.html's "rounded rectangle", and .ring plus 1,650 bytes in
the published stylesheet from a changelog entry reading "a ring with no border
change".

The generated changelog page is excluded at the source, because its prose comes
from CHANGELOG.md and may not be edited to suit a scanner. Template comments
<RECORD WHAT STEP 2 MEASURED>.

This commit also carries the rebuild for all of Phase 4 — the callout tokens,
the inline-code wrap, the rouge key colours and /ports' aside threshold — so
one diff owns the generated files rather than five tasks fighting over them.
Both were grepped rather than assumed.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

Replace `<RECORD WHAT STEP 2 MEASURED>` with the actual finding before committing.

---

### Task 26: The mark has one home

Closes *The mark has two homes*.

`web/static/icon.svg` and `docs/assets/img/icon.svg` are **byte-identical** (verified with `cmp` while planning), and `DESIGN.md` calls the first *"the single source of geometry"* — which the second quietly contradicts. Nothing has drifted yet.

A guard test closes the entry without choosing a build step: it makes the drift impossible rather than merely absent, and it is three lines against a copy step in `package.json` that somebody has to remember to run.

**Files:**
- Create or modify: a test in `internal/shared` (beside the other repository-shape guards)
- Modify: `DESIGN.md` — say that the copy exists and what keeps it honest
- Read for context: `DESIGN.md`'s *single source of geometry* sentence (`grep -n 'single source of geometry' DESIGN.md`)

**Interfaces:**
- Consumes: `repoFile` from `internal/shared`'s test helpers, or `os.ReadFile` with the repository root helper that package already uses

- [ ] **Step 1: Write the guard**

```go
// TestTheMarkHasOneGeometry keeps the two copies of the mark identical.
//
// DESIGN.md calls web/static/icon.svg "the single source of geometry", and
// docs/assets/img/icon.svg is a byte-identical copy of it — which quietly
// contradicts that. Jekyll serves the site from docs/ and the application
// serves its own assets from web/static/, so one file cannot be in both
// places without a build step; a build step is a thing somebody has to
// remember to run, and this is a thing that fails when they do not.
//
// Nothing had drifted when this was written. That is the argument for the
// test, not against it: a mark that differs by two pixels between the
// application and its documentation is exactly the defect nobody notices.
func TestTheMarkHasOneGeometry(t *testing.T) {
	app := repoFile(t, "web", "static", "icon.svg")
	site := repoFile(t, "docs", "assets", "img", "icon.svg")
	if app != site {
		t.Errorf("web/static/icon.svg and docs/assets/img/icon.svg differ.\n"+
			"  DESIGN.md calls the first the single source of geometry, so the second is "+
			"a copy of it — update both, or the application and its documentation show "+
			"two different marks.\n  app: %d bytes, site: %d bytes", len(app), len(site))
	}
}
```

Check `repoFile`'s real signature first — `internal/shared/docs_coverage_test.go:146` calls it with path segments.

- [ ] **Step 2: Prove it fires**

```bash
go test ./internal/shared/ -run TestTheMarkHasOneGeometry -v
# Expected: PASS

printf '<!-- MUTATION -->\n' >> docs/assets/img/icon.svg
go test ./internal/shared/ -run TestTheMarkHasOneGeometry 2>&1 | tail -4
# Expected: FAIL, with both byte counts
git checkout docs/assets/img/icon.svg
```

- [ ] **Step 3: Fix `DESIGN.md`'s sentence**

It currently claims a single source while a copy exists. Say what is true:

> `web/static/icon.svg` is the source of the mark's geometry.
> `docs/assets/img/icon.svg` is a byte-identical copy, because Jekyll serves the
> site out of `docs/` and the application serves its own assets out of
> `web/static/`. `TestTheMarkHasOneGeometry` keeps the two identical, so the
> copy cannot drift into a second mark.

- [ ] **Step 4: Commit**

```bash
go test ./internal/... 2>&1 | tail -3
git add internal/shared/ DESIGN.md
git commit -m "$(cat <<'MSG'
test(shared): the two copies of the mark cannot drift apart

DESIGN.md called web/static/icon.svg the single source of geometry while
docs/assets/img/icon.svg sat beside it as a byte-identical copy, which quietly
contradicted that. Jekyll serves the site from docs/ and the application serves
its assets from web/static/, so the copy has to exist — what did not exist was
anything keeping them the same.

A test rather than a build step: a build step is something somebody has to
remember to run. Nothing had drifted, which is the argument for the guard, not
against it. DESIGN.md now says what is actually true.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

---

# Phase 5 · The four documents

T27–T31 are independent and may run at the same time. None of them touches code.

---

### Task 27: `DESIGN.md` matches the code it describes

Closes *`DESIGN.md`'s `module-active` is modelled as a border, and the code paints a shadow* (ruling #5) and *`DESIGN.md` counts the protection toggles twice, differently* (ruling #6).

**The count has a measured answer, and it is not the one either paragraph assumes.** `web/templates/options.html` renders:

```
14  class="module"          — the cards, one toggle each
14  class="module-head"
14  class="module-name"
14  class="module-desc"
11  class="module-params"   — the cards that carry parameters
```

So **eleven is the number of modules with parameters**, used twice as the number of toggles:

| Line | Says | Verdict |
|---|---|---|
| `DESIGN.md:744` | *"the options page carries eleven protection-module toggles"* | **wrong — there are 14** |
| `DESIGN.md:1272` | *"the largest cluster of controls in the product — eleven toggles on one page"* | **wrong — there are 14** |
| `DESIGN.md:1154` | *"fourteen of them took 1700px of scroll"* | **right** |
| `DESIGN.md:1191` | *"the code coloured eleven"* | unrelated — audit actions, not toggles. **Do not touch** |

`module-active` is `DESIGN.md:535-540` and carries `borderColor: "{colors.select-edge}"`. `web/src/app.css` marks an enabled module with `box-shadow: inset 2px 0 0` — the same device as the active nav item. Ruling #5: **the code is right and the spec is amended**, because consistency with a shipped pattern beats a token entry nobody implemented.

**Files:**
- Modify: `DESIGN.md:535-540` (the token entry), `:744`, `:1272` (the two counts)
- Read for context: `web/src/app.css`'s enabled-module rule (`grep -n 'inset 2px 0 0' web/src/app.css`), and the active nav item rule beside it

**Interfaces:**
- Produces: nothing

- [ ] **Step 1: Confirm both facts yourself**

```bash
grep -o 'class="module[a-z-]*"' web/templates/options.html | sort | uniq -c
grep -n 'inset 2px 0 0' web/src/app.css
sed -n '535,541p' DESIGN.md
```

If the card count is not 14/11, use what you measure and say so — the numbers above were taken on 2026-09-11 against this tree.

- [ ] **Step 2: Amend `module-active` openly**

`DESIGN.md` has been wrong three times and the house rule is to amend it in the open rather than deviate quietly. Replace the token entry:

```yaml
  module-active:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    # Amended 2026-09-11: this read borderColor: "{colors.select-edge}" and the
    # code has never painted a border here. app.css marks an enabled module with
    # box-shadow: inset 2px 0 0 — the same device as the active nav item, which
    # is why an operator reads the two as the same kind of "this one is on".
    # 2.16 only swapped the token in this entry and left the property, so the
    # mismatch predates it. The code is right: consistency with a shipped
    # pattern beats a token entry nobody implemented.
    edgeShadow: "inset 2px 0 0 {colors.select-edge}"
    typography: "{typography.body}"
    rounded: "{rounded.xl}"
```

Use whatever key name `DESIGN.md` already uses for an inset shadow elsewhere — search for another entry that models one (`grep -n 'shadow' DESIGN.md | head`). If none exists, `edgeShadow` is the new key and the comment above introduces it; do not invent a second name for something the file already names.

- [ ] **Step 3: Fix the two counts, and define the term once**

At `:744`:

> …This matters more here than in most products: the options page carries
> **fourteen** protection-module toggles, and an operator has to be able to see
> at a glance which protections are *off*.

At `:1272`:

> The protection modules are the largest cluster of controls in the product —
> **fourteen** toggles on one page, eleven of which carry their own parameters.

Then add one sentence where the modules are first described (`:1152`), so the next reader cannot make the same substitution:

> **Fourteen cards, eleven with parameters.** Both numbers appear in this file
> and they count different things; `options.html` renders fourteen
> `class="module"` cards and eleven `class="module-params"` blocks. *Eleven
> toggles* was wrong in two places until 2026-09-11.

- [ ] **Step 4: Render the options page against the amended entry**

The spec now describes an inset shadow. Load `/options` at **1600 / 900 / 390 px in both themes** with some modules on and some off, and confirm the enabled marker really is an inset shadow on the leading edge and really does match the active nav item. If it does not, **the amendment is wrong** — report what you see rather than writing the spec to match a guess.

Count the cards on screen while you are there. Fourteen.

- [ ] **Step 5: Commit**

```bash
codespell
git add DESIGN.md
git commit -m "$(cat <<'MSG'
docs(design): module-active is a shadow, and there are fourteen toggles

Two carried entries. module-active carried borderColor and app.css has never
painted a border there — it marks an enabled module with box-shadow: inset 2px
0 0, the same device as the active nav item, which is why an operator reads the
two as the same kind of "on". 2.16 swapped the token in that entry and left the
property, so the mismatch predates it. Amended in the open, as this file's own
rule asks.

And the count: options.html renders 14 class="module" cards and 11
class="module-params" blocks. Eleven is the number of modules with parameters,
and it was used twice as the number of toggles — at :744 and :1272, while :1154
had fourteen right all along. Both corrected, and the distinction is now stated
once where the modules are described.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

---

### Task 28: `invariants.md`'s folded table and merged row

Closes *Six rows lose their incident text to the renderer* (ruling #1) and *`TestRulesIsEmptyCountsEveryField` is listed twice* (ruling #2). Both are older than the 2.18 branch and both are proven at `fbac69f`.

**The table.** `docs-tech/invariants.md:106` declares `| Test | Protects |` — two columns. Rows `:108-123` have two cells; rows **`:124-129` have three**. Markdown drops the third, and the dropped column is the *incident* — the reason the file exists — so six entries read as a bare assertion in the rendered view while the source still holds the story.

Ruling #1: **fold.** The six incident texts move into the *Protects* cell. Three columns would mean inventing sixteen incident stories for the rows that have none, which is the opposite of what this file is for.

**The duplicate.** `:142` is in *The rules say what they mean* and reasons *"a seventh rule set added and forgotten makes…"*. `:166` is in *Colour means state, and it stays that way* and reasons *"it decides whether a host counts as configured"*. **Not a copy** — two different claims about one guard. Ruling #2: keep `:142`, merge `:166`'s reasoning into it, delete `:166`.

**Files:**
- Modify: `docs-tech/invariants.md:106-129`, `:142`, `:166`
- Also modify: whatever rows Tasks 9, 13, 14, 19, 25, 29 and 30 add or change — **those tasks own their own rows.** This task owns only the two structural defects

**Interfaces:**
- Produces: nothing

- [ ] **Step 1: See the damage rendered, not in the source**

```bash
sed -n '106,130p' docs-tech/invariants.md | awk -F'|' '{print NR+105": cells="NF-2}'
```

Expected: `cells=2` for `:106-123`, `cells=3` for `:124-129`. Then look at it **rendered** — GitHub's preview, or any Markdown renderer — and confirm the third cell really is dropped rather than wrapped. The entry says whoever picks this up *"should read the source, not the render, or they will think the text is missing"*; this step is the inverse and both are needed.

- [ ] **Step 2: Fold the six**

For each of `:124-129` — `TestTheSearchOverridesAreOutsideTheCascadeLayer` through `TestScreenshotsGrowTheWindowInsteadOfCapturingBeyondIt` — move the third cell's text into the second, after the *Protects* sentence, separated so the two stay distinguishable. The rows are long; keep them on one line each, as the file does elsewhere.

Shape:

```
| `TestTheSearchOverridesAreOutsideTheCascadeLayer` | the search panel's overrides of Pagefind's class names sit outside `@layer components` in the built stylesheet. **Without it:** an unlayered declaration beats every declaration in a named cascade layer, whatever its specificity — Pagefind's stylesheet is fetched at runtime and is unlayered, so an `#id` rule written inside the layer lost to its plain class selectors: the overlay shipped a yellow `<mark>` and a white input on a dark panel while the build stayed green, the rules were present in the built file, and the grep for them passed |
```

Nothing is cut. `**Without it:**` matches the sense of the third column's header in the file's other tables (*What happened without it*, *What it would have shipped*, *The incident*) without pretending this table has one.

- [ ] **Step 3: Verify no row is left with three cells**

```bash
awk -F'|' 'NF-2 > 2 && /^\| `Test/ {print FILENAME":"NR": "NF-2" cells"}' docs-tech/invariants.md
```

Run it over the **whole file**, not just this table: if another two-column table has the same defect, it is in scope here and the carried entry only found one.

```bash
python3 - <<'PY'
import re
rows=open('docs-tech/invariants.md').read().split('\n')
hdr=None
for i,l in enumerate(rows,1):
    if l.startswith('| Test |'):
        hdr=(i, l.count('|')-1)
    elif l.startswith('|') and not l.startswith('|--') and hdr:
        n=l.count('|')-1
        if n != hdr[1]:
            print(f'line {i}: {n} cells under a {hdr[1]}-column header from line {hdr[0]}')
PY
```

Fix everything this reports.

- [ ] **Step 4: Merge the duplicate**

Read both rows in full, then make `:142`'s third cell carry both claims:

```
| `TestRulesIsEmptyCountsEveryField` | `shared.Rules.IsEmpty` counts every field of the struct, by reflection | a seventh rule set added and forgotten makes … — and it is also what decides whether a host counts as configured at all, which is why the colour section used to list it too. One row, both reasons: the guard is the same guard |
```

Then delete `:166`. Keep `:142`'s existing sentence verbatim and append; do not paraphrase either claim into a shorter one that loses which release found it.

- [ ] **Step 5: Confirm nothing links to the deleted row**

```bash
grep -rn 'TestRulesIsEmptyCountsEveryField' --include='*.md' --include='*.go' . | grep -v node_modules
```

- [ ] **Step 6: Commit**

```bash
codespell
git add docs-tech/invariants.md
git commit -m "$(cat <<'MSG'
docs(invariants): six rows get their incident back, and one guard is listed once

The table at :106 declares two columns and its last six rows carried three, so
Markdown dropped the third — which is the incident, the reason this file
exists. Six entries read as bare assertions in the render while the source
still held the story. Folded into the Protects cell rather than made a third
column: three columns would have meant inventing sixteen incident texts for the
rows that have none.

TestRulesIsEmptyCountsEveryField was listed twice and was never a copy — :142
reasoned "a seventh rule set added and forgotten", :166 "it decides whether a
host counts as configured". Two claims about one guard, now one row carrying
both.

Both proven at fbac69f, older than the 2.18 branch.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

---

### Task 29: Six entries move to where a reader meets them

Closes the six Bucket D entries. None of them has a diff that fixes it; each closes by **moving** — into the guard's own comment, into `invariants.md`, or both — with the reason it stays open. `carried-forward.md` is a staging document whose own header says an entry carries enough context to **act on**; an entry nobody can act on belongs beside the thing it limits. Six have sat in a to-do file since 2.7 while being, in fact, decided.

| Entry | Goes to | The reason, stated where it lands |
|---|---|---|
| `TestIntegration_TheWindowIsOpenWhileTheRulesAreLive` is load-sensitive | the test's own comment in `internal/core/firewall_integration_test.go`, **and** one row in `invariants.md` | It polls `Status()` with `runtime.Gosched()` and no sleep, watching a gap its own comment calls *"a handful of instructions"* wide. Under container CPU contention the scheduler widens it and the test truly observes a real but unavoidable transient state. **Seen once in roughly 15 runs.** Byte-identical to `b723422` |
| The `auditBuildFindings` call site is covered by no test | the function's comment in `internal/core/firewall.go`, **and** `invariants.md` | `Firewall.nft` is a concrete `*NftablesManager`, so `Apply` cannot reach the line without a kernel. Covering it needs a finding to exist during a real apply, which needs a deliberately broken builder — a mutation, not a test. The function itself is fully covered |
| The kernel-write guard cannot see reachability | **already** in `daemon_source_order_test.go`'s comment — verify and add the `invariants.md` row | The guard passes when the panic check is kept textually and wrapped in `if false`. Not closable without `go/parser` and constant folding |
| …nor call order beyond *after the write* | same comment, same row | Moving `apply`'s check to after `f.rollback` does not fire it, though a comment says the order matters |
| The nft mutex is pinned only under `integration` | `nftables_mutex_test.go` is already self-documented — add the half it omits | `make test` cannot notice `mu sync.Mutex` being deleted. **CI's `test-integration` job does run it**, which is the half the entry does not say and the half that decides whether this matters |
| `.opencode/opencode.json` is in the history | `invariants.md`, beside the `.gitignore` entry that records the accident | Ruling #12: a public repository, 175 merged pull requests, a file with nothing sensitive in it, and a history rewrite that costs every clone and fork. **Decided, not deferred** |

**Files:**
- Modify: `internal/core/firewall_integration_test.go`, `internal/core/firewall.go`, `internal/core/daemon_source_order_test.go`, `internal/core/nftables_mutex_test.go`
- Modify: `docs-tech/invariants.md` — one row per entry, in the section each belongs to
- Read first: each target file's existing comment. **Three of the six already say most of it** — this task adds the missing half, not a second copy

**Interfaces:**
- Produces: nothing

- [ ] **Step 1: Read what each target already says**

```bash
grep -n -B2 -A12 'TestIntegration_TheWindowIsOpenWhileTheRulesAreLive' internal/core/firewall_integration_test.go | head -24
grep -n -B4 -A10 'auditBuildFindings' internal/core/firewall.go | head -24
grep -n 'if false\|reachability\|go/parser' internal/core/daemon_source_order_test.go
cat internal/core/nftables_mutex_test.go
```

Three of these already carry the reasoning. Adding a paragraph that repeats it is the failure mode here — the aim is that each limit is stated **once**, where a reader meets it.

- [ ] **Step 2: Add only what is missing, to each file**

For each of the four code files, add the sentence the carried entry has and the comment does not. Concretely, at minimum:

- `firewall_integration_test.go` — the **frequency**: *seen once in roughly 15 runs*, and that it is a real transient state rather than a flake to retry away
- `firewall.go` — that covering the `auditBuildFindings` call site needs a mutation and not a test, and that the function itself is fully covered
- `daemon_source_order_test.go` — verify the `if false` and the call-order limits are both named; add whichever is not
- `nftables_mutex_test.go` — that **CI's `test-integration` job runs it**, so `make test`'s blindness is bounded rather than total

- [ ] **Step 3: Add six rows to `invariants.md`**

Each row goes in the section it belongs to, and each states the limit rather than the guarantee — that is what makes them different from every other row in the file. Put them in the table shape that section uses (check the header; after Task 28 the `:106` table is two columns and the others are three).

For `.opencode/opencode.json` there is no test, so it belongs wherever `invariants.md` records repository-shape facts rather than guards. If there is no such section, put it beside the `.gitignore` reasoning and say plainly that it is a decision and not a guard.

- [ ] **Step 4: Commit**

```bash
go test ./internal/... 2>&1 | tail -3
codespell
git add internal/core/ docs-tech/invariants.md
git commit -m "$(cat <<'MSG'
docs: six limits move from the to-do file to where a reader meets them

None of the six has a diff that fixes it: a load-sensitive integration test, a
call site that needs a kernel and a broken builder, a guard that cannot see
`if false` without go/parser, the same guard's blindness to call order, a mutex
pinned only under the integration tag, and a file in two commits of a public
repository's history.

carried-forward.md's own header says an entry carries enough context to act on.
These could not be acted on and had sat there since 2.7 while being, in fact,
decided — so each is now stated in the guard's own comment or in invariants.md,
with the reason it stays. Three of the four code comments already said most of
it; this adds the half they omitted, including the frequency (once in roughly
15 runs) and the fact that CI's test-integration job does run the mutex pin.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

---

### Task 30: The recording-adder guard's claim is narrowed

Closes *`TestEveryRuleIsAddedThroughTheRecordingAdder` matches a selector, so a local copy of the connection evades it* — approved in §5 as a **declined** contract change, closing D-style.

`internal/core/nftables_check_test.go:420-440` walks every non-test source in `internal/core` with `go/parser` and refuses a `.conn.AddRule` selector outside `builtRecorder.AddRule`. **Measured: `cn := m.conn` followed by `cn.AddRule(…)` passes it** — there is no `.conn.AddRule` selector left to match, and refusing that needs type resolution rather than syntax, which is a different kind of guard.

It is **not** the shape the finding measured: a copy-paste from pre-`c4dab40` history writes `m.conn.AddRule` and is caught, in any file of the package. `docs-tech/invariants.md` already states the scope as the selector rather than as the intent, so nothing claims more than this. The decision is to leave the guard and put the limit in its own comment — the one place it is currently missing.

**Files:**
- Modify: `internal/core/nftables_check_test.go:415-440` — the guard's doc comment
- Verify: `docs-tech/invariants.md`'s row for it already states the selector scope; if it states the intent instead, fix it here

**Interfaces:**
- Produces: nothing

- [ ] **Step 1: Reproduce the evasion, so the comment states a measurement**

```bash
cp internal/core/nftables.go /tmp/nftables.go.bak
# Find a function that calls m.conn.AddRule and rewrite one call through a local
grep -n 'conn.AddRule' internal/core/nftables.go | head -3
```

Take one call site and change `m.conn.AddRule(r)` to:

```go
	cn := m.conn
	cn.AddRule(r)
```

```bash
go test ./internal/core/ -run TestEveryRuleIsAddedThroughTheRecordingAdder -v
# Expected: PASS — which is the finding
cp /tmp/nftables.go.bak internal/core/nftables.go
```

Record the result. If it now **fails**, the guard has been strengthened since the entry was written and this task closes by deleting the entry instead — say so.

- [ ] **Step 2: Put the limit in the guard's comment**

Extend the doc comment above `TestEveryRuleIsAddedThroughTheRecordingAdder`:

```go
// What this does NOT catch, measured rather than assumed: a local copy of the
// connection. `cn := m.conn` followed by `cn.AddRule(…)` passes, because there
// is no `.conn.AddRule` selector left to match. Refusing that needs type
// resolution rather than syntax — go/types and a full package load, which is a
// different kind of guard and a much slower test.
//
// Left alone deliberately, and not for lack of time. The shape this exists to
// catch is a copy-paste from pre-c4dab40 history, which writes `m.conn.AddRule`
// and is caught in any file of the package. A local alias is not what anybody
// writes by accident; it is what somebody writes to get around a guard, and a
// guard is not a defence against its own author. invariants.md states this
// scope as the selector rather than as the intent, so nothing in the repository
// claims more than the syntax actually delivers.
```

- [ ] **Step 3: Check `invariants.md` agrees**

```bash
grep -n -A3 'TestEveryRuleIsAddedThroughTheRecordingAdder' docs-tech/invariants.md
```

If its row promises that *every* rule goes through the recorder, narrow the wording to the selector. A row that claims more than the test delivers is the same defect one layer up.

- [ ] **Step 4: Commit**

```bash
go test ./internal/core/ 2>&1 | tail -3
git add internal/core/nftables_check_test.go docs-tech/invariants.md
git commit -m "$(cat <<'MSG'
docs(core): the recording-adder guard says what its syntax cannot see

Measured: `cn := m.conn` followed by `cn.AddRule(…)` passes it, because no
.conn.AddRule selector is left to match. Refusing that needs go/types and a
full package load — a different kind of guard and a much slower test.

Declined deliberately. The shape the guard exists to catch is a copy-paste from
pre-c4dab40 history, which writes m.conn.AddRule and is caught in any file of
the package; a local alias is not what somebody writes by accident. The limit
now lives in the guard's own comment, which is the one place it was missing.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

---

### Task 31: `CLAUDE.md`'s generated-file rule stops over-claiming

Closes *`npm run build:diagrams` is not byte-reproducible*.

The carried entry is correct and the repository's own rule is not. `CLAUDE.md:50` reads:

> **A generated file is rebuilt and diffed, never assumed** — Tailwind drops a rule silently and the build stays green — grep the built file

That holds for `web/static/style.css`, `docs/assets/css/style.css` and the changelog page. It does **not** hold for the diagrams: Mermaid jitters the bezier control points in `label-container outer-path`, so two runs over unchanged sources differ. Rebuild-and-diff is therefore not a staleness test for diagrams — `npm run check:diagrams`, which compares a `data-source-digest`, is. `docs/_docs/contributing.md` said otherwise and was corrected; `CLAUDE.md` still carries the over-broad version.

**Files:**
- Modify: `CLAUDE.md:50`
- Read for context: `scripts/check-diagrams.mjs` (or whatever `check:diagrams` runs — `grep -n 'check:diagrams' package.json`), and `docs/_docs/contributing.md`'s corrected sentence, which is the wording to be consistent with

**Interfaces:**
- Produces: nothing

- [ ] **Step 1: Confirm the non-reproducibility, so the rule is amended on evidence**

```bash
npm run build:diagrams
git diff --stat -- docs/assets/diagrams/
git checkout docs/assets/diagrams/ 2>/dev/null
npm run build:diagrams
git diff --stat -- docs/assets/diagrams/
git checkout docs/assets/diagrams/
```

Expected: a diff over unchanged sources, in `label-container outer-path`. Then confirm the check that **does** work:

```bash
npm run check:diagrams; echo "exit: $?"
```

- [ ] **Step 2: Amend the rule**

Replace `CLAUDE.md:50`:

```markdown
| **A generated file is rebuilt and diffed, never assumed** | Tailwind drops a rule silently and the build stays green — grep the built file. **The diagrams are the exception:** Mermaid jitters its bezier control points, so two runs over unchanged sources differ and a diff proves nothing. `npm run check:diagrams` compares a `data-source-digest` and is the staleness test for those |
```

Keep it in the table — the row's whole value is that it sits beside the incident that produced it.

- [ ] **Step 3: Check the three places that state this rule agree**

```bash
grep -rn 'rebuilt and diffed\|byte-reproducible\|data-source-digest' CLAUDE.md docs/_docs/contributing.md docs-tech/ CONTRIBUTING.md | grep -v node_modules
```

`docs/_docs/contributing.md` was already corrected. `CONTRIBUTING.md` and `docs-tech/ci-and-release.md` may also state it — every copy says the same thing after this task, or the next reader picks the wrong one.

- [ ] **Step 4: Commit**

```bash
codespell
git add CLAUDE.md CONTRIBUTING.md docs-tech/
git commit -m "$(cat <<'MSG'
docs: the rebuild-and-diff rule names its one exception

"A generated file is rebuilt and diffed, never assumed" holds for both
stylesheets and the changelog page. It does not hold for the diagrams: Mermaid
jitters the bezier control points in label-container outer-path, so two runs
over unchanged sources differ and the diff proves nothing.
npm run check:diagrams compares a data-source-digest and is the staleness test
for those.

docs/_docs/contributing.md was corrected when this was found. CLAUDE.md still
carried the over-broad version, which is what the carried entry said.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

---

# Phase 6 · Closing

**Strictly sequential.** T32 needs every Phase 4 stylesheet change built and committed. T33 needs every other task's outcome known. T34 is the only whole-branch verification.

---

### Task 32: The screenshots are re-taken

Not a carried entry — a consequence. Phase 4 changes visible surface on `/ports` (the aside returns above a threshold), on `/options` (nothing, but verify), on `features/security.md`, on every documentation page with a callout, and in every highlighted configuration block. The 36 shots taken on 2026-09-11 then describe an interface that no longer exists, and `docs/assets/img/screens/*` is documentation.

**The version chip stays `v2.18.0`, and that is correct** — this sweep is unversioned. Do **not** rebuild with a new `VERSION`.

**Files:**
- Modify: `docs/assets/img/screens/*.png`
- Read first: `docs-tech/local-review.md` (the demo server, the asset cache and the version chip all bite), `scripts/ui-check.mjs:1520-1545` (`--screenshots`, `DEFAULT_SCREENSHOT_PAGES`), `:1884-1895` (`takeFullScreenshotSet`)

**Interfaces:**
- Consumes: Task 15's URL derivation and Task 16's idempotent catalogue check — both make this step work against a demo server that has already been driven

- [ ] **Step 1: Build the binary the screenshots will photograph**

```bash
make build VERSION=v2.18.0
ls -la bin/easywall-web
```

`v2.18.0` deliberately: the tree is unversioned and the chip must keep reading the last release. A stale binary looks exactly like a real defect — confirm the timestamp is from this run.

- [ ] **Step 2: Start the demo server and prove it is the one being driven**

```bash
scripts/demo-server.sh &
sleep 4
npm run check:ui 2>&1 | head -3
```

Task 15's first line names the URL and the config. **Read it.** The incident that task closes was a check reporting success against a different server than the one under test; this is where that matters most, because a screenshot of the wrong server is a wrong picture in the documentation.

- [ ] **Step 3: Take the whole set**

```bash
npm run check:ui -- --screenshots 2>&1 | tail -20
```

`--screenshots` with no page names shoots the whole documented set via `takeFullScreenshotSet` — `DEFAULT_SCREENSHOT_PAGES` plus the wizard and verify screens. One command, both themes.

```bash
git status --short -- docs/assets/img/screens/ | head -40
ls docs/assets/img/screens/ | wc -l
```

- [ ] **Step 4: Check the pictures, not the file list**

**Open the changed images.** At minimum:

| Screenshot | What must be visible |
|---|---|
| `ports-*` | the aside is **present** at the shot's width if that width is above Task 23's threshold, absent if below — and no description is clipped |
| `options-*` | fourteen module cards, enabled ones marked by the inset shadow |
| `password-*` | the recovery-codes card and the *continue* link as siblings (Task 5) |
| `log-*` | both new audit labels (Task 12), and the German one not breaking its column |
| every shot | version chip reads `v2.18.0` |

```bash
grep -o 'v2\.1[0-9]\.[0-9]' docs/assets/img/screens/*.png 2>/dev/null | sort -u | head
```

That grep will not work on a PNG — it is a reminder that the chip is checked **by looking**, which is this repository's rule and the reason 34 screenshots once shipped reading a stale version.

- [ ] **Step 5: Check the documentation still references what exists**

```bash
# every figure a page references exists, and no screenshot is an orphan
python3 - <<'PY'
import os,re,glob
refs=set()
for p in glob.glob('docs/_docs/**/*.md', recursive=True):
    for m in re.finditer(r'screens/([A-Za-z0-9_-]+)-(light|dark)\.png', open(p).read()):
        refs.add(m.group(1))
have={os.path.basename(f).rsplit('-',1)[0] for f in glob.glob('docs/assets/img/screens/*.png')}
print('referenced but missing:', sorted(refs-have) or 'none')
print('present but unreferenced:', sorted(have-refs) or 'none')
PY
```

The 2.18 pass measured 18 referenced bases, all 18 present, no orphans. Any change here is a finding.

- [ ] **Step 6: Commit**

```bash
git add docs/assets/img/screens/
git commit -m "$(cat <<'MSG'
docs: re-take the screenshots Phase 4 changed

/ports gets its aside back above a measured width, an inline code chip may now
break, the rouge theme colours configuration keys, the callout washes are
tokens, and the password page's continue link is a sibling of the codes card.
The shots from earlier today describe an interface that no longer exists, and
docs/assets/img/screens/ is documentation.

Taken against bin/easywall-web built with VERSION=v2.18.0: this sweep is
unversioned, so the chip correctly still reads the last release.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

---

### Task 33: `carried-forward.md` is emptied

The task this whole plan exists for. Every open row is struck through with a date and a sentence saying how it closed, or — for Bucket D — removed with a pointer to where it now lives.

`carried-forward.md` keeps its header, its rule, the 2.17 exception and every previously struck entry: **the file is a record, not a scratch pad.** What it no longer has is an open row.

**Files:**
- Modify: `docs-tech/carried-forward.md`
- Read first: every commit on this branch (`git log --oneline main..HEAD`), because each closing sentence has to be true of what actually landed

**Interfaces:**
- Consumes: the outcome of all 32 preceding tasks

- [ ] **Step 1: List what actually happened**

```bash
git log --oneline main..HEAD
```

Read it against the 43 entries. Every entry gets exactly one of three treatments, and **which one is decided by what landed, not by what this plan predicted**:

| Treatment | For | Shape |
|---|---|---|
| **Struck through, dated, with the sentence** | A and B and the accepted C items | `~~**Title**~~ \| **Closed 2026-09-11.** <what was done, and the thing worth keeping>` |
| **Removed, with a pointer** | the six D items and the two declined C items | The row goes; the limit lives in the file named in Tasks 29 and 30 |
| **Corrected, then struck** | the three entries whose premise did not hold | Say what the measurement was, because a closed entry that closed for a wrong reason is a worse record than an open one |

- [ ] **Step 2: Correct the three premises as part of closing them**

These are the ones a future reader would otherwise inherit as facts:

```
| ~~**`.callout-info` is the last blue on the documentation site**~~ | **Closed 2026-09-11, and the premise was half wrong.** All three callouts hard-code their wash and edge — info `rgba(56,189,248,…)`, warning `rgba(245,158,11,…)`, success `rgba(16,185,129,…)` — and all three already tokenised their *text* colour. There was no info-only exception. The blue stays: *info* is a state, and *colour means state* does not forbid a state from having one. Six literals became six tokens, and the two theme-scoped overrides went away |

| ~~**`TestRulesIsEmptyCountsEveryField` is listed twice**~~ | **Closed 2026-09-11, and it was never a copy.** `:142` reasoned *"a seventh rule set added and forgotten"*; `:166` reasoned *"it decides whether a host counts as configured"* — two claims about one guard. Merged into `:142`, which now carries both; `:166` deleted |

| ~~**`security.md` overflows at 390px**~~ | **Closed 2026-09-11, and the fix was rejected for a property it does not have.** The entry ruled out `overflow-wrap` because it *"would break every long identifier at an arbitrary point"* — true of `anywhere` and `break-all`, not of `break-word`, which breaks a word only when it cannot fit a line alone. Verified rendered: the 462px test name wraps, `proxy_set_header` does not, and a `<pre>` still scrolls. `DESIGN.md` now carries the rule |
```

- [ ] **Step 3: Record what §5's one reversal actually decided**

The `GET_HEALTH` entry closes **without** the change the spec proposed, and the reason is a measurement. That belongs in the file:

```
| ~~**`GET_HEALTH` is documented as short-deadline because it is read-only, and both its netlink reads take the nft mutex**~~ | **Closed 2026-09-11 by correcting three comments rather than the deadline.** The spec proposed lengthening it; two measurements declined that. `Dockerfile:156` is `--timeout=5s`, so docker abandons the probe at five seconds whatever the core's deadline says — a 35 s deadline would change nothing for the only documented consumer and make every other caller wait. And `TestCommandTimeoutKeepsGetHealthShort` pinned the 5 s deliberately, **on a false premise**: its comment claimed the command does not queue behind the nft mutex, while `Enforcing` (`nftables.go:347`) and `RuleCounters` (`:416`) both take `m.mu`. So one of the three comments was a guard passing for the wrong reason. The deadline stands; the honest scope — 503 during a slow apply, docker's third retry inside the window, nothing restarting on unhealthy — is now in the command's comment, in the guard's comment and in `features/health.md` |
```

- [ ] **Step 4: Check the file has no open row left**

```bash
grep -c '^| \*\*' docs-tech/carried-forward.md      # expected: 2 — the rule's own Defect/Contract hole table
grep -c '^| ~~\*\*' docs-tech/carried-forward.md    # expected: 18 + everything closed here
```

Then read every `# From` section and confirm none has a live entry. If a section is now entirely struck through, leave it: the history is the file's purpose.

- [ ] **Step 5: Say at the top what changed**

Add a short paragraph under the header, above *The one exception*:

> **Emptied 2026-09-11.** Forty-three open entries were ruled on rather than
> carried again: the ones a diff closes were fixed, the ones a decision was
> blocking got the decision, and the six nothing can fix were moved to where a
> reader meets the limit — a guard's own comment, or `invariants.md` — because
> an entry nobody can act on does not belong in a file whose header says an
> entry carries enough context to act on. Three of the entries' own premises
> did not survive measurement and are corrected above rather than quietly
> closed. Two proposed contract changes were declined with a measurement, which
> is also a ruling. See
> `docs-tech/specs/2026-09-11-the-carried-forward-sweep.md`.

- [ ] **Step 6: Commit**

```bash
codespell
git add docs-tech/carried-forward.md
git commit -m "$(cat <<'MSG'
docs: carried-forward.md is empty

Forty-three open entries, ruled on rather than carried a tenth time. The ones a
diff closes are fixed; the ones a decision was blocking got the decision; the
six nothing can fix moved to the guard comments and invariants.md rows where a
reader actually meets the limit.

Three entries' own premises did not survive measurement and are corrected here
rather than quietly closed: all three callouts hard-code their wash and info was
never the exception, the two TestRulesIsEmpty rows were two claims rather than a
copy, and overflow-wrap: break-word does not have the property the security.md
entry rejected it for. Two contract changes were declined with a measurement,
which is also a ruling.

The file keeps every struck entry. It is a record, not a scratch pad — what it
no longer has is an open row.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
MSG
)"
```

---

### Task 34: The whole branch, verified once

The only task that runs everything. Nothing here is new work — it is the proof the branch is mergeable, and the place a missed gate shows up.

**Files:** none.

- [ ] **Step 1: Every Go gate**

```bash
make test
make lint
~/go/bin/golangci-lint run ./... 2>&1 | tail -10
~/go/bin/gosec -include=G115 -fmt=json ./... 2>/dev/null | python3 -c "import json,sys; print('G115 prod:', len(json.load(sys.stdin).get('Issues',[])))"
sudo go test -tags integration ./internal/core/... ./internal/web/... 2>&1 | tail -15
sudo EASYWALL_REQUIRE_SELFTEST=1 go test -tags integration ./internal/core/... 2>&1 | tail -8
```

The last line is what CI runs, and after Task 7 it covers four more tests than it did.

- [ ] **Step 2: Every Node gate**

```bash
npm run check:css 2>/dev/null || true
npm run build:css && npm run build:docs-css
git diff --exit-code --stat -- web/static/style.css docs/assets/css/style.css && echo "stylesheets reproducible"
npm run check:diagrams
npm run check:changelog
npm run check:docs
npm run check:prose
codespell; echo "codespell exit: $?"
```

`codespell` now runs repo-wide in CI (Task 14) — so this local run is the same check, which it was not before.

- [ ] **Step 3: The interface, rendered**

```bash
scripts/demo-server.sh &
sleep 4
npm run check:ui 2>&1 | tail -10
npm run check:ui 2>&1 | tail -10   # twice — Task 16 exists so the second run passes
```

Then, by hand, at **1600 / 900 / 390 px in both themes and both languages**: `/dashboard`, `/ports`, `/options`, `/password`, `/log`. And on the documentation site: the configuration reference, `features/security.md`, `features/audit-log.md`, the changelog page.

- [ ] **Step 4: Prove nothing was carried that this branch caused**

```bash
git diff --stat main..HEAD
git log --oneline main..HEAD | wc -l
grep -c '^| \*\*' docs-tech/carried-forward.md
```

The last must be **2** — the rule's own explanation table and nothing else. If any entry was added to `carried-forward.md` during this branch, it is a defect this work caused and it does not belong there: fix it, or say plainly in the pull request why the rule does not apply.

- [ ] **Step 5: Open the pull request**

```bash
git push -u origin chore/carried-forward-sweep
gh pr create --title "The carried-forward sweep: forty-three entries, ruled on" --body "$(cat <<'MSG'
`docs-tech/carried-forward.md` had 46 open rows across eight pieces of work,
the oldest from 2.7. It has none.

Forty-three distinct entries — two of the 46 rows are the rule's own
explanation table, one said "Shipped in 2.14" in its own text and was never
struck through, and one finding was recorded twice. They were ruled on rather
than carried again:

- **19 mechanical** — a diff and a mutation proof each
- **11 that needed a decision**, taken up front so the plan had no stop points
- **5 accepted contract changes**, and **2 declined with a measurement**
- **6 that nothing can fix**, moved to the guard comments and `invariants.md`
  rows where a reader meets the limit

**Three of the file's own premises did not survive measurement** and are
corrected rather than quietly closed: all three callouts hard-code their wash,
so info was never the blue exception; the two `TestRulesIsEmptyCountsEveryField`
rows were two claims about one guard rather than a copy; and
`overflow-wrap: break-word` does not have the property the `security.md` entry
rejected it for.

**One §5 proposal was reversed by measurement.** `GET_HEALTH`'s deadline was to
be lengthened; `Dockerfile`'s `HEALTHCHECK --timeout=5s` means docker abandons
the probe at five seconds regardless, and the existing guard pinning 5 s turned
out to argue from a false premise — both of its netlink reads *do* take the nft
mutex. The deadline stands and three comments stop lying.

Unversioned: no tag, no CHANGELOG version section, preparation for 2.19.

Spec: `docs-tech/specs/2026-09-11-the-carried-forward-sweep.md`
Plan: `docs-tech/plans/2026-09-11-the-carried-forward-sweep-implementation.md`

🤖 Generated with [Claude Code](https://claude.com/claude-code)
MSG
)"
```

- [ ] **Step 6: Watch CI, and merge only on green**

```bash
gh pr checks --watch
```

Thirteen checks are required on `main` and enforced for administrators. A red check here is this branch's to fix — not a carried entry.

---

## Coverage: every carried entry, and the task that closes it

| # | Entry | Task |
|---|---|---|
| 1 | The ACME serving path has no unit coverage | T1 |
| 2 | `Prompt` could return false | T1 |
| 3 | `newACMEManager`'s hostname re-check is unguarded | T1 |
| 4 | `passkeys.json`'s file mode is unpinned | T2 |
| 5 | The `v3:` domain separator is not pinned | T2 |
| 6 | `TestTheRuleIsStatedOnce` catches the identifier | T3 |
| 7 | No ceremony runs with a port in the origin | T4 |
| 8 | `JustGated` is unobservable without codes | T5 |
| 9 | Three getters have no malformed-reply test | T6 |
| 10 | Four `TestIntegration_Forward_*` skip in a container | T7 |
| 11 | `RunPeer` collapses every dial error into `blocked` | T8 |
| 12 | `GET_HEALTH`'s deadline and the nft mutex | T9 (reversed — see the task) |
| 13 | The harness range has no collision check | T10 |
| 14 | `CmdPanic`'s 35 s deadline | T11 |
| 15 | `boot_enforce_failed` reads "at startup" | T12 |
| 16 | `rollback_skipped`'s label | T12 |
| 17 | G115 excluded globally | T13 |
| 18 | The spelling gate's scope | T14 |
| 19 | `ui-check.mjs` does not derive its URL | T15 |
| 20 | `check:ui` is not re-runnable | T16 |
| 21 | `checkPortsCatalogue` is not idempotent | T16 (same finding) |
| 22 | `render-changelog.mjs` is renderer and checker | T17 |
| 23 | The changelog page has no on-page contents | T18 |
| 24 | `podman build` drops `HEALTHCHECK` | T19 |
| 25 | `.callout-info` is the last blue | T20 |
| 26 | `security.md` overflows at 390px | T21 |
| 27 | Nine rouge classes unstyled | T22 |
| 28 | `/ports` collapses its aside width-blind | T23 |
| 29 | `TestNoRetiredHueSurvives` and comments | T24 |
| 30 | Prose in a template leaks into the stylesheet | T25 |
| 31 | Prose in `docs/` leaks into the published stylesheet | T25 |
| 32 | The mark has two homes | T26 |
| 33 | `module-active` border vs shadow | T27 |
| 34 | `DESIGN.md` counts the toggles twice | T27 |
| 35 | Six rows lose their incident text | T28 |
| 36 | `TestRulesIsEmptyCountsEveryField` listed twice | T28 |
| 37 | The window test is load-sensitive | T29 |
| 38 | `auditBuildFindings`' call site is uncovered | T29 |
| 39 | The guard cannot see reachability | T29 |
| 40 | …nor call order | T29 |
| 41 | The nft mutex is pinned only under `integration` | T29 |
| 42 | `.opencode/opencode.json` is in the history | T29 |
| 43 | The recording-adder guard matches a selector | T30 |
| — | `build:diagrams` is not byte-reproducible | T31 |
| — | *Every page pays a `GET_STATUS`* (shipped in 2.14, never struck) | T33 |
