# Carried forward

What a piece of work found and deliberately did not fix, with enough context to act
on. Every entry was reviewed, triaged and ruled on rather than forgotten; the
reasoning for deferring is part of the entry. Newest first.

**What may not go in here: any defect the release caused, or anything belonging to
the feature it shipped.** A release is complete or it is not finished, and this file
is not a place to put the difference. An entry earns its place by being *found* by
the work rather than *made* by it — and that has to be proven against the base the
branch started from, not against the branch's own head, which already contains the
release's mistakes. 2.15 carried a `check:prose` failure as pre-existing on exactly
that error; comparing against `origin/main` showed its own earlier task had written
the sentence.

## The one exception, added in 2.17

A **contract hole** the release introduced and deliberately did not close is not a
defect it caused, and may be carried. The distinction is what the entry would take
to close:

| | Where it goes |
|---|---|
| **Defect** — the thing is wrong, and a diff makes it right | fixed in the release, never here |
| **Contract hole** — the thing works and its contract is narrower than a reader would assume; closing it is a decision about what the contract *is* | here, under the conditions below |

Three conditions, all of them, or it is a defect being renamed:

1. **A measurement**, showing what the ordinary path actually does today — and
   saying so when the ordinary path is honest by accident rather than by design.
2. **A stated reason** naming the decision closing it would require: a protocol
   change, a new lock, a second independent source of truth. "No time" is not one.
3. **The honest scope written where a reader meets it** — in the code's own
   comment, in `invariants.md`, or in the published documentation, as the case
   needs. A hole nothing outside this file admits to is a hole being hidden.

2.17 wrote this rule because it needed it, and needed it for three entries: it had
`RunPeer` collapsing every dial error into one word, `GET_HEALTH` taking the nft
mutex behind a comment that says it does not block, and a self-test that creates a
fixed range on the host with no collision check. Each works; each has a contract
narrower than its own comment implies. The rule above had no room for that
category, so rather than claim an exception it did not grant — in the file whose
whole subject is not doing that — the rule is amended here, in the open.

Not published — this directory sits outside `docs/`, which is the entire Jekyll
source. See `TestTheTechnicalDocsAreNotPublished`.

# From 2.18

Found while making a second factor mandatory. Nothing here is a defect the
release caused — those were fixed in it.

## Decided in this pass, so it is not rediscovered

- **`/system` gets no screenshot, so ACME has no figure.** The implementation
  plan's Task 14 listed `/system` among the pages to re-shoot. It has never had
  one: `DEFAULT_SCREENSHOT_PAGES` omits it deliberately, and no page under
  `docs/_docs` references such a file. ACME is documented in prose on six pages,
  all of them reference or how-to pages whose other settings carry no figures
  either. Adding one for ACME alone would be the odd one out, so the plan's line
  was an error rather than an instruction. Checked both ways: 18 figure bases are
  referenced, all 18 exist, and no screenshot in the directory is an orphan.

# From 2.17

Found while building the health check and the three proof layers, each proven
against `b723422` — the commit the branch was cut from — and not against the
branch head.

## Tests that pass for a reason outside themselves

| | |
|---|---|
| **`TestIntegration_TheWindowIsOpenWhileTheRulesAreLive` is load-sensitive by construction** | `firewall_integration_test.go` is **byte-identical** to `b723422`. It polls `Status()` with `runtime.Gosched()` and no sleep, watching for a gap its own comment calls "a handful of instructions" wide. Under container CPU contention the scheduler widens it, and the test then truly observes a real but unavoidable transient state. Seen once in roughly 15 runs |
| **Four `TestIntegration_Forward_*` tests skip in a container** | `nftables_forward_test.go` byte-identical to `b723422`; the message is *"the two sides cannot reach each other before any firewall exists"*. A skip whose precondition is absent reads exactly like a pass, which is the class `EASYWALL_REQUIRE_SELFTEST` was added to close for layer C |
| **`CoreClient.GetHealth`, `GetUsage` and `GetAppliedConfig` have no test for a *malformed* reply** | Narrowed from a wider claim after review. `fakeCore` is a real Unix socket listener (`internal/web/testhelpers_test.go:32-51`), so `TestTheDashboardFallsBackWhenTheCoreCannotAnswerAboutHealth` already exercises `GetHealth`'s `!resp.Success` path at wire level. What is uncovered is the reply that parses as a frame and not as the payload. `GetUsage` and `GetAppliedConfig` predate the branch with the same gap; `GetHealth` is new here, so only two of the three are pre-existing |
| **The `auditBuildFindings` call site in `Firewall.apply` is covered by no test** | `Firewall.nft` is a concrete `*NftablesManager`, so `Apply` cannot reach the line without a kernel. Covering it needs a finding to exist during a real apply, which needs a deliberately broken builder — a mutation, not a test. The function itself is fully covered |

