# Carried forward

What a piece of work found and deliberately did not fix, with enough context to act
on. Every entry was reviewed, triaged and ruled on rather than forgotten; the
reasoning for deferring is part of the entry. Newest first.

**What may not go in here: anything the release caused, or anything belonging to
the feature it shipped.** A release is complete or it is not finished, and this file
is not a place to put the difference. An entry earns its place by being *found* by
the work rather than *made* by it — and that has to be proven against the base the
branch started from, not against the branch's own head, which already contains the
release's mistakes. 2.15 carried a `check:prose` failure as pre-existing on exactly
that error; comparing against `origin/main` showed its own earlier task had written
the sentence.

Not published — this directory sits outside `docs/`, which is the entire Jekyll
source. See `TestTheTechnicalDocsAreNotPublished`.

# From 2.16

Found while removing the accent, each proven against `origin/main` before this
branch existed.

| | |
|---|---|
| **`.callout-info` is the last blue on the documentation site** | `web/src/docs.css` hard-codes `rgba(56,189,248,…)` dark and `rgba(2,132,199,…)` light for the info callout — a sky blue wash and edge, no token, and not on the retired list this release swept. It predates the branch. It is now the closest surviving thing to the accent that was just removed, and if *colour means state* is the rule, an **info** callout in blue is the remaining exception. Left alone because the callout set (info / warning / success) is a semantic domain of its own that 2.16's scope did not open |
| **`TestNoRetiredHueSurvives` cannot be satisfied by a comment** | It does a plain `strings.Contains` over the whole file, unlike `TestNoAccentTokenSurvives`, which skips comment lines. So a comment that names the literal it removed — the natural way to record why a colour went — fails the test. Both comments that hit this were reworded to describe the hue instead of spelling it, which loses the grep-ability that made the note useful. The fix is to skip comments in that test too, and it is a change to a guard rather than to the thing guarded |
| **The three landing-page wrappers are still unstyled** | `docs/index.md` carries `.docs-landstrip`, `.docs-cardgrid` and `.docs-card`; `docs.css` defines none of them and Tailwind generates none, so the section renders as unstyled prose. `TestTemplateClassesExistInStylesheet` covers `web/templates/` and `app.js` only — nothing in the suite looks at markup under `docs/`, which is why this survived a release that rewrote the docs stylesheet |

# From the check:ui race fix

| | |
|---|---|
| **`checkPortsCatalogue` is not idempotent against a demo server that has already run it** | Picking Pi-hole a second time adds its rows to a set that already holds them, and the check fails with *"picking Pi-hole added 4 TCP rows, expected 2"*. Invisible in CI, which starts a fresh `easywall-web` for every run; it bites only a maintainer whose `scripts/demo-server.sh` has been up across two `npm run check:ui` invocations — where it reads as a real regression and costs a bisect. Reproduced on unmodified `main`, so it predates this branch. The fix is either a check that tolerates the rows already being there or a demo reset the script performs itself, and that is a decision about what `check:ui` may do to a maintainer's running session |

# From 2.15.1

Found while analysing the Discord lockout report, proven against `main` before
the hotfix branch existed, and unrelated to the two defects it fixes.

| | |
|---|---|
| **The landing page names three classes the stylesheet does not define** | `docs/index.md` carries `.docs-landstrip`, `.docs-cardgrid` and `.docs-card`; `web/src/docs.css` defines none of them, and Tailwind generates none. The section renders as unstyled prose. `TestTemplateClassesExistInStylesheet` covers `web/templates/` and `app.js` only, so nothing in the suite looks at markup under `docs/`. Belongs with 2.16, which is already rewriting `docs.css` |
| **`docs/installation/docker.md` tells a remote operator to open `https://localhost:12227`** | The one instruction that only ever works on the machine easywall is developed on. It is why both defects in this release went unseen: loopback is accepted, so the documented first step succeeds locally and fails on every VPS. The fix is a documentation change to a page 2.16 is not otherwise touching, and it wants the first-run flow rewritten around a remote host rather than one sentence changed |

# From 2.15

Two layout defects found while verifying the *Last used* column, both proven
independent of it, both belonging to 2.16 — *The interface looks like a
firewall* — where the tables are the subject rather than a passenger.

## The ports table's widths don't add up

