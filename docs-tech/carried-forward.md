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

**Emptied 2026-09-11.** Forty-three open entries were ruled on rather than
carried again: the ones a diff closes were fixed, the ones a decision was
blocking got the decision, and the six nothing can fix were moved to where a
reader meets the limit — a guard's own comment, or `invariants.md` — because
an entry nobody can act on does not belong in a file whose header says an
entry carries enough context to act on. Three of the entries' own premises
did not survive measurement and are corrected above rather than quietly
closed. Two proposed contract changes were declined with a measurement, which
is also a ruling. **Forty-three closed, one opened:** the sweep itself found
that `check:ui`'s layout/overflow sweep runs in English only, and a
pre-existing overflow had gone unseen because of it — found by this work
rather than made by it, which is exactly what the entry below is for.
Pretending it does not
exist to leave the file at zero would be the dishonesty this sweep exists to
end. See `docs-tech/specs/2026-09-11-the-carried-forward-sweep.md`.

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

# From 2.19 — what it passes on, it also filters

| | |
|---|---|
| **On /ports between roughly a 1226px and a 1423px viewport, the description column is narrower than its longest value, and no width setting closes it** | Caused by 2.19's Scope column, and carried under an explicit ruling rather than the usual rule, because closing it is a layout change with its own render budget and bolting it onto a fix round is how this release's 1440px regression happened in the first place. **Measured**, on the running demo with the column present: the seven columns' measured floors sum to ~1147px of `.table-wrap` — port 122, SSH 94, scope 156, sources 341, description 243, last-used 131, actions 56 — and a 1300px viewport gives the table 1014px. So it is not a width that can be moved from one column to another: at 1440 (`.table-wrap` 1154) the description clears its longest *rendered* value by 17px and the catalogue's longest, `PostgreSQL — replication peer` at 193px, still misses by 29. **Closing it needs** /ports to reflow to cards at a second, page-specific container threshold: the shared `@container (max-width: 940px)` cannot move, because at 1205 the forwarding row check fails — `check:ui` drives /forwarding at Playwright's default 1280px viewport, where `.table-wrap` is 994px and four columns become cards for no reason. A page-specific threshold means a named container and a second copy of the ~60-line reflow block. **Worth knowing on its own:** `check:ui` sweeps 1600, 1440, 1200, 900 and 390, so it steps over this band entirely — 1440 passes by 17px and 1200 is already cards. The stylesheet says all of this at the container query itself, which is where a reader meets it |
| **`detectDockerBridges` returns IPv4 CIDRs only, so nothing easywall writes into the forward chain ever matches an IPv6 packet** | Found while planning 2.19, not caused by it, and proven not caused by it: the *existing* bridge exception never matched IPv6 either, so inbound IPv6 to a published container port fell to the forward chain's `policy drop` before this release and falls to it after. The new default-deny opens no hole that today does not already have, and `published_ports` changes nothing about it — `docker.custom_networks` is the documented way through, on both sides of the release. **Closing it is a change to what crosses the forward chain on every host that runs `docker.enabled = true`**, IPv6 bridge or not, which is why it is not bolted onto a release whose own deny needs the acceptance round: teaching detection to return IPv6 CIDRs would start accepting traffic that is dropped today, on hosts nobody has measured. Spec §5 of `docs-tech/specs/2026-09-12-2.19-what-it-passes-on-it-also-filters.md` is the write-up; it wants its own piece of work and its own integration test against a real IPv6-enabled bridge |

# From the carried-forward sweep

Found while shortening an audit label this sweep already had open (below);
neither caused by this work nor closable in it.

| | |
|---|---|
| ~~**`check:ui`'s layout/overflow sweep runs in English only, so a German-only overflow has no path to failing red**~~ | **Closed 2026-09-12 by 2.19's Scope column**, which needed it: the column's German option truncated in a rendered select while every gate stayed green, twice. `checkGermanFits` now drives the same 13 pages at the same five widths with `easywall_lang=de`, running the input, select and container overflow checks (not the console/request health checks, which do not vary with language). The 21px this entry measured is fixed too: `.log-action` is a `white-space: nowrap` chip, so *"Netzwerkeinstellungen gespeichert"* could not wrap and made the table 382px wide inside a 360px wrap — `.table-reflow .log-action { white-space: normal }` in the card block, measured 360 in 360 after. Proven against the pre-branch base by serving `f722ed5`'s own `web/static/style.css` to the same page: identical 382>360, so this was found by the new coverage rather than caused by the release. Original entry: Found while rendering the fix for `audit_boot_enforce_failed`'s own German label (below). Independent of it: `audit_settings_saved`'s existing, untouched German translation, *"Netzwerkeinstellungen gespeichert"*, alone measures **381px against a 360px `.table-wrap` at 390px — 21px over** — rendered against the real DOM, both themes, 2026-09-11. **The string is not the finding.** `npm run check:ui` drives 13 pages × 5 widths × 2 themes for the layout/overflow sweep, and that sweep never switches the interface to German (a separate check does exercise `de`, but only its own POST/cookie behaviour, not the width sweep) — so a German-only overflow, this one or any other still sitting behind it, has no path to failing red. That is why a 21px overflow in a label nobody touched survived every release since it was translated. Closing it needs the layout sweep itself to run in German too, which is new scope this sweep did not have |

