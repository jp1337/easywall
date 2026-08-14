# Landing the bug-bounty pass and cutting 2.6.0 — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn an uncommitted twelve-defect bug-bounty pass into eleven reviewable commits, verify each claim it makes against something real, and release it as 2.6.0.

**Architecture:** No code is written here that does not already exist in the working tree. The work is a *split*: a snapshot is taken first, each commit is produced by editing the working tree down to exactly its own change, and the split is proved lossless at the end by diffing the tree against the snapshot. Verification then exercises the three claims Go tests cannot make — a real kernel, a real browser, a real dpkg upgrade — and the release moves the version in the three files that carry it.

**Tech Stack:** Go 1.26 · nftables via `google/nftables` · chi · Playwright (`scripts/ui-check.mjs`) · mermaid + Chromium (`scripts/render-diagrams.mjs`) · debhelper · GoReleaser · Jekyll

**Spec:** [`docs-tech/specs/2026-08-14-landing-the-bug-bounty-pass-and-2.6.0.md`](../specs/2026-08-14-landing-the-bug-bounty-pass-and-2.6.0.md)

## Global Constraints

- **Nothing is reported as passing that was not run.** A check that cannot be run (no root, no container) is named as skipped, never assumed.
- **No new behaviour.** Every line committed here is already in the working tree. If a task seems to need a code change that is not in the snapshot, stop and say so — it means the split lost something.
- **Conventional Commits**, per `CONTRIBUTING.md`: `fix`, `feat`, `security`, `docs`, `chore`, `refactor`, `test`. Scope in parentheses.
- **Every commit carries its own `CHANGELOG.md` paragraph**, its tests and its documentation. A commit that changes behaviour without its changelog line is incomplete.
- **Every commit must build**: `go build ./... && go vet ./...` at each one, enforced once over the series with `git rebase --exec`.
- **Commit message trailers** — every commit ends with the same two-line
  `Co-Authored-By` / `Claude-Session` block the commits already on this branch
  carry. Copy it verbatim from `git log -1 --pretty=%B | tail -2`; `<TRAILERS>`
  in the commands below stands for exactly those two lines. It is not spelled
  out here because `TestNoPersonalEmailAddressesAreTracked` refuses that address
  in a tracked file — in commit metadata it is fine, in file content it is not,
  and this plan is a tracked file.
- **Never write a version number into `docs-tech/`.** Nothing updates it there.
- **The snapshot directory** is `$SNAP` throughout:
  `/tmp/claude-1000/-var-home-jpy-projects-easywall/20bb8b7d-794d-4f61-88f8-b42eb031a4e8/scratchpad/final`

## Deviation from the spec, decided while planning

The spec lists **twelve** commits. Executing it produces **eleven**, because two of
its entries are one change:

`internal/web/handler_settings_networks_test.go` opens with
`TestTheNetworkEditorRefusesExactlyWhatTheCoreRefuses`, which calls
`validateCIDRListEntries` (the page half, spec commit 6) and
`shared.ValidateNetworkList` (the file half, spec commit 7) **in the same
assertion** and fails if the two disagree. That test cannot exist in either
commit alone, and it is the point of the fix: one definition of what a network
is. Splitting it would mean writing a weaker test than the one already in the
tree.

Spec commits 6 and 7 therefore become **Task 6**, carrying both `CHANGELOG`
paragraphs. Nothing else moves.

## File map — which commit owns which file

Single-owner files (staged whole):

| Task | Files |
|---|---|
| 1 | `debian/rules`, `debian/postinst`, `internal/shared/conffiles_test.go`, `docs/installation/debian.md` |
| 2 | `internal/shared/protocol.go`, `internal/web/client.go`, `internal/web/client_timeout_test.go` |
| 3 | `internal/shared/models.go`, `internal/web/handler_options.go`, `web/templates/options.html`, `docs/schemas/easywall.schema.json`, `internal/core/config_limits_test.go`, `internal/web/firewall_limits_test.go` |
| 4 | `internal/core/nftables.go`, `internal/core/nftables_snapshot_test.go` |
| 5 | `web/templates/base.html`, `web/src/app.css`, `web/static/style.css`, `internal/web/handler_login_test.go`, `internal/web/session_lifetime_test.go`, `internal/web/handler_logout_method_test.go` |
| 6 | `internal/shared/validate.go`, `internal/web/handler_iplist_validate.go`, `internal/web/handler_settings.go`, `internal/web/democlient.go`, `web/templates/settings.html`, `docs/features/docker.md`, `docs/features/system-settings.md`, `docs/assets/img/screens/settings-{dark,light}.png`, `internal/shared/validate_networks_test.go`, `internal/core/config_networks_test.go`, `internal/web/handler_settings_networks_test.go` |
| 8 | `systemd/easywall-core.service`, `systemd/easywall-web.service` |
| 9 | `scripts/render-diagrams.mjs`, `docs/assets/diagrams/*.svg` (14 files) |
| 10 | `internal/shared/package_version_test.go`, `internal/web/diagram_palette_test.go`, `.github/workflows/docs.yml` |
| 11 | `.golangci.yml`, `internal/core/acceptance.go` |

Shared files, and who owns which part of them:

| File | Task 1 | Task 3 | Task 5 | Task 6 | Task 7 |
|---|---|---|---|---|---|
| `internal/core/config.go` | — | the `shared.FirewallLimits` loop in `Validate`, the removal of the local `firewallLimit` type and `firewallLimits()`, the new comment in its place, `SaveFirewallOptions` | — | the `checkNetworkLists` call in `Validate` + its comment, `checkCIDRList` → `checkNetworkLists`, the `SaveNetworkSettings` call site, dropping the `"net"` import | — |
| `internal/web/server.go` | — | `"options_invalid_limit"` in `clientStringKeys` and in `templateFuncs`' warning map | `r.Get("/logout"` → `r.Post("/logout"` + its comment | `"settings_invalid_network"` in both places + its comment | — |
| `web/static/app.js` | — | the `options_invalid_limit` toast line | — | the `settings_invalid_network` toast line | — |
| `locales/en.json`, `locales/de.json` | — | `options_invalid_limit` | — | `settings_invalid_network`, both `*_networks_help` texts | — |
| `docs/configuration.md` | the `*.toml.template` paragraph (§ *Where the files live*) | the limits table + the wrapping-32-bit note | — | the CIDR paragraph + the "nothing checked these two lists" note | — |
| `scripts/ui-check.mjs` | — | — | `checkSignOutEndsTheSession` and its call | — | `signIn` hoisted out of the per-context loop, the 429 message, the doc comment |
| `CHANGELOG.md` | ¶ conffile | ¶ limits | ¶ logout | ¶ networks ×2 | ¶ check:ui |

Tasks 8–11 own their `CHANGELOG` paragraphs in the same way; Task 11 additionally **writes one new sentence** for `internal/core/acceptance.go`, which has no entry today.

## The split technique

Used in every task that touches a shared file. It edits *down* from the finished
version rather than rebuilding from `HEAD`, because the finished version is the
one that is known to work.

1. The file on disk holds the finished content (from `$SNAP`).
2. Edit it so it holds **`HEAD` plus the parts owned by this task and by tasks already committed** — that is, revert the parts belonging to *later* tasks to their `HEAD` text.
3. `git add <file>` and commit.
4. At the file's *last* owning task, restore it whole: `cp "$SNAP/<file>" <file>`.

To see a part's `HEAD` text: `git show HEAD:<path>`. To see what is left to
place: `git diff <path>`.

---

### Task 0: Snapshot, and branch off a merged main

**Files:** none modified.

**Interfaces:**
- Produces: `$SNAP`, a byte-for-byte copy of every modified and untracked file, which Task 12 diffs against. Branch `fix/bug-bounty-2.6` off `main`.

- [ ] **Step 1: Snapshot the working tree**

```bash
SNAP=/tmp/claude-1000/-var-home-jpy-projects-easywall/20bb8b7d-794d-4f61-88f8-b42eb031a4e8/scratchpad/final
rm -rf "$SNAP" && mkdir -p "$SNAP"
git ls-files -m -o --exclude-standard | tar -cf - -T - | tar -xf - -C "$SNAP"
find "$SNAP" -type f | wc -l   # expect 66: 55 modified + 11 untracked
```

- [ ] **Step 2: Record the baseline the split must reproduce**

```bash
git stash list  # expect empty; if not, stop and ask
git status --porcelain > "$SNAP/../baseline-status.txt"
wc -l < "$SNAP/../baseline-status.txt"   # expect 66
```

- [ ] **Step 3: Merge PR #122 and take main**

```bash
gh pr checks 122
gh pr merge 122 --squash --delete-branch=false
```

Expected: checks green, merge succeeds. If a check is red, stop — nothing below
is worth doing on a red main.

- [ ] **Step 4: Move the working tree to a branch off the new main**

```bash
git stash push --include-untracked -m "bug-bounty pass"
git checkout main && git pull --ff-only
git checkout -b fix/bug-bounty-2.6
git stash pop
git status --porcelain | wc -l   # expect 66 again
```

If `git stash pop` conflicts, stop: it means the squash merge produced a tree
that differs from what the branch had, and the split needs re-planning.

- [ ] **Step 5: Confirm the starting point is green**

```bash
go build ./... && go vet ./... && go test ./internal/... 2>&1 | tail -5
```

Expected: `ok` for `internal/core`, `internal/shared`, `internal/web`.

---

### Task 1: The conffile that stopped an upgrade

**Files:**
- Modify: `debian/rules`, `debian/postinst`, `docs/configuration.md` (one paragraph — see the split technique), `docs/installation/debian.md`, `CHANGELOG.md`
- Test: `internal/shared/conffiles_test.go` (new, staged whole)

**Interfaces:**
- Produces: `easywall.toml.template` in the package payload; `TestNeitherConfigIsShippedAsAConffile`.

- [ ] **Step 1: Reduce `docs/configuration.md` to this task's paragraph**

Keep only the paragraph beginning "The package and the container install both,
already filled in — from `*.toml.template`". Revert the CIDR paragraph (Task 6)
and the limits table (Task 3) to their `HEAD` text with `git show HEAD:docs/configuration.md`.

- [ ] **Step 2: Reduce `CHANGELOG.md` to this task's paragraph**

Keep only "**An unattended package upgrade could leave easywall unconfigured, with
the old processes still serving.**" under `### Fixed`. Everything else in the
working-tree `CHANGELOG` goes back to `HEAD` for now.

- [ ] **Step 3: Run the test that this commit adds**

```bash
go test ./internal/shared/ -run TestNeitherConfigIsShippedAsAConffile -v
```

Expected: PASS. (It reads `debian/`, so it passes as soon as the files are on
disk — which they are. It is here to confirm the file was not left behind.)

- [ ] **Step 4: Commit**

```bash
git add debian/rules debian/postinst internal/shared/conffiles_test.go \
        docs/installation/debian.md docs/configuration.md CHANGELOG.md
git commit -m "fix(deb): ship easywall.toml as a template, not a conffile

easywall rewrites that file on every saved setting, which dpkg reads as
'modified by you or by a script'. An upgrade that also changed the shipped
default stopped at the conffile prompt and left the package 'install ok
unpacked': new binaries on disk, postinst never run, the old processes still
serving. web.toml has had the template arrangement all along, for this reason.

<TRAILERS>"
```

- [ ] **Step 5: Verify the tree still builds**

```bash
go build ./... && go vet ./...
```

---

### Task 2: One deadline for fifteen commands

**Files:**
- Modify: `internal/shared/protocol.go`, `internal/web/client.go`, `CHANGELOG.md`
- Test: `internal/web/client_timeout_test.go` (new, staged whole)