## Contract holes this release introduced, carried with a stated reason

Four entries this release is responsible for, kept here under *The one
exception* above — which 2.17 added to the top rule for them, rather than
claiming an allowance the rule did not contain. Each carries its measurement,
its stated reason, and the place a reader meets the honest scope. In every case
the ordinary path measured honest; in the first, honest by accident.

| | |
|---|---|
| **`RunPeer` collapses every dial error into `blocked`** | `netns.go:81-83`. `inboundCrosses` separates ECONNREFUSED from a timeout from anything else, precisely so a harness fault is never reported as a verdict. The peer side does not, so a harness that breaks between claim 1's control and its measurement records `failed` — the one inversion `foldClaims`'s own doc comment forbids. The reachable trigger is a second concurrent `selftest`: `wire()` takes no lock and `deleteLink` is unconditional. **Measured: three runs of two concurrent `RunSelftest()` gave all six `unprovable`** — the setup collision wins the race every time, so today the inversion is prevented by accident and not by design. Closing it means the peer reporting *why* it could not connect, which is a change to the pipe protocol in `netns.go`'s header comment |
| **`GET_HEALTH` is documented as short-deadline because it is read-only, and both its netlink reads take the nft mutex** | `protocol.go:99-110` puts it in `CommandTimeout`'s default branch on the grounds that it queues behind nothing. `RuleCounters` (`nftables.go:416`) and `Enforcing` both take that lock, and `Apply` holds it across `applyCustomRules`' nft subprocess for up to 30 s — which is the reasoning `CmdGetUsage`'s own comment gives for *not* reading netlink. Pre-existing at the core level: `GET_STATUS` already does it at `b723422`. What 2.17 adds is a new consumer — during a slow custom-rules apply `/healthz` answers 503, and the image's `--interval=10s --retries=3` reaches its third failure inside that window. **The cost is a wrong word in `docker ps`**: nothing restarts on unhealthy. Note for whoever picks it up that the health check's own comment claims read-only implies non-blocking |
| **`TestEveryRuleIsAddedThroughTheRecordingAdder` matches a selector, so a local copy of the connection evades it** | The guard walks every non-test source in `internal/core` for a `.conn.AddRule` selector outside `builtRecorder.AddRule`. **Measured: `cn := m.conn` followed by `cn.AddRule(…)` passes it** — there is no `.conn.AddRule` selector left to match, and refusing it needs type resolution rather than syntax, which is a different kind of guard. It is not the shape the finding measured: a copy-paste from pre-`c4dab40` history writes `m.conn.AddRule` and is caught, in any file of the package. `invariants.md` states the scope as the selector rather than as the intent, so nothing claims more than this |
| **`10.77.9.0/24`, `ewst-r` and `ewst-p` are created in the host namespace with no collision check** | The names and the range are fixed constants, and setup deletes any host interface called `ewst-r` — whoever made it — because a per-pid name would collide one step later on `10.77.9.1/24` anyway (`netns.go:256-262`). Nothing checks whether the host already uses either. Documented for operators in `features/health.md` in this release; the absence of a check is carried. **Measured: `easywall-core selftest` against a live host table reports `unprovable` with an accurate detail, never `failed`, 2 of 2 runs** — the ordinary case is honest |

## Measured, and left alone