## The mutations that stayed green

A 75-mutation audit of this release's guards: 56 red, 19 green. No shipping
defect — every green one is a place where the code can be broken without a test
noticing. Six were closed before the release; the rest are here with their
measurement, which is the part that saves the next person the work.

| | |
|---|---|
| ~~**The ACME serving path has no unit coverage at all**~~ | **Closed 2026-09-11.** `tlscert_test.go` now builds a `certManager` with ACME on and asserts `m.acme != nil && m.sslDir == ""`, and that a `hello` for the wrong SNI returns autocert's host-policy error rather than a self-signed certificate. Both mutations the entry named — deleting `m.sslDir = ""`, and `GetCertificate` falling through to the self-signed path — go red without the `integration` build tag |
| ~~**No end-to-end passkey ceremony ever runs with a port in the origin**~~ | **Closed 2026-09-11.** A `withBindAddr(":12227")` option on `newPasskeyTestServer`, used by one ceremony test; the fixture's `127.0.0.1:443` default stays, because that is how an ordinary HTTPS installation reads |
| ~~**`TestTheRuleIsStatedOnce` catches the identifier, not the rule**~~ | **Closed 2026-09-11.** The guard now refuses any integer literal compared against a password length outside `auth.go`, not only the constant's name — `len(pw) < 12` written as a second copy of the floor fails it, same as `len(pw) < minPasswordLen` |
| ~~**`passkeys.json`'s file mode is unpinned**~~ | **Closed 2026-09-11.** A test asserts `0600` after `passkeystore.go`'s atomic write |
| ~~**The `v3:` domain separator is not pinned**~~ | **Closed 2026-09-11.** A golden digest for `credentialFingerprint` over a fixed input goes red if the separator reverts to `v2:` |
| ~~**`Prompt` could return false**, and **`newACMEManager`'s hostname re-check is unguarded**~~ | **Closed 2026-09-11.** Two unit tests need no kernel: `Prompt` returns true, and `newACMEManager` refuses an empty hostname. Both were caught only by the integration test that cannot run without `CAP_SYS_ADMIN` |
| ~~**`JustGated` is unobservable when no codes are minted**~~ | **Closed 2026-09-11.** `password.html` no longer nests `{{if .JustGated}}` inside `{{if .Codes}}` — the way-onward link is a sibling of the recovery-codes card — and an HTTP-level test sets the flag without minting codes |

Four more mutations were green and **behaviourally inert** — the mutation cannot
change what the program does. Recorded so nobody re-runs them: the corrupt-store
early return (a failed `json.Unmarshal` leaves the zero value anyway), the
port-80 state computed without ACME (the template's own `{{if .ACMEEnabled}}` is
the real guard, proven by its own red mutation), and the two renewal-loop
short-circuits, which both hinge on `m.sslDir == ""`.

## Found in `invariants.md` itself, and older than this branch

Noticed while adding this release's guards. Both are proven against `fbac69f`;
the third was a plain error and is fixed rather than carried.