| | |
|---|---|
| ~~**`.col-port` is 120px and `8000:9000` needs 122px**~~ | **Closed 2026-09-08.** 128px. The decision `DESIGN.md` did not answer was already answered four declarations below, in `.col-toggle`'s own comment: every pixel given back goes to `.col-flex`, which is `width: auto` and absorbs the remainder. There was never a zero-sum choice — only a rule nobody had written down, and `DESIGN.md` now carries it: *a data column is sized by its widest documented value, measured rendered rather than added up on paper*. Verified after: `8000:9000` needs 94px in the field and has 94 |
| ~~**`.table-wrap` overflows horizontally by 10px in card mode**~~ | **Closed 2026-09-08**, in two commits and deliberately not one. First the check: a container with `overflow-x: auto` absorbs its own overflow, so the document never widens and the page-level check cannot see it — by construction, for every such container in the stylesheet. `checkContainersDoNotOverflow` measures each one against its own laid-out children and went red 24 times, on all four table pages at exactly the three widths the container query switches at. Then the fix: a horizontal margin on a `width: 100%` block asks its container for room it does not have, and only one side displaces, which is why it was 10 and not 20. The inset moved to the container's own padding. Measured at a 714px container: 724px before, 714px after |

# From the 2026-08-30 documentation-site polish

Twenty-four planned tasks, five defects found while verifying them, and two features
added mid-run. What follows is what was seen and left alone, and why.

## Design decisions nobody has made yet

| | |
|---|---|
| **`security.md` overflows at 390px** | 482px against a 390px viewport, in both themes, caused by one `<code>` holding the Go test name `TestLoginVerify_TheSixteenthCodeAttemptDoesNotGetThrough`: it renders 462px wide and cannot wrap. Present before the branch. The obvious fix — `overflow-wrap` on `.content-body code` — would break **every** long identifier on the site at an arbitrary point, and a mid-token `proxy_set_header` may serve a firewall's reader worse than one page that scrolls sideways. `DESIGN.md` is silent on it |
| **The rouge theme leaves nine emitted token classes unstyled** | `.p` (162 occurrences), `.nt` (68 — nginx directives, YAML tags), `.nl` (58 — TOML and YAML **keys**), plus `.na`, `.kc`, `.no`, `.se`, `.si`, `.sh`. In the configuration reference a key therefore renders in body-text colour while its value is coloured. Highlighting is otherwise used correctly: 52 fences carry a language and the 11 that do not are output, paths and a CSP header, rightly plain. Note that labelling the `$ easywall-core …` blocks `console` would be a regression — the copy button would take the `$` prompt with it |
| **The changelog page has no on-page contents** | `docs/_layouts/default.html` filters headings inside `<details>`, all 30 version headings are, and the `heads.length < 3` rule then drops the column. Correct by that rule's own logic, and the longest page on the site is the one with no jump list |
| ~~**The application sidebar still has the weakness the docs sidebar lost**~~ | **Closed 2026-09-08.** Carried across three design reviews, and the entry 2.16 existed to close. The divider and the indent are sibling-scoped `:has()` selectors, so the first labelled group correctly gets neither — it follows the ungrouped *Dashboard* link, not another group — and a fourth group added to `base.html` later gets both without anyone remembering a class |

## Repository hygiene