**Interfaces:**
- Consumes: nothing.
- Produces: `shared.NftTimeout` (`30 * time.Second`) and `shared.CommandTimeout(cmd shared.CommandType) time.Duration`, used by `internal/web/client.go`. `clientTimeout` is renamed `dialTimeout` and now bounds only `net.DialTimeout`.

- [ ] **Step 1: Reduce `CHANGELOG.md` to this task's paragraph**

Keep "**An import that succeeded was reported as failed.**"

- [ ] **Step 2: Run the tests this commit adds**

```bash
go test ./internal/web/ -run 'TestAnImportIsNotAbandonedWhileTheCoreIsStillValidatingIt|TestAStatusPollStillGivesUpQuickly|TestTheClientOutwaitsWhatTheCoreMaySpendOnNft' -v
```

Expected: three PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/shared/protocol.go internal/web/client.go \
        internal/web/client_timeout_test.go CHANGELOG.md
git commit -m "fix(web): a deadline per command instead of one for fifteen

IMPORT_RULES runs every custom rule past \`nft --check\` before storing
anything, which the core bounds at 30s; the client gave up at 5s. So the
staged rule set had been replaced and the interface reported the import as
failed — and the next move after that is to retry, or to apply, on top of a
set that is not the one on screen.

<TRAILERS>"
```

- [ ] **Step 4: Verify**

```bash
go build ./... && go vet ./...
```

---

### Task 3: One table for all nine limits

**Files:**
- Modify: `internal/shared/models.go`, `internal/core/config.go` (limits parts), `internal/web/handler_options.go`, `internal/web/server.go` (one key), `web/static/app.js` (one line), `locales/en.json`, `locales/de.json` (one key each), `web/templates/options.html`, `docs/schemas/easywall.schema.json`, `docs/configuration.md` (limits table), `CHANGELOG.md`
- Test: `internal/core/config_limits_test.go`, `internal/web/firewall_limits_test.go` (new, staged whole)

**Interfaces:**
- Consumes: nothing.
- Produces: `shared.FirewallLimits` — a slice whose elements expose `Key string`, `Min`, `Max`, `Default int`, `Enabled(*shared.FirewallOptions) *bool`, `Value(*shared.FirewallOptions) *int`, `InRange(int) bool`, `Clamp(int) int`. Consumed by `internal/core/config.go` (`Validate`, `SaveFirewallOptions`), `internal/web/handler_options.go`, and both guard tests. Locale key `options_invalid_limit`.

- [ ] **Step 1: Reduce the shared files to this task's parts**

- `internal/core/config.go`: keep the `shared.FirewallLimits` loop in `Validate`, the deletion of the local `firewallLimit` type and `firewallLimits()`, the comment that replaces them, and the `SaveFirewallOptions` loop. Revert the network parts: `Validate` must **not** yet call `checkNetworkLists`, `checkCIDRList` stays as it is at `HEAD`, and the `"net"` import stays.
- `internal/web/server.go`: add `"options_invalid_limit"` to `clientStringKeys` and to the warning map in `templateFuncs`. Nothing else — the route stays `r.Get("/logout", …)`.
- `web/static/app.js`: keep the `options_invalid_limit:` line only.
- `locales/{en,de}.json`: keep `options_invalid_limit` only.
- `docs/configuration.md`: keep the limits table and the 32-bit wrapping note only.

- [ ] **Step 2: Verify it compiles before the tests run**

```bash
go build ./... && go vet ./...
```

Expected: clean. If `config.go` fails on an unused `"net"` import, the network
half was reverted too far — `checkCIDRList` must still be present.

- [ ] **Step 3: Run the tests this commit adds**

```bash
go test ./internal/core/ -run 'TestAnOutOfRangeLimitIsRefusedOnTheWayIn|TestAnOutOfRangeLimitInTheFileIsClampedAndSaidOutLoud|TestEveryFirewallLimitIsWiredToItsOwnField' -v
go test ./internal/web/ -run TestTheAdvertisedLimitsAreTheOnesTheDaemonEnforces -v
```

Expected: four PASS. The last one is the guard that derives `options.html`'s
`max` attributes and both JSON Schemas from the Go table — if it fails, the
template or the schema was left at its `HEAD` version.

- [ ] **Step 4: Reduce `CHANGELOG.md` and commit**

Keep "**One number on the options page could turn the firewall into a total
block.**"

```bash
git add internal/shared/models.go internal/core/config.go internal/web/handler_options.go \
        internal/web/server.go web/static/app.js locales/en.json locales/de.json \
        web/templates/options.html docs/schemas/easywall.schema.json docs/configuration.md \
        internal/core/config_limits_test.go internal/web/firewall_limits_test.go CHANGELOG.md
git commit -m "fix(core): one table for the range of all nine firewall limits

The page said max=9999, the schema said 100/1000/10000/100000 for five of
them and nothing for four, and the daemon checked only that an enabled
module's limit was positive. The values land in 32-bit nftables fields, so
too large did not fail — it wrapped: connection_limit_max = 4294967296
arrived as \`ct count over 0\`, which matches every connection from every
source and drops it, with nothing logged.

shared.FirewallLimits now carries key, range and default for all nine,
including the two log limits nothing validated at all. Out of range in the
file is clamped and said out loud; out of range from the interface is refused
with the key named.

<TRAILERS>"
```

---

### Task 4: A snapshot that described chains that do not exist

**Files:**
- Modify: `internal/core/nftables.go`, `CHANGELOG.md`
- Test: `internal/core/nftables_snapshot_test.go` (new, staged whole)

**Interfaces:**
- Consumes: nothing.
- Produces: `Snapshot` matches chains on name **and** family; an unreadable rule count is `null` with its error beside it rather than `0`.

- [ ] **Step 1: Run the tests this commit adds**