| | |
|---|---|
| ~~**Six rows lose their incident text to the renderer**~~ | **Closed 2026-09-11.** Folded into the *Protects* cell rather than made a third column: three columns would have meant inventing six incident stories for the rows above them that have none, which is the opposite of what this file is for. The six entries now carry their incident in the rendered view, not only in the source |
| ~~**`TestRulesIsEmptyCountsEveryField` is listed twice**~~ | **Closed 2026-09-11, and it was never a copy.** `:142` reasoned *"a seventh rule set added and forgotten"*; `:166` reasoned *"it decides whether a host counts as configured"* — two claims about one guard. Merged into `:142`, which now carries both; `:166` deleted |

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
| ~~**Four `TestIntegration_Forward_*` tests skip in a container**~~ | **Closed 2026-09-11.** `EASYWALL_REQUIRE_SELFTEST` makes the skip a failure where the precondition is supposed to hold — reused rather than given a variable of its own, so its scope now exceeds its name: it gates the four forward tests as well as the self-test provers it was named for. CI cannot report a pass for four tests that never ran |
| ~~**`CoreClient.GetHealth`, `GetUsage` and `GetAppliedConfig` have no test for a *malformed* reply**~~ | **Closed 2026-09-11.** Three tests through `fakeCore`, a real Unix socket listener, cover the reply that parses as a frame and not as the payload — the half `!resp.Success` did not reach |

## Contract holes this release introduced, carried with a stated reason

Four entries this release is responsible for, kept here under *The one
exception* above — which 2.17 added to the top rule for them, rather than
claiming an allowance the rule did not contain. Each carries its measurement,
its stated reason, and the place a reader meets the honest scope. In every case
the ordinary path measured honest; in the first, honest by accident.

| | |
|---|---|
| ~~**`RunPeer` collapses every dial error into `blocked`**~~ | **Closed 2026-09-11, and the contract changed.** The peer now reports *why* it could not dial — `peerVerdict` mirrors `inboundCrosses`' classification, with a fourth pipe-protocol word, `failed <reason>`, so a harness fault can never again be read as a verdict. The measurement that made this affordable stands: three runs of two concurrent `RunSelftest()` gave all six claims `unprovable` before the fix, so the inversion this closes was until now prevented by a setup collision winning the race, not by design |
| ~~**`GET_HEALTH` is documented as short-deadline because it is read-only, and both its netlink reads take the nft mutex**~~ | **Closed 2026-09-11 by correcting three comments rather than the deadline.** The spec proposed lengthening it; two measurements declined that. `Dockerfile:156` is `--timeout=5s`, so docker abandons the probe at five seconds whatever the core's deadline says — a 35 s deadline would change nothing for the only documented consumer and make every other caller wait. And `TestCommandTimeoutKeepsGetHealthShort` pinned the 5 s deliberately, **on a false premise**: its comment claimed the command does not queue behind the nft mutex, while `Enforcing` (`nftables.go:347`) and `RuleCounters` (`:416`) both take `m.mu`. So one of the three comments was a guard passing for the wrong reason. The deadline stands; the honest scope — 503 during a slow apply, docker's third retry inside the window, nothing restarting on unhealthy — is now in the command's comment, in the guard's comment and in `features/health.md` |
| ~~**`10.77.9.0/24`, `ewst-r` and `ewst-p` are created in the host namespace with no collision check**~~ | **Closed 2026-09-11, and the contract changed.** `harnessCollision()` now checks the range and the peer name before `wire()` deletes anything, reporting `unprovable` with the collision named rather than a broken harness reading as `failed`. `ewst-r`'s own unconditional deletion is untouched — it is documented, reasoned, and the case the check would break. Measured before the fix: a live host table already reported `unprovable` with an accurate detail, never `failed`, 2 of 2 runs — this hardens an honest path rather than fixing a dishonest one |

## Measured, and left alone