| | |
|---|---|
| **Two gosec runs, one rule, different answers — and only the one nobody runs locally reaches a human** | `.golangci.yml:38` excludes **G115** (*"int→uint conversions safe for rate-limit and length values"*), so the `Lint` check is green on every integer narrowing in the tree. `security.yml:101` runs **standalone** gosec and uploads SARIF, applying none of golangci-lint's exclusions — so code scanning is the only path on which a G115 ever reaches anybody, on a check-run named `gosec`, after the branch is already pushed. **Measured on this release: `netns.go:382` converted `cmd.Process.Pid` to the `uint32` of `IFLA_NET_NS_PID` with no bound — `-1` becomes `4294967295`, a namespace named by root — and passed `Lint` from the line being written, through a task review, a scoped re-review, a whole-branch review and a push.** Nothing in the repository said the ordinary local path cannot see this class. The line is fixed in this release; the divergence is what is carried. **And the premise three agents were given is wrong: standalone `gosec` runs fine on this host** — `~/go/bin/gosec -include=G115 ./internal/core/...` — even though `golangci-lint` refuses the target outright (*"the Go language version (go1.26) used to build golangci-lint is lower than the targeted Go version (1.27.1)"*), so this class **is** locally checkable and always was. Two more measurements for whoever picks it up: under CI's exact flags (`-tests -tags integration ./...`) gosec reports each line **twice**, which is why one conversion arrived as *"2 new alerts including 2 errors"*; and the 10 G115s now left in the tree are **all** in `_test.go` files, which `security.yml`'s filter step already keeps out of the alert list. *Recommendation, not a decision:* narrow the exclusion to the paths its own comment names, since the tree's other G115s already sit behind explicit bounds with their reasoning beside them (`nftables.go:2389`, `2440`, `totp.go:75`, `118`). That is a lint-config change with a blast radius across every package, so it belongs to whoever takes it up and not to a red-CI fix |
| **`render-changelog.mjs` is both the renderer and the checker, so `check:changelog` cannot see a heading neither of them parses** | `check:changelog` is that script with `--check`, and one version regex decides both what gets written and whether what was written is right. **Measured at `b723422`: `CHANGELOG.md:108` read `## [2.15.1] —## [2.15.1] — 2026-09-07`, the generated page carried 32 `<details>` sections rather than 34, and `check:changelog` reported it current** — so 2.15.1 had no section of its own and no compare link on the site, and its entry read as part of 2.16.0's. The heading is fixed in this release; the sharing is not. Closing it means a checker that does not derive its expectations from the renderer — a second, independent list of versions to compare against — and that is a decision about what `check:changelog` *is*, not a fix to a broken line. `invariants.md` carries the class it belongs to |
| **`podman build` silently drops `HEALTHCHECK`, and the documented compose path builds locally** | OCI is podman's default image format and has no healthcheck field, so a build **exits 0** while producing `HealthCheck: null`. `docker-compose.yml` carries a `build:` section and the documented path is `docker compose up -d`, so a podman operator following the documentation gets no health check and no error. `docker.md` now names `--format docker`, which is a workaround rather than a fix. The two real fixes: publish an image as the documented path, or accept a compose-level `healthcheck:` and give up the single-definition rule. Ready-to-lift text is in `task-11-report.md`, fix round 3 §1 |
| **`ui-check.mjs` does not derive its URL from the config it already reads** | `ui-check.mjs:48` is `process.env.EASYWALL_URL \|\| 'https://127.0.0.1:12227'`. It has an override; what it lacks is deriving the URL from the `web.toml` it already parses for the password hash — so `EASYWALL_DEMO_ADDR` moves the server and `check:ui` keeps driving 12227. **The incident:** Task 14's first run reported *"UI checks passed"* against a pre-existing `easywall-web` on that port, having never loaded the stylesheet it was checking, in the check this repository trusts most. Byte-identical to `b723422` |
| **`check:ui` is not re-runnable against a live demo server** | The ports-catalogue check fails on a second run because run 1's rows are still there. Already carried from the `check:ui` race fix and independently rediscovered here, which is the argument for naming it in `local-review.md`'s traps table beside the rate limiter |
| ~~**All 34 remaining screenshots read `v2.15.1`**~~ | **Closed 2026-09-11.** All 36 re-taken in one run, against `bin/easywall-web` built with `make build VERSION=v2.18.0`, and every chip now reads `v2.18.0`. The entry reasoned that a full re-take was a change of its own; that was true when it was written and stopped being true when `--screenshots` with no arguments grew `takeFullScreenshotSet`. Taking the whole set costs one command and nothing extra, so the 34 were never a separate change — only a separate invocation |
| **`/ports` collapses its aside at every width, and the reasoning is width-blind** | `app.css:790-802` records the decision — the aside is *"worth having, but not worth this table's row width"* — so it collapses unconditionally rather than below 1570px like every other page. From **2.15**, when *Last used* made it a six-column table; `ports.html` is byte-identical to `b723422`. **Measured at 1920px: the five fixed columns need 796px and `Description` absorbs 830px of slack.** A 320px rail would leave `Description` 510px, against a longest real value of *"PostgreSQL — replication peer"*. The budget was genuinely tight at 1570 and is not tight at 1920. Not a bug and not this release's; the measurement is the part worth keeping |

# From 2.16

Found while removing the accent, each proven against `origin/main` before this
branch existed.