```bash
go test ./internal/core/ -run 'TestSnapshotAttributesEachChainToItsOwnFamily|TestEnforcingIgnoresASameNamedTableInAnotherFamily' -v
```

Expected: two PASS.

- [ ] **Step 2: Reduce `CHANGELOG.md` and commit**

Keep "**The post-incident nftables snapshot described chains that do not
exist.**"

```bash
git add internal/core/nftables.go internal/core/nftables_snapshot_test.go CHANGELOG.md
git commit -m "fix(core): attribute each chain to its own family in the snapshot

Chains were matched to tables by name alone, so every table was credited with
the chains of every same-named table in another family and the rule counts
beside them were read from the wrong table. A hand-written \`table ip
easywall\` next to easywall's \`table inet easywall\` produced one chain
reported as three, including a decoy(0) — a chain that is not in that table,
reported as existing and empty, because the lookup failed and the count kept
its zero value. This file is what an operator opens after a lockout.

<TRAILERS>"
```

- [ ] **Step 3: Verify**

```bash
go build ./... && go vet ./...
```

---

### Task 5: Signing out is a POST

**Files:**
- Modify: `internal/web/server.go` (route), `web/templates/base.html`, `web/src/app.css`, `web/static/style.css`, `scripts/ui-check.mjs` (sign-out check only), `internal/web/handler_login_test.go`, `internal/web/session_lifetime_test.go`, `CHANGELOG.md`
- Test: `internal/web/handler_logout_method_test.go` (new, staged whole)

**Interfaces:**
- Consumes: nothing.
- Produces: `POST /logout`; the sidebar control is `form[method=POST] > button.logout-btn`; `checkSignOutEndsTheSession(page)` in `scripts/ui-check.mjs`.

- [ ] **Step 1: Reduce the shared files to this task's parts**

- `internal/web/server.go`: change `r.Get("/logout", s.handleLogout)` to `r.Post(…)` with its comment. `options_invalid_limit` is already committed; `settings_invalid_network` must **not** appear yet.
- `scripts/ui-check.mjs`: keep `checkSignOutEndsTheSession` and its call site. The `signIn` rework and the 429 message belong to Task 7 — revert those to `HEAD`.

- [ ] **Step 2: Run the tests this commit adds and the ones it changes**

```bash
go test ./internal/web/ -run 'TestSigningOut|TestHandleLogout|TestLogoutSurvives|TestSessionRefusal' -v
```

Expected: all PASS. `TestSigningOutIsNotReachableWithASafeMethod` expects 405
for `GET /logout`; `TestSigningOutRefusesACrossOriginPost` expects 403.

- [ ] **Step 3: Confirm the built stylesheet matches its source**

```bash
cp web/static/style.css /tmp/style.check.css
npm run build:css >/dev/null && diff -q /tmp/style.check.css web/static/style.css && echo CURRENT
```

Expected: `CURRENT`. Tailwind drops rules silently — a green build is not proof
`.logout-btn` shipped:

```bash
grep -c 'logout-btn' web/static/style.css   # expect >= 2 (base + :hover)
```

- [ ] **Step 4: Reduce `CHANGELOG.md` and commit**

Keep "**Any page the operator had open could sign them out of the firewall's
interface.**" It stays under `### Fixed` — that was decided in the spec.

```bash
git add internal/web/server.go web/templates/base.html web/src/app.css web/static/style.css \
        scripts/ui-check.mjs internal/web/handler_login_test.go \
        internal/web/session_lifetime_test.go internal/web/handler_logout_method_test.go \
        CHANGELOG.md
git commit -m "security(web): signing out is a POST, so CSRF protection covers it

Go's CrossOriginProtection checks Origin and Sec-Fetch-Site on unsafe methods
only, because a safe method is not supposed to change anything. /logout was a
GET, so one <img src=\"https://the-host:12227/logout\"> on an unrelated site
ended the session and revoked the cookie. The control looks and reads exactly
as it did; only the method changed. check:ui now drives the button rather than
the URL, because a template that goes back to an anchor leaves every Go test
green.

<TRAILERS>"
```

---

### Task 6: One definition of "is that a network"

Carries **both** spec entries — the page half and the file half — because
`TestTheNetworkEditorRefusesExactlyWhatTheCoreRefuses` asserts the two agree in
one assertion and cannot be split.

**Files:**
- Modify: `internal/shared/validate.go`, `internal/core/config.go` (network parts), `internal/web/handler_iplist_validate.go`, `internal/web/handler_settings.go`, `internal/web/democlient.go`, `internal/web/server.go` (one key), `web/static/app.js` (one line), `locales/en.json`, `locales/de.json`, `web/templates/settings.html`, `docs/configuration.md` (CIDR paragraph), `docs/features/docker.md`, `docs/features/system-settings.md`, `docs/assets/img/screens/settings-{dark,light}.png`, `CHANGELOG.md`
- Test: `internal/shared/validate_networks_test.go`, `internal/core/config_networks_test.go`, `internal/web/handler_settings_networks_test.go` (new, staged whole)

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `shared.ValidateNetworkList(what string, entries []string) error` — skips comments and blanks (`shared.IsListComment`), requires `net.ParseCIDR` of the rest. `core.checkNetworkLists(dockerNetworks, routingNetworks []string) error` replaces `checkCIDRList`. `web.validateCIDRListEntries(raw string) []lineError`. Locale key `settings_invalid_network`.

- [ ] **Step 1: Restore every shared file whole**

This is the last task owning `internal/core/config.go`, `internal/web/server.go`,
`web/static/app.js` and both locale files:

```bash
for f in internal/core/config.go internal/web/server.go web/static/app.js \
         locales/en.json locales/de.json docs/configuration.md; do
  cp "$SNAP/$f" "$f"
done
git diff --stat -- internal/core/config.go internal/web/server.go web/static/app.js \
                   locales/en.json locales/de.json docs/configuration.md
```

