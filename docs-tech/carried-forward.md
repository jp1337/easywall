# Carried forward

What a piece of work found and deliberately did not fix, with enough context to act
on. Every entry was reviewed, triaged and ruled on rather than forgotten; the
reasoning for deferring is part of the entry. Newest first.

Not published — this directory sits outside `docs/`, which is the entire Jekyll
source. See `TestTheTechnicalDocsAreNotPublished`.

# From 2.15

Two layout defects found while verifying the *Last used* column, both proven
independent of it, both belonging to 2.16 — *The interface looks like a
firewall* — where the tables are the subject rather than a passenger.

## The ports table is too narrow for its own placeholder

| | |
|---|---|
| **`.col-port` is 120px and `8000:9000` needs 122px** | `web/src/app.css:1611`. The column is 2px short of the widest port value the field's own placeholder documents, so a range clips. Real user data, not a contrived string, and it predates this release: the *Last used* column did not cause it and removing that column does not fix it. Not widened here because the widths on that table are a set — `.col-port` gaining 8px has to come out of another column, and which one is a design decision `DESIGN.md` does not currently answer |
| **`.table-wrap` overflows horizontally by 10px in card mode** | `web/src/app.css:1570`, at every width where the container query switches the table to cards. Measured identical with six, five and four columns, and unchanged with `white-space: nowrap` removed, so it is neither the new column nor a wrapping problem: something inside the card layout is 10px wider than the container it sits in. `npm run check:ui`'s overflow check does not fire on it, which is worth understanding before fixing it — a fix that only silences the symptom would leave the check still blind |

# From the 2026-08-30 documentation-site polish

Twenty-four planned tasks, five defects found while verifying them, and two features
added mid-run. What follows is what was seen and left alone, and why.

## Design decisions nobody has made yet

| | |
|---|---|
| **`security.md` overflows at 390px** | 482px against a 390px viewport, in both themes, caused by one `<code>` holding the Go test name `TestLoginVerify_TheSixteenthCodeAttemptDoesNotGetThrough`: it renders 462px wide and cannot wrap. Present before the branch. The obvious fix — `overflow-wrap` on `.content-body code` — would break **every** long identifier on the site at an arbitrary point, and a mid-token `proxy_set_header` may serve a firewall's reader worse than one page that scrolls sideways. `DESIGN.md` is silent on it |
| **The rouge theme leaves nine emitted token classes unstyled** | `.p` (162 occurrences), `.nt` (68 — nginx directives, YAML tags), `.nl` (58 — TOML and YAML **keys**), plus `.na`, `.kc`, `.no`, `.se`, `.si`, `.sh`. In the configuration reference a key therefore renders in body-text colour while its value is coloured. Highlighting is otherwise used correctly: 52 fences carry a language and the 11 that do not are output, paths and a CSP header, rightly plain. Note that labelling the `$ easywall-core …` blocks `console` would be a regression — the copy button would take the `$` prompt with it |
| **The changelog page has no on-page contents** | `docs/_layouts/default.html` filters headings inside `<details>`, all 30 version headings are, and the `heads.length < 3` rule then drops the column. Correct by that rule's own logic, and the longest page on the site is the one with no jump list |
| **The application sidebar still has the weakness the docs sidebar lost** | `web/src/app.css`'s `.nav-section-label` carries the identical label/link colour pair — twelve steps per channel — with no divider and no indent between its Dashboard, Rules and System groups. The docs site fixed it with mono type, a rule and indented items, and `DESIGN.md` was amended to describe both devices. The application was deliberately not touched |

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