| | |
|---|---|
| **`.callout-info` is the last blue on the documentation site** | `web/src/docs.css` hard-codes `rgba(56,189,248,…)` dark and `rgba(2,132,199,…)` light for the info callout — a sky blue wash and edge, no token, and not on the retired list this release swept. It predates the branch. It is now the closest surviving thing to the accent that was just removed, and if *colour means state* is the rule, an **info** callout in blue is the remaining exception. Left alone because the callout set (info / warning / success) is a semantic domain of its own that 2.16's scope did not open |
| **`TestNoRetiredHueSurvives` cannot be satisfied by a comment** | It does a plain `strings.Contains` over the whole file, unlike `TestNoAccentTokenSurvives`, which skips comment lines. So a comment that names the literal it removed — the natural way to record why a colour went — fails the test. Both comments that hit this were reworded to describe the hue instead of spelling it, which loses the grep-ability that made the note useful. The fix is to skip comments in that test too, and it is a change to a guard rather than to the thing guarded |
| **`DESIGN.md`'s `module-active` is modelled as a border, and the code paints a shadow** | The token entry carries `borderColor`; `web/src/app.css` marks an enabled module with `box-shadow: inset 2px 0 0`, the same device as the active nav item. Predates 2.16 — the entry said `borderColor: {colors.accent}` before this release only swapped the token. Which of the two is right is a design decision, and it is the kind that should be made once and written down rather than discovered again |
| **`DESIGN.md` counts the protection toggles twice, differently** | *Why `control-edge` is separate* and *Toggles and checkboxes* both say **eleven**; *Protection modules* says **fourteen**. Predates 2.16 and survived it because the release only touched the colour in those paragraphs. One of the two numbers is wrong and the file argues from both |
| **Prose in `docs/` leaks into the published stylesheet, not just prose in templates** | The existing entry below records this for template comments. It reaches further: `@source "../../docs/**/*.{html,md}"` scans the generated changelog page, so an ordinary English word that happens to be a Tailwind utility is compiled into the site's CSS. Measured on this release — writing *"a ring with no border change"* in the changelog added `.ring`, and 1,650 bytes, to `docs/assets/css/style.css`. The stylesheet is still correct and CI reproduces it exactly; what it is not is minimal, and the cause is invisible in the diff |

# From the check:ui race fix

| | |
|---|---|
| **`checkPortsCatalogue` is not idempotent against a demo server that has already run it** | Picking Pi-hole a second time adds its rows to a set that already holds them, and the check fails with *"picking Pi-hole added 4 TCP rows, expected 2"*. Invisible in CI, which starts a fresh `easywall-web` for every run; it bites only a maintainer whose `scripts/demo-server.sh` has been up across two `npm run check:ui` invocations — where it reads as a real regression and costs a bisect. Reproduced on unmodified `main`, so it predates this branch. The fix is either a check that tolerates the rows already being there or a demo reset the script performs itself, and that is a decision about what `check:ui` may do to a maintainer's running session |

# From 2.15.1

Found while analysing the Discord lockout report, proven against `main` before
the hotfix branch existed, and unrelated to the two defects it fixes.

| | |
|---|---|
| ~~**`docs/installation/docker.md` tells a remote operator to open `https://localhost:12227`**~~ | **Closed 2026-09-09.** The first-run flow is rewritten around a remote host rather than corrected in one sentence: commands on the server, browser on your own machine, `https://<server>:12227`, and the three things to expect on that first page — the certificate warning, nothing filtered yet, and a container that reads `unhealthy` until the first apply. The page also names what is *not* easywall when the page will not load, which is the question a loopback instruction was hiding |

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
| **The spelling gate's scope does not match its configuration** | `.codespellrc` is written repo-wide and `codespell` runs repo-wide, but the only job that runs it triggers on `docs/**`, `.github/workflows/docs.yml`, `.github/actions/**`, `CHANGELOG.md` and `.codespellrc`. A typo in `README.md`, `CONTRIBUTING.md`, `DESIGN.md`, `locales/en.json`, `internal/**/*.go`, `config/`, `debian/`, `docker/` or `systemd/` is never checked. Moving the step to `test.yml` closes it. **Measured 2026-09-09, so the move is safe rather than merely plausible:** `codespell` run from the repository root with no paths — the whole tree, config honoured — produces **zero** findings and exits 0. Trap: passing explicit paths on the command line bypasses the skip list's `./`-prefixed entries, which is what makes the repo-wide run the cheaper one as well as the correct one |
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
| ~~**A forgotten panic mode is invisible to monitoring**~~ | **Closed 2026-09-09.** `easywall-core health` exits **2** under panic where `status` still exits 0, and `/healthz` answers 503. The two commands are allowed to disagree: a console asking after *intent* is right to be quiet, a monitoring system asking after *health* is asking something else, and `TestHealthAndStatusDisagreeUnderPanic` pins it so the divergence stays a decision. One correction to the release's own spec: health reads `fail` with reason `panic`, **not** `degraded`. `Panic` calls `nft.Reset()`, which leaves the input chain empty, so `Enforcing()` is false and `degraded` would have understated a machine that is not filtering at all |
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