Expected: only this task's parts remain unstaged — everything else is committed.
`docs/configuration.md` is finished by this task too (Tasks 1 and 3 took their
paragraphs).

- [ ] **Step 2: Verify it compiles**

```bash
go build ./... && go vet ./...
```

- [ ] **Step 3: Run the tests this commit adds**

```bash
go test ./internal/shared/ -run 'TestNetworkListsKeepCommentsAndBlanksTheEditorAllows|TestNetworkListsRefuseWhatWouldProduceNoRule' -v
go test ./internal/core/ -run 'TestABadNetworkInTheConfigFileStopsTheDaemon|TestNetworkListsInTheConfigFileMayCarryComments|TestSaveNetworkSettingsAcceptsWhatTheEditorSends' -v
go test ./internal/web/ -run 'TestTheNetworkEditorRefusesExactlyWhatTheCoreRefuses|TestTheDemoRefusesANetworkTheCoreWouldRefuse|TestClientStringsCarryNoMarkupAppJSCannotRender' -v
```

Expected: eight PASS. `TestClientStringsCarryNoMarkupAppJSCannotRender` fails if
a string reaching app.js carries backticks or asterisks — the two `*_networks_help`
texts do carry a backtick, which is why `settings.html` renders the Docker one
through `richText`; the help texts are not client strings.

- [ ] **Step 4: Confirm both locales stayed at parity**

```bash
go test ./internal/web/ -run 'Locale|Translation|I18n' 2>&1 | tail -3
```

Expected: PASS — every id in `en.json` exists in `de.json`.

- [ ] **Step 5: Reduce `CHANGELOG.md` to this task's two paragraphs and commit**

Keep "**A blank line between two networks on the Network page made the save fail,
and blamed the core for it.**" and "**A mistyped network in `easywall.toml`
reached the kernel as no rule at all.**"

```bash
git add internal/shared/validate.go internal/core/config.go internal/web/handler_iplist_validate.go \
        internal/web/handler_settings.go internal/web/democlient.go internal/web/server.go \
        web/static/app.js locales/en.json locales/de.json web/templates/settings.html \
        docs/configuration.md docs/features/docker.md docs/features/system-settings.md \
        docs/assets/img/screens/settings-dark.png docs/assets/img/screens/settings-light.png \
        internal/shared/validate_networks_test.go internal/core/config_networks_test.go \
        internal/web/handler_settings_networks_test.go CHANGELOG.md
git commit -m "fix(networks): one definition of what a network is, in all three places

The editor validated with the blacklist's rules, which accept a bare address
and skip comments; the core demanded net.ParseCIDR of every element including
blank lines and comments; the demo checked nothing. So a blank line between
two networks was accepted by the page, refused by the core, and reported to
the operator as 'Failed to save changes. Check core connection.' — with a core
that was answering perfectly.

The other direction was worse: nothing looked at these lists when they arrived
in easywall.toml, and system-settings.md tells operators to edit that file and
send SIGHUP. Measured against a real kernel, networks = [\"10.8.0.0/24\",
\"10.9.0.0-24\"] started the daemon with no warning and a forward chain
holding the accept for the first and none for the second — destroyed by the
drop policy without a word.

shared.ValidateNetworkList is the one definition now; comments and blanks are
skipped, as everywhere else a list is typed by hand.

<TRAILERS>"
```

---

### Task 7: `check:ui` could not be run twice in ten minutes

**Files:**
- Modify: `scripts/ui-check.mjs` (restore whole), `CHANGELOG.md`

**Interfaces:**
- Consumes: `checkSignOutEndsTheSession` from Task 5 — already committed; this task must not touch it.
- Produces: `signIn(page)` called once and reused by every context; a 429 named as a 429.

- [ ] **Step 1: Restore the file whole**

```bash
cp "$SNAP/scripts/ui-check.mjs" scripts/ui-check.mjs
git diff --stat -- scripts/ui-check.mjs
```

Expected: only the `signIn` rework remains — Task 5's part is already in.

- [ ] **Step 2: Reduce `CHANGELOG.md` and commit**

Keep "**`npm run check:ui` could not be run twice in ten minutes, and blamed the
interface when it failed.**"

```bash
git add scripts/ui-check.mjs CHANGELOG.md
git commit -m "fix(scripts): sign in once in check:ui, and name a 429 as a 429

/login allows five attempts per ten minutes per source address, and this
script spent three of them on every run — one per browser context. Two runs
inside ten minutes hit the limiter on the sixth attempt and reported 'could
not sign in', which reads as a broken login page. CI never saw it: a fresh
runner each time. So it was a trap set for whoever ran the checks locally
twice while working on something.

<TRAILERS>"
```

---

### Task 8: The start rate limit, in the section systemd documents it in

**Files:**
- Modify: `systemd/easywall-core.service`, `systemd/easywall-web.service`, `CHANGELOG.md`

- [ ] **Step 1: Check both units still parse**

```bash
systemd-analyze verify systemd/easywall-core.service systemd/easywall-web.service 2>&1 | head
```

Expected: no output about `StartLimitIntervalSec` or `StartLimitBurst`. Warnings
about a missing binary at `/usr/bin/easywall-core` are expected on a host with no
package installed and are not a failure of this change — say so if they appear.
If `systemd-analyze` is unavailable, report the step as skipped.

- [ ] **Step 2: Reduce `CHANGELOG.md` and commit**

Keep "**The start rate limit moved to the section systemd documents it in.**"

```bash
git add systemd/easywall-core.service systemd/easywall-web.service CHANGELOG.md
git commit -m "fix(systemd): put the start rate limit in [Unit], where it belongs

systemd moved StartLimitInterval and StartLimitBurst out of [Service] in v229
and keeps the old spelling as a compatibility alias, so this was working —
measured on systemd 259. It is written where it is documented anyway: an alias
can be removed, and the failure would be silent. The unit would fall back to
the manager defaults and a crash-looping firewall daemon would keep restarting
instead of stopping.

<TRAILERS>"
```