| | |
|---|---|
| ~~**`main` has no required status checks**~~ | **Closed 2026-09-01.** Thirteen checks are now required and enforced for administrators; see *What protects `main`* in [ci-and-release](ci-and-release.md). Two things are worth recording. It did **not** close the case that prompted it — the pull request run was green and the `main` run of the same tree was red, so no merge gate would have seen it. And the release commit now needs a pull request, because a direct push carries no passing checks. `check:docs` stays duplicated in the docs deploy job: `docs.yml` is path-filtered, so its checks cannot be required without blocking every pull request that touches no `docs/**` file |
| **The spelling gate's scope does not match its configuration** | `.codespellrc` is written repo-wide and `codespell` runs repo-wide, but the only job that runs it triggers on `docs/**`, `.github/workflows/docs.yml`, `.github/actions/**`, `CHANGELOG.md` and `.codespellrc`. A typo in `README.md`, `CONTRIBUTING.md`, `DESIGN.md`, `locales/en.json`, `internal/**/*.go`, `config/`, `debian/`, `docker/` or `systemd/` is never checked. Moving the step to `test.yml` closes it |
| **The mark has two homes** | `web/static/icon.svg` and `docs/assets/img/icon.svg` are byte-identical copies. `DESIGN.md` calls the first "the single source of geometry", which the second quietly contradicts. Nothing has drifted yet |
| **Prose in a template leaks into the stylesheet** | Tailwind scans templates for class-like tokens and does not understand HTML comments, so an ordinary English word inside one is emitted as a utility rule. `.absolute` ships because `password.html` explains something about an absolute position; `.rounded` ships because `ports.html` describes a *rounded rectangle*. Harmless bytes, but each one is a diff against the committed stylesheet, and *Generated assets are current* fails on it — so a comment written in one pull request turns CI red in the next. Rebuild and commit the stylesheet whenever a template comment changes, or keep such words out of comments |
| **`npm run build:diagrams` is not byte-reproducible** | Mermaid jitters the bezier control points in `label-container outer-path`, so two runs over unchanged sources differ. Rebuild-and-diff is therefore **not** a staleness test for diagrams; `npm run check:diagrams`, which compares a `data-source-digest`, is. `docs/_docs/contributing.md` said otherwise and was corrected. **`CLAUDE.md` still carries the same over-broad rule** — "a generated file is rebuilt and diffed, never assumed" — which holds for the two stylesheets and the changelog page but not for the diagrams |

## Not carried — decided in this pass

- **The whitelist page ships without a screenshot.** Reusing `blacklist-*.png` with reworded alt text was the plan's instruction; another page's screenshot relabelled is a false statement in the documentation, and a real one needs demo mode running, which nothing in the plan sets up. An absence beats a wrong picture. A genuine `whitelist-*.png` is follow-up work for a branch that starts the application.
- **`docs/_docs/changelog.md` is excluded from the prose corpus.** It is generated from `CHANGELOG.md`, its bullets may not be edited, and it carries 214 sentences over 30 words — a checker that walked it could never pass. Excluded by exact path rather than a glob, so a future page cannot be swallowed silently.
- **`MAX = 30` and `AVG = 18` in `scripts/prose-check.mjs` were not softened** when the corpus looked far from them. It reached 0 over 30 and an average of 14.1 words without the target moving.

# From 2.7

## Operator-visible

| | |
|---|---|
| **`boot_enforce_failed` reads "at startup"** | 2.7 also writes it when a mid-apply panic teardown fails. The colour is right, the label is not, and `actionLabel` is what the audit filter searches — so somebody hunting a 15:00 teardown failure must search for "startup". Fixing it means rewording both locales and the *Reads as* column in `features/audit-log.md` |
| **`rollback_skipped`'s label** | Same shape, same fix: it now also covers a rollback that *was* written and then torn down. The detail says so; the label says "Rollback skipped" |
| **A forgotten panic mode is invisible to monitoring** | `easywall-core status` exits 0 under panic, deliberately — it is a state somebody chose. So a panic nobody remembers never pages anyone. Documented in the man page. A product decision, not a defect |
| **Every page pays a `GET_STATUS`** | **Shipped in 2.14** as `Server.statusForRender`, a ~2 s TTL cache read only by `render`. Not a passenger: the topbar countdown put a clock on every page, and a stall that used to cost the panic banner would have cost the countdown as well. Handlers that act on the status still ask the core directly |

## Invisible failures

| | |
|---|---|
| ~~**Four audit-silent paths in `apply`**~~ | **Closed 2026-09-07.** The second `GetState` (the re-read after promote), `BackupCurrent`, `PromoteStaged` and `acceptance.Start` now all write an audit entry before returning; see `TestEveryFailurePathInApplyIsAudited` in [invariants](invariants.md) |
| ~~**`acceptance.Start`'s error path**~~ | **Closed 2026-09-07.** This entry described 2.7's ordering. Since 2.14 the window opens before the kernel write, so what the error path left behind was not live rules but a stored `Current` holding an unconfirmed set — which the next boot or `resume` would install with no window at all. It now rolls back rather than return |

## Guards that do not see enough

