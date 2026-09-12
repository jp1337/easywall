---
title: "The carried-forward sweep"
date: 2026-09-11
status: approved, not yet planned
---

# The carried-forward sweep

`docs-tech/carried-forward.md` holds what eight pieces of work found and
deliberately did not fix. Every entry was triaged rather than forgotten, and the
file's own rule is that an entry earns its place by being *found* by a release
rather than *made* by it. That rule has held. What has not held is the exit: over
eight pieces of work, 18 of 64 entries were closed and 46 were not, and the oldest open
one is from 2.7.

This piece of work empties the file. Not by relaxing its rule — by ruling on
every open entry, closing what a diff closes, and moving the entries that no
diff can close to where a reader actually meets them.

Agreed with the maintainer on 2026-09-11:

- **All four buckets, including D** — the entries nothing can fix close as
  *rulings*, recorded where a reader meets them, not as a to-do list that
  outlives the file.
- **Unversioned on `main`.** A sweep branch, merged without a tag. It is
  preparation for 2.19, not 2.19, and not a patch release either: three of the
  Bucket C items change a contract, which a patch version would misdescribe.
- **The ten Bucket B rulings were taken up front**, in one round, so the plan
  has no decision stops and the tasks can overlap.

## 0 · What is in the file, measured rather than read

| | |
|---|---|
| Table entries | **64** |
| Struck through (closed) | **18** |
| Open | **46** |
| Of those, not work items | **2** — the *Defect* / *Contract hole* rows are the rule's own explanation table |
| Already shipped, never struck | **1** — *Every page pays a `GET_STATUS`* says *Shipped in 2.14* in its own text and is not struck through |
| The same finding twice | **1 pair** — `check:ui` idempotency appears under *Measured, and left alone* (2.17) and again under *From the check:ui race fix* |
| **Distinct work items** | **43** |

Six entries were spot-checked for staleness before planning any of this, because
an entry written five releases ago may have been closed by accident since. None
had:

| Checked | Result |
|---|---|
| `codespell` moved out of `docs.yml`? | No. `.github/workflows/docs.yml` is still the only job that runs it |
| The two `icon.svg` diverged? | No. `cmp web/static/icon.svg docs/assets/img/icon.svg` — byte-identical |
| `.callout-info`'s blue retired? | No. `web/src/docs.css:1113-1125` |
| `.opencode/opencode.json` gone from history? | No. Two commits still carry it |
| `TestRulesIsEmptyCountsEveryField` de-duplicated? | No. `invariants.md:142` and `:166` |
| The six three-cell rows in `invariants.md`? | Still there. Header at `:106` declares two columns; rows `:108-123` have two cells, rows `:124-129` have three |

## 1 · The four buckets, and what *done* means in each

The 43 are not one kind of work, and a plan that treats them as one will stall
on the first item that needs a decision instead of a diff.

| Bucket | Items | Closing an entry means | Proof it owes |
|---|---|---|---|
| **A** — mechanical | 19 | A diff. No question outstanding; several entries name the closing recipe themselves | The test goes red when the implementation is mutated, per this repository's rule that a test is verified by breaking the code |
| **B** — a ruling, then a small diff | 11 | The ruling was taken 2026-09-11 (§2). The diff follows from it | Rendered verification for the five that touch visible surface; a mutation for the two that touch guards |
| **C** — a contract change | 7 | A decision about what the contract *is* — a protocol field, a deadline class, a second source of truth. Each is proposed in §5 with its cost | The proposal is accepted or declined at spec review; execution proves the new contract, not the old one |
| **D** — nothing can fix it | 6 | The entry closes by **moving**: into the guard's own comment, into `invariants.md`, or into the published documentation, with the reason it stays open. A limit nothing outside `carried-forward.md` admits to is a limit being hidden | The text exists in the named file, and `carried-forward.md` no longer carries the row |

Bucket D is the one that needs stating plainly, because it looks like
bookkeeping and is not. `carried-forward.md` is a *staging* document: its own
header says an entry carries enough context to act on. An entry nobody can act
on does not belong there — it belongs beside the thing it limits. Six entries
have sat in a to-do file since 2.7 while being, in fact, decided.