---

### Task 9: Diagrams laid out in a font that is present

**Files:**
- Modify: `scripts/render-diagrams.mjs`, `docs/assets/diagrams/*.svg` (14 files), `CHANGELOG.md`

**Interfaces:**
- Produces: the source digest covers `docs/assets/fonts/inter-var.woff2` as well as the `.mmd` and the mermaid version; Inter is embedded as a data URI and awaited before any measurement.

- [ ] **Step 1: Confirm the committed SVGs are what this renderer produces**

```bash
npm run check:diagrams
```

Expected: `7 diagram(s), all current`. If it reports a stale diagram, re-render
with `npm run build:diagrams` **in this task**, not later.

- [ ] **Step 2: Confirm the font is actually loaded, not silently fallen back on**

The script throws `Inter did not load; the layout would be measured in a
fallback font` if the face is unusable. A successful `build:diagrams` is that
proof:

```bash
npm run build:diagrams 2>&1 | tail -3
git diff --stat -- docs/assets/diagrams | tail -1
```

Expected: it completes. Up to six files may show as changed with no visible
difference — the renderer's comment explains why (run-dependent bezier control
points, all measured at 0.000000 px off the line). Restore them from the
snapshot rather than committing churn:

```bash
cp "$SNAP"/docs/assets/diagrams/*.svg docs/assets/diagrams/
```

- [ ] **Step 3: Reduce `CHANGELOG.md` and commit**

Keep "**The documentation diagrams were laid out in whatever font the rendering
machine happened to have.**"

```bash
git add scripts/render-diagrams.mjs docs/assets/diagrams CHANGELOG.md
git commit -m "fix(docs): render the diagrams in the font the site serves

The themes ask for Inter and nothing ever loaded it into the rendering page,
so the layout was measured in whatever the machine resolved that to. mermaid
sizes every node from the text inside it, so the font decides the box widths,
which decide the edge geometry — re-rendering on a host without Inter moved
the control points in six of the fourteen committed files with the same
sources and the same mermaid. The digest could not see it, so --check called
them current. Inter is embedded and awaited now, and the font is part of the
digest.

<TRAILERS>"
```

---

### Task 10: Three things that were true and had nothing holding them true

**Files:**
- Modify: `.github/workflows/docs.yml`, `CHANGELOG.md`
- Test: `internal/shared/package_version_test.go`, `internal/web/diagram_palette_test.go` (new, staged whole)

**Interfaces:**
- Consumes: `shared.CurrentVersion`.
- Produces: `TestThePackageVersionIsTheReleaseVersion` (compares `debian/changelog` to `shared.CurrentVersion`), `TestTheDiagramPaletteIsTheDocumentationPalette` (compares the token literals in `render-diagrams.mjs` to `web/src/docs.css`), and a "the site is not empty" step on the deploying job.

- [ ] **Step 1: Run both new guards**

```bash
go test ./internal/shared/ -run TestThePackageVersionIsTheReleaseVersion -v
go test ./internal/web/ -run TestTheDiagramPaletteIsTheDocumentationPalette -v
```

Expected: two PASS.

- [ ] **Step 2: Watch each one fail for its own reason**

A guard that has never been seen red is a guess. Break each, confirm the
failure, restore:

```bash
sed -i 's/^easywall (2\.5\.1)/easywall (2.5.99)/' debian/changelog
go test ./internal/shared/ -run TestThePackageVersionIsTheReleaseVersion   # expect FAIL
git checkout -- debian/changelog
go test ./internal/shared/ -run TestThePackageVersionIsTheReleaseVersion   # expect PASS
```

Do the same for the palette guard by changing one hex value in
`scripts/render-diagrams.mjs`, then `git checkout -- scripts/render-diagrams.mjs`
— it is committed as of Task 9, so the checkout is safe.

- [ ] **Step 3: Check the workflow is valid YAML and the new step is on the deploying job**

```bash
python3 -c "import yaml,sys; d=yaml.safe_load(open('.github/workflows/docs.yml')); print(list(d['jobs']))"
grep -n -B2 'The site is not empty' .github/workflows/docs.yml
```

Expected: the step sits in the job that runs `actions/upload-pages-artifact`, not
in the pull-request job.

- [ ] **Step 4: Reduce `CHANGELOG.md` and commit**

Keep "**Three things that were true and had nothing holding them true.**"

```bash
git add internal/shared/package_version_test.go internal/web/diagram_palette_test.go \
        .github/workflows/docs.yml CHANGELOG.md
git commit -m "test: hold three things that were true and unwatched

The Debian package version was checked only by the release workflow, at the
upload step, after GoReleaser had already published the images. The docs
deploy asserted 'the site is not empty' on the pull-request build and not on
the job that ships. And the diagram renderer carried the design tokens as two
literal blocks under a 'keep them in step with docs.css' comment, where a
changed token would have left fourteen committed pictures in the old palette
with check:diagrams still calling them current — the digest covers the source
and the mermaid version, not the colours. Each was watched red.

<TRAILERS>"
```

---

### Task 11: Two comments that described code that had changed

**Files:**
- Modify: `.golangci.yml`, `internal/core/acceptance.go`, `CHANGELOG.md`

- [ ] **Step 1: Write the missing changelog sentence**

`internal/core/acceptance.go` has no entry. Extend the existing paragraph
"**The linter's own comment claimed a permission the code had already
changed.**" to cover both, and retitle it:

```markdown
- **Two comments that described code that had changed.** `.golangci.yml`
  justified the `G302` exclusion with "socket (0660) and audit log (0640) are
  intentional" a release after the audit log's group bit was deliberately
  removed and it became `0600` — with a paragraph in `WriteAuditLog` explaining
  why. The exclusion is the socket's alone now. And `Acceptance.Start` promised
  "returns an error if an acceptance is already in progress", which it never
  does: starting inside an open window is a no-op, and the guard against a
  second apply is `beginApply`, which claims the slot synchronously and refuses
  with `ErrApplyInProgress`. The comment made the error check at the one call
  site read as the guard it is not
```