| | |
|---|---|
| ~~**Two gosec runs, one rule, different answers — and only the one nobody runs locally reaches a human**~~ | **Closed 2026-09-11.** The `G115` exclusion is deleted from `.golangci.yml`. Measured before removing it: `gosec -include=G115 ./...` found zero production issues, and the four sites its comment defended already carry `#nosec G115` with their bound written beside them. Under CI's exact flags (`-tests -tags integration`) it finds 10 more, at six distinct sites, every one in a `_test.go` file — those six now carry `#nosec` with a reason. The blast radius the entry feared was six lines in two test files |
| ~~**`render-changelog.mjs` is both the renderer and the checker, so `check:changelog` cannot see a heading neither of them parses**~~ | **Closed 2026-09-11, and the contract changed.** `check:changelog` now has a second, independent parser that does not share the renderer's regex. Proven against the real 2.15.1 defect rather than argued from design: mutating `CHANGELOG.md` and then running `npm run build:changelog` before checking, the old checker exits 0 reporting *"current — 34 versions"* while the new checker exits 1 on 36 headings against 34 sections — the renderer folds the doubled heading into 34 self-consistent sections and passes its own check, exactly as it did in 2.15.1 |
| ~~**`podman build` silently drops `HEALTHCHECK`, and the documented compose path builds locally**~~ | **Closed 2026-09-11, and the contract changed.** `docker-compose.yml` now carries its own `healthcheck:` block rather than relying on the image's `HEALTHCHECK`, and a test keeps the two definitions from drifting. Verified against real `podman`: `podman build` reproduced the OCI healthcheck drop (`HealthCheck: null`), and `podman compose up -d` with the new block reached `(healthy)` |
| ~~**`ui-check.mjs` does not derive its URL from the config it already reads**~~ | **Closed 2026-09-11.** The script now derives its URL from the `web.toml` it already parses, and prints the URL and the config it chose. `EASYWALL_URL` still wins, the literal is still the last resort — the incident this closes was a run reporting *"UI checks passed"* against a stale `easywall-web`, having never loaded the stylesheet it was checking |
| ~~**`check:ui` is not re-runnable against a live demo server**~~ | **Closed 2026-09-11 — one finding, recorded twice: this row and the one below, independently rediscovered.** `checkPortsCatalogue` now tolerates rows a previous run already added rather than requiring a clean server, which was the chosen direction: a check that reset a maintainer's running session was judged the larger surprise |
| ~~**All 34 remaining screenshots read `v2.15.1`**~~ | **Closed 2026-09-11.** All 36 re-taken in one run, against `bin/easywall-web` built with `make build VERSION=v2.18.0`, and every chip now reads `v2.18.0`. The entry reasoned that a full re-take was a change of its own; that was true when it was written and stopped being true when `--screenshots` with no arguments grew `takeFullScreenshotSet`. Taking the whole set costs one command and nothing extra, so the 34 were never a separate change — only a separate invocation |
| ~~**`/ports` collapses its aside at every width, and the reasoning is width-blind**~~ | **Closed 2026-09-12, at a rendered threshold rather than a summed one.** The declared column widths never added up to the carried "796px" — `table-layout: auto` redistributes them, so the five fixed columns render at 780px today, not 796, and no sum on paper was ever going to reproduce that. Driving the grid's own column split at each viewport found the real handoff at exactly **1650px**: the description field needs 205px for *"PostgreSQL — replication peer"*, has 206 at 1650 and 196 at 1640. `max-width: 1650px` is inclusive, so the aside collapses at and below 1650 and returns from 1651, where the field has 207px — the threshold that was derived without rendering (also 1650) turned out right, and could not have been known to be |

# From 2.16

Found while removing the accent, each proven against `origin/main` before this
branch existed.

| | |
|---|---|
| ~~**`.callout-info` is the last blue on the documentation site**~~ | **Closed 2026-09-11, and the premise was half wrong.** All three callouts hard-code their wash and edge — info `rgba(56,189,248,…)`, warning `rgba(245,158,11,…)`, success `rgba(16,185,129,…)` — and all three already tokenised their *text* colour. There was no info-only exception. The blue stays: *info* is a state, and *colour means state* does not forbid a state from having one. Six literals became six tokens, and the two theme-scoped overrides went away |
| ~~**`TestNoRetiredHueSurvives` cannot be satisfied by a comment**~~ | **Closed 2026-09-11.** The guard now skips comment lines, matching `TestNoAccentTokenSurvives`; the two comments that had been reworded to evade it were restored to naming the literal they removed |
| ~~**`DESIGN.md`'s `module-active` is modelled as a border, and the code paints a shadow**~~ | **Closed 2026-09-12.** `DESIGN.md` now models it as `edgeShadow`, the same device as the active nav item — the code was right, and consistency with a shipped pattern beat a token entry nobody had implemented |
| ~~**`DESIGN.md` counts the protection toggles twice, differently**~~ | **Closed 2026-09-12, and both existing numbers were wrong.** Counted from the configuration struct directly rather than trusting either paragraph: **fourteen** protection-module toggles. *Why `control-edge` is separate* and *Toggles and checkboxes* are corrected to match; *Protection modules* already said fourteen and was left alone |
| ~~**Prose in `docs/` leaks into the published stylesheet, not just prose in templates**~~ | **Closed 2026-09-12, and measured rather than reasoned about.** `@source not` excludes the generated changelog page from the Tailwind scan, dropping the one class it had leaked (`.ring`, `1 → 0`). Tailwind 4.3.3 does **not** skip HTML comments, so the template half stays a documented constraint, not a fix — see the paired entry below |

# From the check:ui race fix