## 2 · The twelve rulings, taken 2026-09-11

Eleven of them are Bucket B and carry their own diff. The twelfth governs a
Bucket D entry: `.opencode/opencode.json` is ruled on and then moved, not fixed.

| # | Entry | Ruling |
|---|---|---|
| 1 | `invariants.md` table at `:106` mixes two- and three-cell rows | **Fold.** The six incident texts move into the *Protects* cell. Three columns would mean inventing sixteen incident stories for the rows that have none — the opposite of what this file is for |
| 2 | `TestRulesIsEmptyCountsEveryField` listed twice | **Keep `:142`** (*The rules say what they mean* — the guard's structural home), **delete `:166`**, and merge `:166`'s reasoning into `:142`'s cell so the colour-relevant half is not lost |
| 3 | `/ports` collapses its aside width-blind | **The same rule as every other page, at a higher threshold** — determined by rendering, not by adding pixels on paper. The carried measurement already says there is room at 1920px |
| 4 | `.callout-info`'s blue | **The blue stays** — *info* is a state, and *colour means state* does not forbid states from having colours. Instead the three washes become named callout tokens, and the entry's premise is corrected (§3) |
| 5 | `DESIGN.md`'s `module-active`: border vs shadow | **The code is right; `DESIGN.md` is amended.** The inset shadow is the same device as the active nav item, and consistency with a shipped pattern beats a token entry nobody implemented |
| 6 | `DESIGN.md` says both eleven and fourteen toggles | **Distrust both numbers.** `options.html` carries 22 checkboxes, many of them `*_log` / `*_limit` companions rather than modules. The count is taken from the configuration struct, then the wrong paragraph is corrected — not the other one bent to match it |
| 7 | `security.md` overflows at 390px | **`overflow-wrap: break-word` on inline `code`**, never on `pre`, verified in the render against both strings before it stays (§3) |
| 8 | The rouge theme leaves nine token classes unstyled | **Style them**, `.nl` / `.nt` / `.p` at minimum: a TOML key in body-text colour beside a coloured value is the configuration reference being wrong. Labelling the `$ easywall-core …` blocks `console` stays forbidden — the copy button would take the prompt with it |
| 9 | The changelog page has no on-page contents | **Its own version list**, built by the renderer that already knows every version — rather than loosening `heads.length < 3`, which is correct on every other page |
| 10 | `TestNoRetiredHueSurvives` cannot be satisfied by a comment | **Skip comment lines**, as `TestNoAccentTokenSurvives` already does, and restore the two comments that were reworded to evade it |
| 11 | G115 is excluded globally | **Delete the exclusion.** Measured: `gosec -include=G115 ./...` finds **zero** production issues, and all four sites its comment defends already carry `#nosec G115` with their bound written beside them (`nftables.go:2389`, `:2440`, `totp.go:75`, `:118`). Under CI's flags (`-tests -tags integration`) there are 10 findings at **6 distinct sites, every one in a `_test.go` file** — `nftables_log_test.go:66` and five in `totp_test.go`. Those six get `#nosec` with a reason. The blast radius the entry feared is six lines in two test files |
| 12 | `.opencode/opencode.json` is in two commits | **Do not rewrite history.** A public repository, 175 merged pull requests, and a file containing nothing sensitive. The entry becomes a permanent note; `.gitignore` already prevents a recurrence |

## 3 · Three premises in the file do not hold

Found while measuring for §2. Each is corrected in the entry's own wording as
part of closing it, because a closed entry that closed for a wrong reason is a
worse record than an open one.

| Premise as written | What the measurement says |
|---|---|
| *`.callout-info` is the last blue on the documentation site … no token* | Half wrong. **All three callouts** hard-code their wash and edge: info `rgba(56,189,248,…)`, warning `rgba(245,158,11,…)`, success `rgba(16,185,129,…)`; and in all three the **text** colour is already a token (`--code-builtin` / `--code-number` / `--code-string`). The pattern is uniform. There is no info-only exception to fix — only three literals to name, and a question about blue-for-info that ruling #4 answers |
| *The two `TestRulesIsEmptyCountsEveryField` rows are a duplicate* | Not a copy. `:142` reasons *"a seventh rule set added and forgotten"*; `:166` reasons *"it decides whether a host counts as configured"*. Two different claims about the same guard — which is why ruling #2 merges rather than deletes |
| *`overflow-wrap` would break every long identifier at an arbitrary point* | True of `anywhere` and `break-all`; **not** of `break-word`, which breaks a word only when it cannot fit on a line by itself. The entry's own counter-example, `proxy_set_header`, fits in 390px and would never break. The 462px `TestLoginVerify_TheSixteenthCodeAttemptDoesNotGetThrough` does not fit and would. The rejected fix was rejected for a property it does not have — to be confirmed in the render, not on this reasoning alone |

## 4 · Bucket A — nineteen diffs

Grouped by what they touch, because that is what decides which can run in
parallel.

### A1 · The 2.18 mutation audit's seven survivors

| Entry | What closes it |
|---|---|
| The ACME serving path has no unit coverage | `tlscert_test.go`: build a `certManager` with ACME enabled and assert `m.acme != nil && m.sslDir == ""`; then a `hello` for the wrong SNI must return autocert's host-policy error rather than a self-signed certificate. Both mutations the entry names must go red without the integration build tag |
| No ceremony runs with a port in the origin | A `withBindAddr(":12227")` option on `newPasskeyTestServer`, used by one end-to-end ceremony test. The fixture's `127.0.0.1:443` default stays — it is how an ordinary HTTPS installation reads — so this adds a case rather than changing one |
| `TestTheRuleIsStatedOnce` catches the identifier, not the rule | The guard at `auth_test.go:191` greps for `minPasswordLen`, so the one restatement a careless author would write — `len(pw) < 12` — is the one it cannot see. It must also refuse an integer literal compared against a password length outside `auth.go` |
| `passkeys.json`'s mode is unpinned | A test asserting `0600` after `passkeystore.go:279`'s atomic write. The file holds credential IDs and public keys, not secrets — the reason to pin it is that an operator reads `ls -l` for reassurance |
| The `v3:` domain separator is not pinned | A golden digest for `credentialFingerprint` (`auth.go:185`) over a fixed input, so reverting the separator to `v2:` goes red. The same claim was made for v1 → v2 in 2.8 and guarded neither time |
| `Prompt` could return false; `newACMEManager`'s hostname re-check is unguarded | Two unit tests in `acme.go`'s package that need no kernel: `Prompt` returns true, and `newACMEManager` refuses an empty hostname. Today both are caught only by the integration test that cannot run without `CAP_SYS_ADMIN` |
| `JustGated` is unobservable when no codes are minted | `password.html` nests `{{if .JustGated}}` inside `{{if .Codes}}`. Un-nest it, and add an HTTP-level test that sets the flag without minting codes |

### A2 · A skip that reads like a pass

| Entry | What closes it |
|---|---|
| Four `TestIntegration_Forward_*` skip in a container | The pattern already exists in this repository: `EASYWALL_REQUIRE_SELFTEST` was added in 2.17 for exactly this class. An `EASYWALL_REQUIRE_FORWARD` equivalent makes the skip a failure where the precondition is supposed to hold, so CI cannot report a pass for four tests that never ran. **Re-bucketed from D to A** during this spec: the entry describes the fix without naming it |

### A3 · The web client's wire edge

| Entry | What closes it |
|---|---|
| `GetHealth`, `GetUsage` and `GetAppliedConfig` have no malformed-reply test | Three tests through `fakeCore` (`internal/web/testhelpers_test.go:32`), which is a real Unix socket listener: a reply that parses as a frame and not as the payload. `!resp.Success` is already covered; this is the other half |

### A4 · The checks that check the wrong thing

| Entry | What closes it |
|---|---|
| `ui-check.mjs` does not derive its URL from the config it reads | `scripts/ui-check.mjs:48` is `process.env.EASYWALL_URL \|\| 'https://127.0.0.1:12227'`, while the script already parses `web.toml` for the password hash. It must take the address from there, so `EASYWALL_DEMO_ADDR` cannot leave the check driving a different server than the one under test. The incident this closes: a run that reported *"UI checks passed"* against a stale `easywall-web`, having never loaded the stylesheet it was checking |
| `check:ui` is not re-runnable against a live demo server | One entry, not two — the 2.17 row and the *check:ui race fix* row are the same finding. `checkPortsCatalogue` must either tolerate rows that are already there or reset the demo state itself. Chosen: tolerate, because a check that mutates a maintainer's running session is the larger surprise |
| The spelling gate's scope does not match its configuration | Move the `codespell` step from `docs.yml` to `test.yml`, repo-wide with no paths. Measured 2026-09-09 and re-confirmed here: from the repository root with config honoured it produces zero findings and exits 0. Trap to preserve: passing explicit paths bypasses the skip list's `./`-prefixed entries, which is why the repo-wide invocation is both the correct one and the cheaper one |

### A5 · Two sources for one thing

| Entry | What closes it |
|---|---|
| The mark has two homes | `web/static/icon.svg` and `docs/assets/img/icon.svg` are byte-identical, and `DESIGN.md` calls the first the single source of geometry. Nothing has drifted yet — a guard test asserting the two files match closes the entry without choosing a build step, and makes the drift impossible rather than merely absent |
| Prose leaks into the stylesheet — from templates **and** from `docs/` | Two entries, one cause: Tailwind scans HTML comments and generated Markdown for class-like tokens, so `.absolute` ships because `password.html` explains an absolute position, and writing *"a ring with no border change"* in the changelog added `.ring` and 1,650 bytes to `docs/assets/css/style.css`. Measure first whether `@source not` can exclude the generated changelog page and template comments. If it can, that closes both; if it cannot, the rule *rebuild and commit the stylesheet whenever a template comment changes* is written where an author meets it, and both entries close as a documented constraint rather than a surprise that turns the next pull request red |

### A6 · Wording and bookkeeping

| Entry | What closes it |
|---|---|
| `boot_enforce_failed` reads *"at startup"* | Since 2.7 it is also written when a mid-apply panic teardown fails. `actionLabel` (`internal/web/server.go:698`) is what the audit filter searches, so somebody hunting a 15:00 teardown failure must search for *startup*. Reword `audit_boot_enforce_failed` in both locales and the *Reads as* column in `docs/_docs/features/audit-log.md` |
| `rollback_skipped`'s label | Same shape, same fix: `audit_rollback_skipped` now also covers a rollback that was written and then torn down. The detail says so; the label does not |
| `npm run build:diagrams` is not byte-reproducible | The entry is correct and the repository's own rule is not: **`CLAUDE.md` still says *a generated file is rebuilt and diffed, never assumed***, which holds for the two stylesheets and the changelog page and not for the diagrams, because Mermaid jitters bezier control points. `check:diagrams` compares a `data-source-digest` and is the staleness test. One-line amendment to `CLAUDE.md` |
| *Every page pays a `GET_STATUS`* | Bookkeeping. The entry's own text says *Shipped in 2.14* as `Server.statusForRender`; it was never struck through. Strike it |

## 5 · Bucket C — seven contract changes, each proposed

These are the items where the thing works and its contract is narrower than a
reader assumes. Each proposal below is what the spec asks approval for; a
declined one returns to the file as a Bucket D entry with the refusal as its
reason, which is a smaller file and an honest one.

| Entry | Proposal | Cost |
|---|---|---|
| `RunPeer` collapses every dial error into `blocked` | The peer reports *why* it could not connect, so a harness fault cannot be recorded as a verdict — the one inversion `foldClaims`'s doc comment forbids. Changes the pipe protocol in `netns.go`'s header comment | One protocol field, one header comment, and a test that a harness fault yields `unprovable` rather than `failed`. The concurrency trigger stays: `wire()` takes no lock, and today the collision wins the race every time (measured: three runs of two concurrent `RunSelftest()` gave all six `unprovable`), so the inversion is currently prevented by accident |
| `GET_HEALTH` is documented short-deadline and takes the nft mutex | Move `CmdGetHealth` out of `CommandTimeout`'s default branch and give it the class `CmdGetUsage` already has, for the reason `CmdGetUsage`'s own comment gives. Correct the health check's comment, which claims read-only implies non-blocking | A deadline class and two comments. `GET_STATUS` has done the same thing since before 2.17 and is out of scope: what 2.17 added was a new consumer, and the measured cost is a wrong word in `docker ps` — nothing restarts on unhealthy |
| `TestEveryRuleIsAddedThroughTheRecordingAdder` matches a selector | Leave the guard and **narrow the claim**. Measured: `cn := m.conn` followed by `cn.AddRule(…)` passes it. Refusing that needs type resolution rather than syntax — a different kind of guard, and `invariants.md` already states the scope as the selector rather than the intent. Closes as a D-style move: the limit goes in the guard's own comment | Two comments. Declining to build a type-resolving guard for a shape no copy-paste produces |
| `ewst-r`, `ewst-p` and `10.77.9.0/24` have no collision check | Check before creating, and report `unprovable` with the collision named. Measured: today a live host table yields `unprovable` with an accurate detail, never `failed`, 2 of 2 runs — so this hardens an honest path rather than fixing a dishonest one | One check, one detail string, one test. The fixed range and names stay: a per-pid name collides one step later on `10.77.9.1/24` anyway |
| `render-changelog.mjs` is both renderer and checker | `check:changelog` gets a second, independent list of versions to compare against — `CHANGELOG.md`'s headings read by a parser that is not the renderer's. Measured at `b723422`: `CHANGELOG.md:108` read `## [2.15.1] —## [2.15.1] — 2026-09-07`, the page carried 32 `<details>` rather than 34, and the check reported it current | A second parser, deliberately dumber than the renderer. The alternative — trusting one regex to decide both what is written and whether it is right — is what shipped a release with no section of its own |
| `podman build` silently drops `HEALTHCHECK` | Accept a compose-level `healthcheck:` and give up the single-definition rule, rather than publishing an image as the documented path — which is a distribution decision this sweep should not take. OCI has no healthcheck field, so a podman build exits 0 producing `HealthCheck: null`, and `docker.md`'s `--format docker` is a workaround | One duplicated definition, and a note saying which is authoritative. Ready-to-lift text exists in the 2.17 task-11 report, fix round 3 §1 |
| `CmdPanic`'s 35 s deadline | On expiry the CLI reports the daemon is not answering, while the marker reached the disk in the first millisecond. The timeout must read the marker and report what it finds. Carried since 2.7 and re-deferred in 2.15 on the grounds that it belongs to a release looking at the console tool | A CLI contract change and a test. This sweep is that release, or the entry is refused rather than deferred a third time |

## 6 · Bucket D — six entries that close by moving

No diff closes these. Each moves to where a reader meets the limit, with the
reason it stays, and leaves `carried-forward.md`.

| Entry | Where it goes | The reason, stated |
|---|---|---|
| `TestIntegration_TheWindowIsOpenWhileTheRulesAreLive` is load-sensitive | The test's own comment, and `invariants.md` | It polls `Status()` with `runtime.Gosched()` and no sleep, watching a gap its comment calls *"a handful of instructions"* wide. Under container CPU contention the test truly observes a real transient state. Seen once in roughly 15 runs. Byte-identical to `b723422` |
| The `auditBuildFindings` call site is covered by no test | The function's comment, and `invariants.md` | `Firewall.nft` is a concrete `*NftablesManager`, so `Apply` cannot reach the line without a kernel; covering it needs a deliberately broken builder, which is a mutation and not a test. The function itself is fully covered |
| The kernel-write guard cannot see reachability | Already in `daemon_source_order_test.go`'s own comment | The guard passes when the panic check is kept textually and wrapped in `if false`. Not closable without `go/parser` and constant folding |
| …nor call order beyond *after the write* | Same comment | Moving `apply`'s check to after `f.rollback` does not fire it, though the comment says the order matters |
| The nft mutex is pinned only under `integration` | `nftables_mutex_test.go`, already self-documented; add the CI half to `invariants.md` | `make test` cannot notice `mu sync.Mutex` being deleted. CI's `test-integration` job does run it, which is the half the entry does not say |
| `.opencode/opencode.json` is in the history | `invariants.md`, beside the `.gitignore` entry that records the accident | Ruling #12: a public repository with 175 merged pull requests, a file with nothing sensitive in it, and a history rewrite that costs every clone and fork. Not worth it, decided rather than deferred |

## 7 · What this touches outside Go

| Surface | Why, and what must be re-done |
|---|---|
| **Screenshots** | Bucket B changes visible surface on `/ports` (the aside returns above a threshold), on `security.md`, on every documentation page with a callout, and in every highlighted configuration block. The 36 shots taken on 2026-09-11 then describe an interface that no longer exists. Re-taken in this work, in the same change, per this repository's rule — and the version chip still reads `v2.18.0`, which is correct, because this sweep is unversioned |
| **Both locales** | `audit_boot_enforce_failed` and `audit_rollback_skipped` in `en.json` and `de.json`, which stay at exact parity |
| **Published documentation** | `features/audit-log.md`'s *Reads as* column follows the two labels. `docker.md` follows the healthcheck ruling. The changelog page gains its version list |
| **`DESIGN.md`** | `module-active` becomes a shadow; the toggle count is corrected in whichever paragraph is wrong; three callout tokens are added. Amended openly, in the file, because this spec has already found that the design document has been wrong before |
| **`invariants.md`** | The `:106` table is folded; the duplicate row is merged and deleted; six Bucket D entries arrive; and every new guard from Bucket A is listed with the incident that produced it |
| **`CLAUDE.md`** | One over-broad rule about generated files, corrected for diagrams |
| **Generated assets** | `web/static/style.css` and `docs/assets/css/style.css` are rebuilt and committed. Diagrams are checked with `check:diagrams`, not rebuilt and diffed — which is the amendment above |

## 8 · The proof this work owes

| Claim | How it is proven |
|---|---|
| Every new test would catch what it says | Mutation: break the implementation, watch it go red. Seven tests in 2.12 passed for the wrong reason and were found this way, and 19 of 75 mutations survived in 2.18 — which is why nine of Bucket A's entries exist at all |
| Every visible change looks right | Rendered in a browser at 1600 / 900 / 390 px in both themes. Not read in CSS: a clipped port number, a class that no longer existed and a documentation site with no background were all invisible in their diffs |
| The stylesheets shipped what the source says | Grep the built file. A green Tailwind build is not proof the rule survived |
| Removing the G115 exclusion breaks nothing | `gosec -include=G115 ./...` before and after, and under CI's `-tests -tags integration` flags. Measured baseline: 0 production, 10 findings at 6 test-file sites |
| `codespell` repo-wide is safe | Run from the repository root with no paths. Measured: zero findings, exit 0 |
| Nothing was carried that this work caused | Every remaining claim of *pre-existing* is proven against `765d615` — the commit this branch is cut from — and never against the branch head. 2.15 carried a `check:prose` failure as pre-existing that its own earlier task had written |
| The file is actually empty | `carried-forward.md` holds its header, its rule, its one exception, and the closed entries as history. No open row survives, and each of the 43 is traceable to a diff, a ruling, or a moved paragraph |

## 9 · Out of scope

- **No version bump, no tag, no release assets.** Decided: unversioned on `main`.
- **`GET_STATUS`'s own mutex contention**, which predates 2.17 at the core level.
  Bucket C touches `GET_HEALTH` because 2.17 added the consumer; the older half is
  a separate piece of work.
- **Publishing a container image** as the documented compose path. It is the
  better fix for the healthcheck entry and it is a distribution decision.
- **A type-resolving guard** for `.conn.AddRule`. Declined in §5 with the
  measurement that says why.
- **Rewriting Git history.** Ruling #12.
- **The three items the file already ruled on and marked *not carried*.** They
  are not reopened: `easywall-core status` exiting 2 with no daemon, the panic
  banner having no button, and the whitelist screenshot's absence beating a
  relabelled one.

## 10 · The end state

`carried-forward.md` keeps its header, its rule, the 2.17 exception and every
struck-through entry — the file is a record, not a scratch pad. What it no longer
has is an open row. The next release starts against a file that says *nothing is
outstanding*, and the next entry written into it is the first thing the next
piece of work found rather than the forty-fourth thing nobody got to.