- [ ] **Step 2: Confirm the comment now matches the code**

```bash
go test ./internal/core/ -run TestAcceptance -v 2>&1 | tail -20
```

Expected: `TestAcceptance_StartIdempotent` PASS — it is the test the new comment
cites. If no such test exists, stop: the comment must not cite a test by a name
nothing answers to.

- [ ] **Step 3: Confirm the linter is still clean with the narrowed exclusion**

```bash
make lint
```

Expected: `0 issues`. A `G302` finding here would mean the audit log's `0600` was
being suppressed by the old wording.

- [ ] **Step 4: Commit**

```bash
git add .golangci.yml internal/core/acceptance.go CHANGELOG.md
git commit -m "docs(code): two comments that described code that had changed

The G302 exclusion claimed the audit log is 0640 a release after its group bit
was deliberately removed; it is the socket's exclusion alone now. And
Acceptance.Start promised an error it never returns, which made the error
check at its one call site read as the guard against a second apply — that is
beginApply.

<TRAILERS>"
```

---

### Task 12: Prove the split lost nothing

**Files:** none modified.

**Interfaces:**
- Consumes: `$SNAP` from Task 0.

- [ ] **Step 1: The working tree must be clean**

```bash
git status --porcelain
```

Expected: **empty**. Anything listed is a hunk that never found a commit — find
its owner and amend that commit rather than adding a thirteenth.

- [ ] **Step 2: The committed tree must equal the snapshot, file for file**

```bash
cd "$SNAP" && find . -type f | while read -r f; do
  diff -q "$f" "/var/home/jpy/projects/easywall/${f#./}" >/dev/null || echo "DIFFERS: $f"
done; cd -
```

Expected: no output. A `DIFFERS` line means the split changed content rather than
just distributing it — that is a defect in the split, not a judgement call.

- [ ] **Step 3: Every commit in the series builds**

```bash
git rebase --exec 'go build ./... && go vet ./...' main
```

Expected: eleven successful execs, no stop. This is where a shared-file split
that compiled only at the end shows up.

- [ ] **Step 4: The whole suite, and the linter**

```bash
make test lint
```

Expected: three `ok` lines and `0 issues`.

---

### Task 13: Verify against a real kernel

**Files:** none modified.

- [ ] **Step 1: Run the integration suite**

```bash
sudo go test -tags integration ./internal/core/... 2>&1 | tail -20
```

Expected: `ok`. This is the only check that exercises Task 3 and Task 4 against
nftables rather than against Go structs. Requires root — if it cannot be run,
say so plainly and do not describe the release as verified.

- [ ] **Step 2: Confirm the clamp reaches the kernel**

The bug was that `connection_limit_max = 4294967296` arrived as `ct count over 0`.
With the integration suite green, confirm the ruleset the daemon produces for an
out-of-range value:

```bash
sudo nft list ruleset | grep -n 'ct count over' || echo "no connection limit rule present"
```

Expected: any `ct count over N` has `N >= 1`. `ct count over 0` is a failure of
this release, not a curiosity.

---

### Task 14: Verify in a real browser

**Files:** possibly `docs/assets/img/screens/*.png` — see Step 3.

- [ ] **Step 1: Run the interface checks**

```bash
npm run check:ui 2>&1 | tail -30
```

Expected: `UI checks passed`, including `checkSignOutEndsTheSession`. If it
reports a 429, the message from Task 7 explains it: wait out the ten-minute
window or restart `easywall-web`.

- [ ] **Step 2: Render the sidebar footer and compare it to what shipped**

The claim in `base.html` is that the sign-out control "looks and reads exactly as
it did". Confirm it, in both themes, at 1600 / 900 / 390 px. Use the demo-mode
server the UI check drives, and capture the sidebar footer at each combination.

Expected: the control is the same height, the same colour, and the same hover
treatment as the anchor it replaced; at 390 px it does not stretch to full width
in a way the anchor did not.

- [ ] **Step 3: Decide about the screenshots, and act on it**

If Step 2 shows *any* visible difference, all twenty screenshots under
`docs/assets/img/screens/` are stale and must be re-taken in this pass, both
themes, and committed as `docs(screens): re-take after the sign-out control
changed`. If there is no visible difference, say so explicitly — "unchanged,
compared at three widths in both themes" — and re-take nothing. The two settings
screenshots were already re-taken in Task 6 for the help text.

---

### Task 15: Verify the upgrade dpkg actually performs

**Files:** none modified.

- [ ] **Step 1: Build the package from this branch**

```bash
dpkg-buildpackage -us -uc -b 2>&1 | tail -5
ls -la ../easywall_*.deb
```

- [ ] **Step 2: Take the upgrade in a container, from a 2.5.1 that carries the conffile**

```bash
podman run --rm -it -v "$PWD/..":/pkgs:ro debian:trixie bash -c '
  set -e
  apt-get update -qq
  # install 2.5.1 (the release asset), save a setting so the file is "modified"
  # then upgrade to the package built above with a plain apt-get install
  '
```

Expected, and each one must be seen rather than assumed:
- the upgrade prints **no** conffile prompt,
- `dpkg-query -W -f='${Status} ${Version}\n' easywall` ends `install ok installed`,
- `/etc/easywall/easywall.toml` still holds the saved setting, `root:root`, `0600`,
- `/etc/easywall/easywall.toml.template` exists beside it.

- [ ] **Step 3: Take a fresh install in the same image**

Expected: both config files created from their templates, the permission layout
the Build workflow asserts, both services startable.

If neither podman nor docker is available, report Task 15 as **skipped** and say
which of Task 1's claims is therefore unverified locally — CI's Build workflow
installs the package, which covers the fresh install but not the upgrade.

---