| | |
|---|---|
| ~~**`checkPortsCatalogue` is not idempotent against a demo server that has already run it**~~ | **Closed 2026-09-11 — the same finding as `check:ui is not re-runnable`, above, independently rediscovered under a different name.** Both rows close on the one fix: the check now selects rows by port after measuring what the catalogue actually appended, rather than assuming an empty starting set |

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
| ~~**`security.md` overflows at 390px**~~ | **Closed 2026-09-12, and the entry closes on the rule, not on the page.** The rejected fix was rejected for a property it does not have: `overflow-wrap: break-word` breaks a word only when it cannot fit a line by itself — true of `anywhere` and `break-all`, not of this one — so `proxy_set_header` and every other ordinary identifier on the site are untouched. The 56-character Go test name that caused the original 482px measurement has since moved to the generated changelog page (Task 21's fix round), so `security.md` **no longer overflows at all**: measured 2026-09-12, scrollWidth 382 against a 390px viewport, both themes, and 892/900, 1592/1600 likewise. The changelog page, which now holds the identifier, also does not overflow (387/892/1592), the 35-link version list wrapping rather than scrolling. `.content-body code` carries the rule generally, which is what makes it true of whichever page holds the next long identifier |
| ~~**The rouge theme leaves nine emitted token classes unstyled**~~ | **Closed 2026-09-11.** `.p`, `.nt`, `.nl`, `.na`, `.kc`, `.no`, `.se`, `.si` and `.sh` are styled; a TOML or YAML key in the configuration reference no longer renders in body-text colour beside a coloured value. The `$ easywall-core …` blocks stay unlabelled `console`, deliberately — the copy button would take the prompt with it |
| ~~**The changelog page has no on-page contents**~~ | **Closed 2026-09-11.** The page gets its own version list, built by the renderer that already knows every version, rather than loosening the `heads.length < 3` rule that is correct on every other page |
| ~~**The application sidebar still has the weakness the docs sidebar lost**~~ | **Closed 2026-09-08.** Carried across three design reviews, and the entry 2.16 existed to close. The divider and the indent are sibling-scoped `:has()` selectors, so the first labelled group correctly gets neither — it follows the ungrouped *Dashboard* link, not another group — and a fourth group added to `base.html` later gets both without anyone remembering a class |

## Repository hygiene

| | |
|---|---|
| ~~**`main` has no required status checks**~~ | **Closed 2026-09-01.** Thirteen checks are now required and enforced for administrators; see *What protects `main`* in [ci-and-release](ci-and-release.md). Two things are worth recording. It did **not** close the case that prompted it — the pull request run was green and the `main` run of the same tree was red, so no merge gate would have seen it. And the release commit now needs a pull request, because a direct push carries no passing checks. `check:docs` stays duplicated in the docs deploy job: `docs.yml` is path-filtered, so its checks cannot be required without blocking every pull request that touches no `docs/**` file |
| ~~**The spelling gate's scope does not match its configuration**~~ | **Closed 2026-09-11.** `codespell` moved from `docs.yml`, which is path-filtered, to `test.yml`, run repo-wide with no paths — measured, again, from the repository root before the move: zero findings, exit 0. A guard now asserts both halves: `test.yml` runs it unfiltered, `docs.yml` does not, and passing an explicit path (which would bypass the skip list's `./`-prefixed entries) is refused |
| ~~**The mark has two homes**~~ | **Closed 2026-09-11.** A guard test asserts `web/static/icon.svg` and `docs/assets/img/icon.svg` stay byte-identical, so the two files cannot drift apart without being noticed — without choosing a build step to unify them |
| ~~**Prose in a template leaks into the stylesheet**~~ | **Closed 2026-09-12, as a documented constraint rather than a fix.** Measured with a probe file: Tailwind 4.3.3 does not skip HTML comments, so a class-like word in one is still scanned. `CONTRIBUTING.md` now says so, beside the paired entry above, whose generated-page half `@source not` **did** close |
| ~~**`npm run build:diagrams` is not byte-reproducible**~~ | **Closed 2026-09-12.** `CLAUDE.md`'s generated-file rule now names its one exception: the two stylesheets and the changelog page are rebuilt and diffed, the diagrams are checked with `npm run check:diagrams`'s `data-source-digest` instead, because Mermaid jitters bezier control points and a diff would prove nothing. Confirmed non-reproducible on two clean rebuilds; `check:diagrams` exits 0 on both |

## Not carried — decided in this pass