| | |
|---|---|
| ~~**The kernel-write guard is scoped to two files**~~ | **Closed 2026-09-07.** `daemon_source_order_test.go` no longer enumerates `firewall.go` and `restore.go`; `coreSources` now globs every non-test source file in the package with `filepath.Glob`, so a fourth writer of the table added in a third file cannot be added invisibly |
| **…and cannot see reachability** | The same guard passes when the panic check is kept textually but wrapped in `if false`. Not closable without `go/parser` and constant folding; now named directly in the guard's own comment in `daemon_source_order_test.go` |
| **…nor call order beyond "after the write"** | Moving `apply`'s check to after `f.rollback` does not fire it, though a comment says the order matters — also now named in the guard's own comment |
| ~~**`bootBridges` has no test**~~ | **Closed 2026-09-07.** `TestRestoreCurrent_RecordsTheBridgesItBakedIn` writes through `RestoreCurrent` rather than the field directly, so deleting `setBootBridges` from it fails the suite |
| **The nft mutex is pinned only under `integration`** | `make test` cannot notice `mu sync.Mutex` being deleted. Self-documented in `nftables_mutex_test.go`, and CI's `test-integration` job does run it |

## Narrow races

| | |
|---|---|
| ~~**Marker check to netlink write is not atomic**~~ | **Closed 2026-09-07.** The known state is now passed into the helper rather than re-read a third time; see `TestPanicLandedDuringWriteIsToldTheMarkerState` in [invariants](invariants.md) |
| ~~**`Panic` and `Resume` share no lock**~~ | **Closed 2026-09-07.** `panicMu` serialises them; see `TestPanicAndResumeShareALock` in [invariants](invariants.md) |
| ~~**The accept loop `Add`s to `d.wg` from a counter of zero**~~ | **Closed 2026-09-07.** Shipped in 2.14.0 — `git show 04a0e4f:internal/core/daemon.go` line 212 is the bare `d.wg.Add(1)`, and 2.14 had no usage ticker at all, so the counter reached zero more readily there than on a default 2.15 install. This release's 300-second ticker holds a slot for the daemon's whole life and *hides* the hazard; the test-race fix beside it found the production half and recorded it as open. `Daemon.track` now takes the slot under the `d.mu` that `Stop` holds before it waits, so an accept either registers before the wait or is refused and the connection closed. Reproduced at 76 process runs in 160, 0 in 256 after; see `TestDaemonStart_StopRefusesAConnectionItCannotWaitFor` in [invariants](invariants.md) |

## Wording and hygiene

| | |
|---|---|
| ~~**`CHANGELOG.md`'s count claim**~~ | **Closed 2026-09-07.** The entry now says the assertion in `daemon_dispatch_test.go` is deliberately loose — at least fifteen, not exactly seventeen — because the bidirectional source checks are the real guard |
| ~~**`locales/de.json`'s `Fortsetzen`**~~ | **Closed 2026-09-07.** Replaced with `Notfallmodus`, matching every other panic string in the file |
| ~~**The Docker reconcile reuses `RestoreReasonBoot`**~~ | **Closed 2026-09-07.** A third constant now distinguishes a Docker-triggered restore from a boot restore; neither locale gained a new string, since the reason reaches the detail and not the action |
| **`CmdPanic`'s 35 s deadline** | On expiry `daemonAbsent` is false and the CLI reports the daemon is not answering — but the marker reached the disk in the first millisecond and the teardown lands moments later. A timeout should check the marker and say so. **Reviewed again in 2.15 and deliberately still carried:** what it wants is a timeout that checks the marker and reports what it finds, which is a change to the CLI's contract rather than a fix to an incorrect one. It belongs in a release that is looking at the console tool, not in one that is adding counters |
| ~~**The reconciler polls under panic mode**~~ | **Closed 2026-09-07.** One check at the top skips the poll entirely while panic mode is engaged, so the misleading "putting the rules back" no longer logs immediately before `RestoreCurrent`'s own refusal |
| **`.opencode/opencode.json` is in the history** | Added and removed on the 2.7 branch by a blanket `git add`, so it survives unless the branch is squashed. `.gitignore` records the accident |

## Not carried — decided

Two things were ruled on rather than deferred, and should not be reopened without
a reason:

- **`easywall-core status` exits 2 when no daemon is running, whatever the marker
  says.** A machine with no daemon is not in the state it should be, even when
  deliberately unfiltered: nothing will restore the rules when panic mode ends.
  This was documented wrongly twice before it was documented right.
- **The panic banner has no button.** Ending panic mode from the web interface
  would let the network-facing process re-arm a firewall a human disarmed at the
  console, reachable by a stolen session. Whoever ran `panic` is at that console.