### Task 16: The pull request

**Files:** none modified.

- [ ] **Step 1: Push and open it**

```bash
git push -u origin fix/bug-bounty-2.6
gh pr create --base main --title "Zwölf Defekte aus dem Bug-Bounty-Durchlauf" --body "$(cat <<'EOF'
Eleven commits, twelve defects — the page half and the file half of the network
validation are one commit, because the test that proves them asserts they agree.

Three of them decide whether an operator can reach their own machine:

- `connection_limit_max = 4294967296` wrapped to `ct count over 0`, a rule that
  drops every connection from every source, with nothing logged.
- `GET /logout` sat outside `CrossOriginProtection`, which exempts safe methods
  by design, so any page the operator had open could end their session.
- A dpkg conffile prompt left an upgrade at `install ok unpacked` — new binaries
  on disk, postinst never run, the old processes still serving.

Verified beyond `make test lint`: the integration suite against a real kernel,
`npm run check:ui` in a real browser including the sign-out control, and the
2.5.1 → this upgrade in a `debian:trixie` container.

Plan: `docs-tech/plans/2026-08-14-landing-the-bug-bounty-pass-and-2.6.0.md`

🤖 Generated with [Claude Code](https://claude.com/claude-code)

https://claude.ai/code/session_019GDuzEucHPoB2cBmFYSiPM
EOF
)"
```

- [ ] **Step 2: Wait for CI and read what it says**

```bash
gh pr checks --watch
```

Expected: all green. A red check is fixed in this pass, in the commit that owns
it — not in a follow-up commit at the end.

- [ ] **Step 3: Merge**

```bash
gh pr merge --squash --delete-branch
git checkout main && git pull --ff-only
```

Note: a squash merge collapses the eleven commits on `main`. If the eleven should
survive in the history — they are the reason for the split — use `--merge`
instead. **Ask before merging.**

---

### Task 17: Cut 2.6.0

**Files:**
- Modify: `internal/shared/version.go`, `docs/_config.yml`, `debian/changelog`, `CHANGELOG.md`, `docs/roadmap.md`

- [ ] **Step 1: Move the version in all three places**

```bash
sed -i 's/var CurrentVersion = "2.5.1"/var CurrentVersion = "2.6.0"/' internal/shared/version.go
sed -i 's/^version: "2.5.1"/version: "2.6.0"/' docs/_config.yml
```

Then add a `debian/changelog` entry at the top, in the existing style — the
package version is `2.6.0`, distribution `unstable`, urgency `medium`, with the
release's headline items as bullets and the maintainer line matching the entry
below it.

- [ ] **Step 2: Close the changelog section**

`## [Unreleased]` becomes `## [2.6.0] — <today's date, ISO>`, and the link
reference at the foot of the file gains
`[2.6.0]: https://github.com/jp1337/easywall/compare/v2.5.1...v2.6.0`. Leave a
fresh empty `## [Unreleased]` above it.

- [ ] **Step 3: Move the roadmap up one number**

In `docs/roadmap.md`: **2.7** Proof · **2.8** Identity · **2.9** Reach and the
trusted-proxy list · **2.10** counting installations. Replace *Done in 2.5* with
*Done in 2.6* listing the nine limits, the conffile, the import timeout, the
snapshot, the sign-out method, the arm64 package, `--write-config` and the
documentation split — and keep a shorter *Done in 2.5* below it.

- [ ] **Step 4: Run the guards that hold the three files together**

```bash
go test ./internal/shared/ -run 'TestThePackageVersionIsTheReleaseVersion|TestDocsVersionMatchesRelease' -v
go test ./internal/web/ -run TestEveryPageIsDocumented -v
make test lint
```

Expected: all PASS. The first two are exactly why Task 10 exists.

- [ ] **Step 5: Commit**

```bash
git add internal/shared/version.go docs/_config.yml debian/changelog CHANGELOG.md docs/roadmap.md
git commit -m "chore(release): 2.6.0

A minor, because it carries features: --write-config, the arm64 package, and
four documented pages the interface already had. The roadmap's themes each
move up one number — proof-not-counts is 2.7.

<TRAILERS>"
git push
```

- [ ] **Step 6: Tag, after asking**

Tagging publishes: the release workflow builds the archives, both `.deb`
packages and the container images. **Confirm with the maintainer first**, then:

```bash
git tag -a v2.6.0 -m "easywall 2.6.0"
git push origin v2.6.0
gh run watch
```

Expected: the release workflow succeeds and the release page carries the
tarballs, both `.deb` packages and the checksums. If the architecture check
fails at the upload step, the package matrix is wrong — that check exists
because a `.deb` named arm64 carrying amd64 binaries is the one mistake a file
name cannot show.

---

## Self-review

**Spec coverage:** conffile → T1 · import timeout → T2 · limits → T3 · snapshot →
T4 · logout → T5 · both network entries → T6 · check:ui → T7 · systemd → T8 ·
diagram font → T9 · three guards → T10 · golangci + acceptance comments → T11 ·
`rebase --exec` → T12 · integration suite → T13 · check:ui + nav footer + the
screenshot decision → T14 · debian upgrade → T15 · PR → T16 · version in three
files, changelog heading, roadmap, tag → T17. The spec's `check:diagrams` and
`build:css` checks live in T9 and T5, next to the change each one guards.

**Placeholders:** none. Task 15's container script is deliberately a shape rather
than a transcript — the 2.5.1 asset URL depends on what the release page serves —
and its four expected outcomes are each stated exactly.

**Type consistency:** `shared.FirewallLimits`, `shared.CommandTimeout`,
`shared.NftTimeout`, `shared.ValidateNetworkList`, `core.checkNetworkLists`,
`web.validateCIDRListEntries`, `logout-btn`, `options_invalid_limit`,
`settings_invalid_network` are each named identically in the file map, the
interfaces blocks and the commands. Verified against the working tree, not
recalled.