- **The whitelist page ships without a screenshot.** Reusing `blacklist-*.png` with reworded alt text was the plan's instruction; another page's screenshot relabelled is a false statement in the documentation, and a real one needs demo mode running, which nothing in the plan sets up. An absence beats a wrong picture. A genuine `whitelist-*.png` is follow-up work for a branch that starts the application.
- **`docs/_docs/changelog.md` is excluded from the prose corpus.** It is generated from `CHANGELOG.md`, its bullets may not be edited, and it carries 214 sentences over 30 words — a checker that walked it could never pass. Excluded by exact path rather than a glob, so a future page cannot be swallowed silently.
- **`MAX = 30` and `AVG = 18` in `scripts/prose-check.mjs` were not softened** when the corpus looked far from them. It reached 0 over 30 and an average of 14.1 words without the target moving.

# From 2.7

## Operator-visible

| | |
|---|---|
| ~~**`boot_enforce_failed` reads "at startup"**~~ | **Closed 2026-09-11.** Reworded to *"Rules could not be restored"* in both locales and in `features/audit-log.md`'s *Reads as* column — true of all six write sites, not only boot: `restore.go:72/133/145/183` are reachable from resume and a Docker bridge appearing, not only startup. The German label needed a second, shorter pass to stop overflowing its column (measured against the real DOM: 155px over at the original string, 91px after the first reword, 22px after the second, `"Regeln nicht wiederhergestellt"`) |
| ~~**`rollback_skipped`'s label**~~ | **Closed 2026-09-11.** Reworded to *"Stored, not written — panic"* (en, measured `over: 0` standalone) and *"Gespeichert, nicht geschrieben — Notfallmodus"* (de); both write sites end with the stored rules reverted and only the kernel write undone, which the label now says instead of contradicting |
| ~~**A forgotten panic mode is invisible to monitoring**~~ | **Closed 2026-09-09.** `easywall-core health` exits **2** under panic where `status` still exits 0, and `/healthz` answers 503. The two commands are allowed to disagree: a console asking after *intent* is right to be quiet, a monitoring system asking after *health* is asking something else, and `TestHealthAndStatusDisagreeUnderPanic` pins it so the divergence stays a decision. One correction to the release's own spec: health reads `fail` with reason `panic`, **not** `degraded`. `Panic` calls `nft.Reset()`, which leaves the input chain empty, so `Enforcing()` is false and `degraded` would have understated a machine that is not filtering at all |
| ~~**Every page pays a `GET_STATUS`**~~ | **Closed 2026-09-11, as bookkeeping.** The entry's own text already said *Shipped in 2.14* as `Server.statusForRender`; it was never struck through. Struck now — nothing here was open |

## Invisible failures

| | |
|---|---|
| ~~**Four audit-silent paths in `apply`**~~ | **Closed 2026-09-07.** The second `GetState` (the re-read after promote), `BackupCurrent`, `PromoteStaged` and `acceptance.Start` now all write an audit entry before returning; see `TestEveryFailurePathInApplyIsAudited` in [invariants](invariants.md) |
| ~~**`acceptance.Start`'s error path**~~ | **Closed 2026-09-07.** This entry described 2.7's ordering. Since 2.14 the window opens before the kernel write, so what the error path left behind was not live rules but a stored `Current` holding an unconfirmed set — which the next boot or `resume` would install with no window at all. It now rolls back rather than return |

## Guards that do not see enough

| | |
|---|---|
| ~~**The kernel-write guard is scoped to two files**~~ | **Closed 2026-09-07.** `daemon_source_order_test.go` no longer enumerates `firewall.go` and `restore.go`; `coreSources` now globs every non-test source file in the package with `filepath.Glob`, so a fourth writer of the table added in a third file cannot be added invisibly |
| ~~**`bootBridges` has no test**~~ | **Closed 2026-09-07.** `TestRestoreCurrent_RecordsTheBridgesItBakedIn` writes through `RestoreCurrent` rather than the field directly, so deleting `setBootBridges` from it fails the suite |

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
| ~~**`CmdPanic`'s 35 s deadline**~~ | **Closed 2026-09-11, and the contract changed.** On expiry the CLI now reads the panic marker and reports what it finds — recorded and engaged, recorded and not, or unreadable — rather than only "the daemon is not answering", which was true of the socket and false about the marker that had already reached disk in the first millisecond. This is the release that was looking at the console tool |
| ~~**The reconciler polls under panic mode**~~ | **Closed 2026-09-07.** One check at the top skips the poll entirely while panic mode is engaged, so the misleading "putting the rules back" no longer logs immediately before `RestoreCurrent`'s own refusal |

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
