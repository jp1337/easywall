# Landing the bug-bounty pass, and cutting 2.6.0

**Date:** 2026-08-14 · **Status:** approved, not yet executed

Not published. This directory sits outside `docs/`, which is the entire Jekyll
source — see `TestTheTechnicalDocsAreNotPublished`.

## The situation

`feat/write-config` is two commits ahead of `main` (open as PR #122), and the
working tree on top of it holds a complete bug-bounty pass that was never
committed: 55 files, ~911 insertions, 11 new test files, twelve `CHANGELOG`
entries. `go test ./internal/...` is green, `golangci-lint` reports 0 issues,
`check:diagrams` reports 7 diagrams current, and a fresh `build:css` reproduces
the committed `web/static/style.css` byte for byte.

Three of the twelve are the kind that decide whether an operator can reach their
own machine:

- `connection_limit_max = 4294967296` wrapped to `ct count over 0`, which matches
  every connection from every source and drops it.
- `GET /logout` sat outside `CrossOriginProtection`, which exempts safe methods
  by design, so any page the operator had open could end their session.
- A dpkg conffile prompt left an upgrade at `install ok unpacked` — new binaries
  on disk, postinst never run, old processes still serving.

Leaving that in a working tree is the problem this spec closes.

## Decisions

| Question | Decision |
|---|---|
| Order of work | Land what exists first; the 2.7 *Proof* work gets its own spec afterwards |
| Commit granularity | One commit per fix (~12), each carrying its test, its documentation and its `CHANGELOG` paragraph |
| Release number | **2.6.0** now. It contains features (`--write-config`, the arm64 `.deb`), so it is a minor; the roadmap's themes each move up one number |
| Disclosure of the CSRF finding | Stays under `### Fixed`. The entry already describes the attack in full; impact is a forced sign-out, no state change and no read access |
| PR shape | Merge #122 as it is, then a second branch `fix/bug-bounty-2.6` for the twelve, then the release commit |
| Spec location | `docs-tech/specs/`, never `docs/` |

## 1 — The commit split

| # | Commit | Files |
|---|---|---|
| 1 | `fix(deb)`: ship `easywall.toml` as a template, not a conffile | `debian/rules`, `debian/postinst`, `conffiles_test.go`, `docs/installation/debian.md` |
| 2 | `fix(web)`: a deadline per command instead of one for fifteen | `shared/protocol.go`, `web/client.go`, `client_timeout_test.go` |
| 3 | `fix(core)`: one table for all nine limits | `shared/models.go`, `core/config.go` ⚠, `handler_options.go`, `options.html`, `easywall.schema.json`, `configuration.md`, locales ⚠, `app.js` ⚠, `firewall_limits_test.go`, `config_limits_test.go` |
| 4 | `fix(core)`: match chains on name **and** family | `core/nftables.go`, `nftables_snapshot_test.go` |
| 5 | `security(web)`: signing out is a POST | `web/server.go`, `base.html`, `app.css`, `style.css`, `handler_login_test.go`, `session_lifetime_test.go`, `handler_logout_method_test.go` |
| 6 | `fix(web)`: check networks by the rules the core applies | `handler_iplist_validate.go`, `handler_settings.go`, locales ⚠, `app.js` ⚠, `handler_settings_networks_test.go` |
| 7 | `fix(core)`: one definition of "is that a network" | `shared/validate.go`, `core/config.go` ⚠, `democlient.go`, `validate_networks_test.go`, `config_networks_test.go`, `docker.md`, `system-settings.md` |
| 8 | `fix(scripts)`: `check:ui` twice within ten minutes | `scripts/ui-check.mjs` |
| 9 | `fix(systemd)`: the start rate limit in the section systemd documents it in | both `.service` files |
| 10 | `fix(docs)`: diagrams rendered in a font that is present | `render-diagrams.mjs` ⚠, 14 SVGs |
| 11 | `test`: three things that were true and had nothing holding them true | `package_version_test.go`, `diagram_palette_test.go`, `docs.yml`, `render-diagrams.mjs` ⚠ |
| 12 | `docs(code)`: two comments that described code that had changed | `.golangci.yml`, `core/acceptance.go` |

⚠ — touched by two commits; separated with `git add -p`. `CHANGELOG.md` is
touched by all twelve and is split the same way, one paragraph per commit.

**Loose end, resolved here:** `core/acceptance.go` changes only a doc comment —
it promised an error that `Start` never returns, which made the error check at
its one call site read as the guard against a second apply, when `beginApply` is.
It has no `CHANGELOG` entry today. It gets one short sentence and rides in commit
12, next to the `.golangci.yml` comment that claimed a `0640` the audit log had
already stopped having.

## 2 — Verification, before the PR

Each line exists because something in this pass could break it, not as ritual.

| Check | What it would catch |
|---|---|
| `git rebase --exec 'go build ./... && go vet ./...'` over the series | A twelve-way split of a mixed working tree easily leaves a middle commit that does not compile |
| `make test lint` | The state before the split was green; the split must not change that |
| `sudo go test -tags integration ./internal/core/...` | Fixes 3 and 4 are claims about the kernel, not about Go |
| `npm run check:ui` | Fix 8 changed the checking script itself; fixes 5 and 6 change what the browser sees |
| Nav footer rendered, both themes, 1600 / 900 / 390 | The code *claims* the sign-out control is visually unchanged after `<a>` → `<form><button>`. If it is not, all 20 screenshots under `docs/assets/img/screens/` are silently stale |
| `debian:trixie` container: 2.5.1 → this branch, with a setting saved first | Fix 1 is exactly this path. Expected: `install ok installed`, the operator's file untouched at `root:root 0600` |
| `check:diagrams`, `build:css` byte-identical | Already confirmed on 2026-08-14; re-run after the split |

Nothing is reported as passing that was not run. A check that cannot be run (no
root, no container) is named as skipped rather than assumed.

## 3 — The release

The version is written in three places and guarded by two tests
(`TestDocsVersionMatchesRelease`, `package_version_test.go`):

- `internal/shared/version.go` — `CurrentVersion = "2.6.0"`
- `docs/_config.yml` — `version: "2.6.0"`
- `debian/changelog` — a `easywall (2.6.0) unstable; urgency=medium` entry

Plus `CHANGELOG.md`: `## [Unreleased]` becomes `## [2.6.0] — <the tag's date>` with its
link reference at the foot of the file. Then tag `v2.6.0`, which is what the
release workflow triggers on.

`docs/roadmap.md` moves up one number — **2.7** Proof · **2.8** Identity ·
**2.9** Reach and the trusted-proxy list · **2.10** counting installations — and
gains a *Done in 2.6* section: the nine limits, the conffile, the import timeout,
the snapshot, the sign-out method, the arm64 package, `--write-config`, and the
documentation split.

## Order

1. Merge PR #122.
2. Branch `fix/bug-bounty-2.6` off the new `main`; carry the working tree over.
3. Split into the twelve commits above.
4. Run section 2 top to bottom; fix what it finds, in the same pass.
5. Open the PR, let CI run.
6. Merge, then a `chore(release): 2.6.0` commit with the version, the changelog
   heading and the roadmap.
7. Tag `v2.6.0`.

## What this spec does not cover

The 2.7 *Proof, not counts* work — tests that assert meaning rather than rule
counts, and real packets through a veth pair. It is a separate sub-project with
its own questions (where the tests run, how CI gets a kernel, what "meaning"
means per rule) and gets its own spec once 2.6.0 is out.
