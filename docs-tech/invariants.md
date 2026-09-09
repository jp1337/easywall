# The guard tests

Ordinary unit tests check that code does what it says. These check that the
*repository* stays consistent with itself — that the documentation, the workflows,
the packaging, the translations and the generated files still describe the thing
that ships.

Every one of them exists because that consistency had already broken, silently, in
a way no reviewer spotted by reading. When one fails, the useful question is not
"how do I make it pass" but "which half is wrong".

## Documentation describes what exists

| Test | Protects | What happened without it |
|---|---|---|
| `TestEveryConfigKeyIsDocumented` | every `toml` key appears in `docs/configuration.md` | the README said nine protection modules when there were twelve, and three shipped documented as working while producing no rule |
| `TestEveryConfigKeyIsInTheSchema` | both JSON Schemas know every key | both set `additionalProperties: false`, so a key the schema missed was reported invalid in the operator's editor while the daemon accepted it — `ipv6.mode` and `demo_mode` |
| `TestEveryPageIsDocumented` | every `r.Get` route has a documentation page | `/firstrun`, `/apply` and `/dashboard` had none. `/apply` is the feature easywall exists for, and its screenshots sat in the repository referenced by nothing |
| `TestAuditColourTableMatchesTheCode` | `docs/features/audit-log.md`'s colour table equals `auditActionTones` | the table said four coloured actions, the code coloured five, and the missing one was `rollback_failed` — the entry that same page calls the one worth alerting on |
| `TestEveryChangelogVersionHasALinkDefinition` | every `## [x.y.z]` heading in `CHANGELOG.md` has a matching `[x.y.z]: …` link definition | 2.12.0 shipped with none, so its heading rendered as the literal text `[2.12.0]` instead of a link to its diff |
| `TestUnreleasedComparesAgainstTheNewestRelease` | `[unreleased]` compares HEAD against the newest release | it sat at `v2.8.0` for four releases — wrong since 2.9.0. A stale one is worse than a missing one: it renders as a working link to a diff that already shipped, so nothing looks broken. The release checklist is not the fix; the same checklist had been followed four times |
| `TestDocsVersionMatchesRelease` | `docs/_config.yml` `version:` equals `shared.CurrentVersion` | the sidebar badge was hardcoded `v2.4` and drifted a patch release behind |
| `TestEveryEnvVarIsDocumented` | every `shared.CoreEnvVars`/`shared.WebEnvVars` name appears in `docs/_docs/environment.md`, and the page names nothing else | the operator complaint that started the environment-variable feature was "there is no list" — a page that drifts from the code recreates exactly that |
| `TestTheEnvironmentPageIsInTheNav` | `docs/_config.yml`'s `nav:` links to `/docs/environment/` | a page reachable only by its URL is, for a page whose whole point is being findable, the same as not adding it |
| `TestEveryDocsPageIsInTheNav` | every page under `docs/_docs/` has a `nav:` entry, and every `/docs/` nav entry has a page | the same check applied to the collection instead of to one name. Grouping the twenty-seven flat sidebar entries into five sections moved every path in the file at once — a page dropped in that move, or a path mistyped into a link that 404s from the sidebar of every page on the site, is invisible in the diff |

## The preview reports everything it can change

| Test | Protects | What it would have shipped |
|---|---|---|
| `TestDiffRulesReachesEveryRuleSet` | every field of `shared.Rules` is reached by `DiffRules` | a seventh rule set the apply screen silently omits: the operator reads "what changes", sees six sections, and the seventh applies anyway |
| `TestDiffConfigReachesEveryOption` | every leaf of `FirewallOptions` and `NetworkSettings` is reached by `DiffConfig`, or is named in `skippedConfigKeys` with a reason | the same defect one struct over, and the one that produced this release: option changes were in no pending calculation at all, so `/options` said "apply to activate" while `/apply` said there was nothing to apply |
| `TestIntegration_ReachableAgreesWithTheKernel` | `shared.Reachable`'s verdict equals what a real packet from a real source address meets | the chain order is duplicated between `nft.Apply` and `Reachable` by construction. Without a real packet the lockout warning is an assertion about nftables written in Go, and the two have disagreed before — that is what `nftables_semantics_test.go` exists for |
| `TestCoreWritesItsFilesForRootOnly` (extended) | `applied-config.json` is 0600, beside the audit log and the last-apply marker it now shares the assertion with | it holds the machine's whole firewall configuration and only the core reads it; the web process asks over the socket |
| `TestLogEventPayloadCarriesNoFreeText` (extended) | `LogEventPayload.Proxied` stays a `bool` | true only when resolution fell back to the peer, false when it named a client; a string here would be a way for the web process to write arbitrary text into the core's own log through a field that looks like a flag |

## The counter is a number somebody will close a port on

A counter that is wrong is worse than no counter. Every guard here was verified
by breaking the implementation and watching it go red — 2.12 shipped seven tests
that were green while proving nothing, and this release was written not to repeat
it.

| Test | Protects | What it would have shipped |
|---|---|---|
| `TestPortAcceptRules_EveryPortKernelRuleCarriesItsRuleID` | every kernel rule a port rule produces carries the id in its `UserData` comment | a UI rule with three sources is three kernel rules; untagged, the collector finds nothing and every port reports `never` for ever, with no error anywhere |
| `TestPortAcceptRules_TheCounterSitsAfterTheMatch` | `expr.Counter` sits immediately before the verdict | placed earlier it counts what *reached* the rule rather than what matched it — a plausible, monotonically rising figure measuring the rules above it, on a port nobody has ever connected to |
| `TestCollectionReadsTheInputChain` | `addPortAccept` writes to the input chain and the collector reads the same one, both by the same constant | a collector pointed at a chain nothing writes: every port `never`, no error, and the two halves are decided by an argument at a call site that no runtime assertion can see |
| `TestApplyCollectsBeforeItFlushes` | the counters are booked before `nft.Apply` rebuilds the table | the flush zeroes every counter, so the whole interval since the last tick is lost — silently, and it looks exactly like an idle port |
| `TestARebuiltTableDoesNotLoseTheInterval` | the baselines are reset after the kernel write | the next delta measured against counts no longer in the kernel: nothing booked at all until the new counters climb past the old totals, which on a busy port is an hour of traffic reported as silence |
| `TestEveryKernelWriteBooksTheCountersFirst` | **every** write that destroys the kernel counters books them first — all five, in every non-test file in the package, not just the one `apply` | the two guards above scope themselves to `apply`, and four of the five sites were invisible to them. The rollback case is every unconfirmed apply: collect at T+0, apply at T+2min, the window expires at T+4min and the flush takes those two minutes with it, so a port whose only use was in the window reads `never` and the dashboard advises closing it |
| `TestValidateRules_RejectsAnIDThatIsNotOne` | a rule id is twelve lowercase hex characters, checked at the boundary the web process crosses | an arbitrary string reached the kernel: `userdata.Append` writes the TLV length as `byte(len(data))`, so at 254 characters every apply is refused until that rule is deleted — the firewall cannot be changed — and at 255 the length byte wraps to zero, the id read back is `""`, and the port reports `never` for ever. A 400-character id was stored and rendered before this existed |
| `TestEnsureRuleIDs_ReplacesAMalformedID` | a malformed id is repaired on the write paths, not left for the validator | a hand-edited `rules.json` would otherwise reach `nft.Apply`, be refused there, and leave a machine whose firewall cannot be applied at all until somebody finds the line |
| `TestUsageStore_ACorruptFileDoesNotWedgeTheCollector` | an unparseable `usage.json` is warned about once and started again | `Collect` reads before it writes, so a hard read error made the bad file permanent: one warning per interval for the life of the installation, and *Last used* rendering `—` for every rule for ever |
| `TestUsageStore_ADaemonRestartDoesNotDoubleCount` | the baseline is in `usage.json` and not in a struct field | a memory-only baseline re-counts everything already booked on the first collect after a restart — a port that saw one packet a month ago reporting as busy today. The case is a table that survived the daemon: a crash, or a stop and start while the rules stayed live. Not the ordinary restart, where `Daemon.Start` restores unconditionally and `Collect`'s below-baseline branch is what keeps the count honest |
| `TestRuleIDsAreUniqueAndStable` | ids are assigned on write paths, never on a read, and never rewritten | a value generated per read is a different value every read, and every counter is keyed to a rule that no longer exists by the time it is read back |
| `TestARuleWithoutAnIDRendersEmDash` | a rule with no id renders `—` and not `never` | `never` is a measurement and the one an operator acts on; printing it for a rule nothing has been able to key on is a measurement nobody took |
| `TestThePortsFormCarriesTheRuleIDBothWays` | `data-id` is rendered and read back by `syncHidden` | the form rebuilds the whole rule list from the DOM at every save, so editing one description would re-key every rule on the page and orphan every counter — and the column would still render |
| `TestStatTileSpanMatchesItsChildren` (existing) | the dashboard's new second line lives inside the tile's note | a conditional fifth child gives six tiles two different heights, in one language, on the front page |
| `TestIntegration_TheCounterMovesWhenAPortIsUsed` | packets sent at a real kernel move a real counter, and the total survives an apply | the ten guards above could all be green with the feature reporting `never` for every port, because none of them puts a packet on a wire |
| `TestPanicAndResumeShareALock` | `Panic` and `Resume` serialise | `Resume`'s `ClearPanic` landing between `EngagePanic` and `nft.Reset` leaves no marker and an empty table: unfiltered, with nothing recording that anybody chose it |
| `TestPanicLandedDuringWriteIsToldTheMarkerState` | the helper acts on the state its caller read | three stats of one marker in a rollback is three chances to disagree, and an unreadable marker between the second and the third puts back the inversion the gate exists to prevent |
| `TestEveryFailurePathInApplyIsAudited` | every error return in `apply` writes an entry first | four of five returned into a journal nobody reads, on a machine whose interface says nothing about why the firewall did not change |
| `TestRestoreCurrent_RecordsTheBridgesItBakedIn` | `RestoreCurrent` records the bridges it applied | all four reconciler tests wrote the field directly, so deleting the call left the suite green — and that call was the whole subject of the commit that added it |

## Who a request is from

A mistake in the trusted-proxy check is a login-rate-limiter bypass — the three
advisories `buildRouter` has cited since it was written. Reading the code is not
enough, and a unit test writes the one field a real request does not choose.

| Test | Protects | What it would have shipped |
|---|---|---|
| `TestTheEmptyListIsTwoPointTwelve` | with no list configured, `resolveClient` equals the peer and the presence check, for every request shape | 2.13's default silently differing from 2.12's only behaviour, on every installation that configures nothing |
| `TestIntegration_AnUntrustedPeerCannotChooseItsAddress` | a forwarding header from a peer that is not on the list changes neither the address, nor the marker, nor the bucket — measured with a kernel-assigned peer | the header believed unconditionally, which is `middleware.RealIP` and the advisories |
| `TestIntegration_ATrustedPeerResolvesToTheClient` | a peer on the list resolves to the client and is no longer marked `via-proxy` | the feature wired to nothing: a list that parses, validates, documents, and never changes an answer |
| `TestIntegration_TheCallerCannotNameATrustedProxyAsItself` | the rightmost-untrusted walk; naming a trusted address in the header does not hand the caller that identity | the smaller bypass — the caller picks its own address by writing the proxy's, and gets a fresh rate-limit budget per attempt |
| `TestIntegration_TheLimiterKeysOnTheResolvedClient` | the bucket key is the resolved client both ways: one budget for an untrusted peer however it rewrites the header, one per client behind a trusted one | either half alone — a shared budget that was the point of the release, or a per-header budget that is the bypass |
| `TestTheIntegrationJobCoversEveryTaggedPackage` | every package with an integration-tagged test is in the workflow's `go test` path | the five above green in CI and never executed — the job ran `./internal/core/...` alone |

## One source for a version

| Test | Protects |
|---|---|
| `TestGoToolchainIsTheSameEverywhere` | `go.mod`'s `toolchain` line, the Dockerfile tag, `debian/control` and seven prose pins agree — and nothing under `.github/` spells a version out. The workflows are a list; the composite actions are a glob, because a hand-written list is what let the search index's `setup-node` step move into `.github/actions/` outside the guard's reach |
| `TestTheCSRFClaimNamesTheReleaseItArrivedIn` | the five sentences saying `CrossOriginProtection` arrived in Go 1.25 are **not** treated as version pins |
| `TestRenovateEditsOnlyTheGoPinsItShould` | Renovate's own regexes, run against the tree, capture only the toolchain |
| `TestEveryRenovateFilePatternReachesAPin` | every `managerFilePatterns` entry matches a tracked file, and those files contain a pin. `renovate.json` still named `docs/installation/manual.md` after the Jekyll move put the page under `docs/_docs/`; what is at the old path is a `redirect_to` stub with no version, so Renovate read it, found nothing and said nothing |

The background is in [dependencies](dependencies.md).

## The workflows still prove what they claim

| Test | Protects | Why it is not obvious |
|---|---|---|
| `TestCodeQLSeesTheGoToolchainItTraces` | `setup-go` runs **before** `codeql-action/init` | the wrong order makes CodeQL analyse a build it never traced, and it says so as a *warning* — the job stays green |
| `TestCodeQLBuildsEverything` | the analysis build covers every package | |
| `TestGosecScansTheIntegrationCode` | `-tests` accompanies `-tags integration` | without `-tests`, the tag changes nothing: 41 files scanned either way |
| `TestLatestImageTagIsOnlyForStableReleases` | a release candidate does not move `:latest` | |
| `TestEveryImageArchitectureAlsoGetsAPackage` | the image platforms in `.goreleaser.yaml` and the `.deb` matrix are the same set | the container was multi-arch for a year while the package was amd64 only |
| `TestBothPackageArchitecturesAreInstalledNatively` | each `.deb` is built and installed on a runner of its own architecture | a cross-built package is one nobody installs — see [packaging](packaging.md) |
| `TestTheSearchIndexIsBuiltBeforeThePagesUpload` | both `docs.yml` jobs call `.github/actions/build-search-index`, and `deploy` calls it before `actions/upload-pages-artifact` | the index is the one part of the site that is neither committed nor written by Jekyll. Deleting the call, or moving it below the upload, deploys green — and every visitor gets a search field whose engine answers 404. Dropping it from the pull-request job deletes the only check that the index still covers all 26 pages, since that assertion lives inside the action |

## What the interface promises

| Test | Protects |
|---|---|
| `TestTemplatesOnlyUseTranslatedKeys` | no visible string bypasses `T` |
| `TestLocaleFilesAreAtParity` | `en.json` and `de.json` hold the same keys |
| `TestTranslationsAreNotCopiedEnglish` | a German value is not the English one pasted across |
| `TestMarkupStringsAreRenderedThroughRichText` | a message with a link or a `code` span stays one message |
| `TestClientStringsCoverWhatAppJSAsksFor` | text `app.js` builds has its key in `clientStringKeys` |
| `TestClientStringsCarryNoMarkupAppJSCannotRender` | a string inlined for `app.js` has no `` ` `` or `*` — it escapes them, so the markers would be shown literally |
| `TestTemplateClassesExistInStylesheet` | a template does not name a class Tailwind no longer generates |
| `TestStatTileRowsComeFromTheGrid`, `TestStatTileSpanMatchesItsChildren` | the dashboard tiles take their four rows from the grid above them, and the row count in the stylesheet equals the children in the markup. French was what exposed the need: "Règles personnalisées" wraps where "Custom rules" does not, and two numbers of six sat 20px below the other four. The fix costs English and German nothing, which is why its absence is invisible to everyone who would notice |
| `TestDocsStylesheetKeepsLoadBearingRules`, `…CodeBlockHasASingleFrame`, `…InlineCodeIsNotThemeScoped` | the documentation site's stylesheet after a Tailwind rebuild. `.sr-only` is in that list and is written nowhere in `web/src/docs.css` — it exists only because `docs/_includes/search.html` uses the class and the `@source` scan reaches that file, so renaming either one un-hides the search field's label on all 26 pages |
| `TestVersionedStaticAssetsCarryTheReleaseInTheirURL`, `TestStaticFilesSayHowLongTheyMayBeKept` | an upgrade actually changes the stylesheet URL |
| `TestEveryDocsLayoutDeclaresTheSiteLanguage` | every docs layout says one language, and a `redirect` layout exists at all. The site said two — `en` on 27 pages, `en-US` on the 23 redirect stubs, because jekyll-redirect-from renders those from its own template. Pagefind read two languages and built two indexes that could not see each other; a screen reader was told the wrong language on 23 URLs |
| `TestTheDocsSidebarRendersTheSearchContainer`, `TestTheSearchFieldIsHiddenWithoutJavaScript` | the search container is in the layout, and the built stylesheet hides it until a script says otherwise. Both halves fail silently: the page renders either way, and what is missing is a control, not a page. Both read the layout with its comments stripped: the first version searched for the bare string `data-js`, which a commented-out `setAttribute('data-js', 'on')` would have satisfied just as well — the assertion looked for the substring anywhere in the file, not proof that the line still runs |
| `TestTheDocsLayoutMountsPagefind` | the loader fetches `pagefind-ui.js`, constructs `PagefindUI` against `#docs-search`, and gives the highlight script `type = 'module'`. Every other search guard passed with the whole `<script>` block deleted — what is left then is a placeholder input that accepts a keystroke for ever. The module line carries its own incident: `pagefind-highlight.js` is an ES module whose body also assigns `window.PagefindHighlight`, so as a classic script it throws on the `export` before that ever runs, and highlighting is dead with nothing in the browser saying so |
| `TestMobileSidebarOutranksItsBackdrop` | on a narrow viewport, the open `.sidebar` (z-index 160) sits above `.sidebar-backdrop` (150). Since commit `cd89c02d` (2026-05-03) it had not: the backdrop outranked the drawer it was meant to sit behind, so nothing inside an open drawer — no nav link, no search field — could receive a touch. A future edit to either number regresses it with no other signal |
| `TestTheSearchOverridesAreOutsideTheCascadeLayer` | the search panel's overrides of Pagefind's class names sit outside `@layer components` in the built stylesheet | An unlayered declaration beats every declaration in a named cascade layer, whatever its specificity. Pagefind's stylesheet is fetched at runtime and is unlayered, so an `#id` rule written inside the layer lost to its plain class selectors: the overlay shipped a yellow `<mark>` and a white input on a dark panel while the build stayed green, the rules were present in the built file, and the grep for them passed |
| `TestTheDocsLayoutMountsPagefind` (extended) | the overlay is opened with `showModal()`, and the highlight script is given `markContext` and `addStyles: false` | `show()` gives up the focus trap, the Esc key and the inert background — the four platform behaviours that made a dialog cheaper than a results list in the sidebar. Left at its default the highlight script marked the whole document: every `a` in the sidebar's page list, and the logo read "e a syw a ll" |
| `TestEveryReachReasonHasALabel`, `TestEveryReachVerdictHasALabel`, `TestEveryPreviewSetHasALabel` | every verdict, reason and rule-set heading the apply screen can render has a key in `en` and `de` | a reason with no key renders as `reach_bogon_filter` on the one screen whose whole job is to be believed |
| `TestDetailLabelEscapesWhatItPassesThrough` | `detailLabel` returns `template.HTML`, so what it passes through is escaped by it | the detail column carries values composed from rules an operator typed; the `via-proxy` chip is the first markup that function has ever written |
| `TestScreenshotsAreTakenAboveTheTwoColumnBreakpoint` | `SHOT_VIEWPORT` in `scripts/ui-check.mjs` is wider than the `max-width` at which `.page-grid` drops its context column | the screenshots were taken at 1440px against a breakpoint of 1570px from 2.11 to 2.13, so every figure in `docs/` showed the narrow fallback — aside cards stacked under the table rather than beside it. The two numbers live in different files and neither is near the other: lowering the breakpoint and narrowing the viewport re-create it independently |
| `TestScreenshotsGrowTheWindowInsteadOfCapturingBeyondIt` | `shoot()` resizes the viewport to the document instead of passing `fullPage`, for as long as `.sidebar` is `position: fixed` | a fixed element in a fullPage capture stays laid out against the window it was rendered in. The sidebar therefore stopped at 900px in every taller image, with the language switch, the theme toggle and *Logout* floating in the middle of a column that then went blank — 22 of the 34 files in `docs/assets/img/screens/` shipped that way |

## The rules say what they mean

A rule that is present in the table and matches nothing is invisible to every
count-based test in the repository, and 2.15.1 shipped three of them for five
releases.

| Test | Protects | What it would have shipped |
|---|---|---|
| `TestCtStateMaskIsTheByteOrderTheKernelCompares` | the ct state mask is a native `u32`, so the kernel compares against the bits the name means | the mask was written `[]byte{0x00, 0x00, 0x00, 0x06}` and the kernel read `0x06000000`. The expected bytes in the test come from `nft --debug=netlink`, not from the implementation, so reversing `ctStateMask` turns it red rather than agreeing with itself |
| `TestIntegration_EstablishedTrafficIsAcceptedByName` | one rule in the input chain matches `ct state established,related` under that name, counts, and accepts under the reserved id | it rendered as `ct state 0x2000000,0x4000000` and matched no packet ever sent — the whole stateful half of the input chain. An installation with a live table could complete no outbound connection at all, and applying the rules dropped the SSH session that was already open. 2.17 put a counter and a rule-id comment into that rule, so the three fragments are no longer adjacent text and the assertion is `indexOfRule`, which requires them in the *same* rule: as separate whole-ruleset substrings the verdict half still passed under the byte-reversal and the whole duty fell on one fragment |
| `TestIntegration_NoCtStateRendersAsARawMask` | **no** ct state anywhere in the table renders as hex | the class rather than the three instances. `nft` names every state it recognises, so a raw mask means bits no conntrack state sets — including on a rule added after this test was written |
| `TestRulesIsEmptyCountsEveryField` | `shared.Rules.IsEmpty` counts every field of the struct, by reflection | a seventh rule set added and forgotten makes a configured host look unconfigured, and its stored rules quietly stop being enforced at boot |
| `TestRestoreCurrent_DoesNotEnforceAnInstallationNobodyConfigured` | a host where nothing was ever applied is left alone at boot | `RulesStore` initialises `rules.json` with `emptyState()` and the boot restore installed it: `policy drop` with no port open, so SSH and the interface that would have opened SSH both closed. `docker compose up -d` and `dpkg -i` both reach it before the operator can open anything |
| `TestRestoreCurrent_EnforcesAnEmptySetSomebodyApplied` | the guard above reads "never configured", not "empty" | an operator who deliberately applied an empty set, and an installation upgrading from before the guard existed, would both stop being restored at boot — the one direction that silently stops a firewall |

The shape worth copying: **assert the kernel's own rendering, by name.** Every
`ruleCount` assertion in this package was green throughout, because a rule that
matches nothing is still a rule.

## Colour means state, and it stays that way

2.16 removed the ice-blue accent because a page carrying it in fourteen places
has no accent at all. What these guard is not the removal but the return.

| Test | Protects | What it would have shipped |
|---|---|---|
| `TestNoAccentTokenSurvives` | no `--accent*` custom property in either source stylesheet **or** either committed build output | a token reintroduced "just for this one case" is a one-line diff nobody reads twice, and it puts colour back into a system whose only remaining rule is that colour means firewall state. Both halves, because they disagree: Tailwind drops rules silently, so a token in `web/src` and not in the built file is a different defect from one in both — and only the built file ships. Verified the way a "must not exist" guard has to be, by proving it goes **green** against a copy with the tokens renamed, not only red while they are present |
| `TestNoRetiredHueSurvives` | three literal colours `docs.css` hard-codes from palettes retired before this release | a grep over token *names* walks straight past a literal. Without this, "no accent token survives" was green while the Aurora cyan lived on in three components and a teal in both hero glows |
| `checkFocusIsVisible` (`check:ui`) | every focusable element on all 13 pages composites its focus indicator to ≥ 3:1 against its actual backdrop, in both themes | the outline this suite was written beside was `rgba(143,211,251,0.13)` — 11.4:1 as a colour and **1.31:1** once composited over the card behind it. No number in a CSS file says that. Red at 66 controls per theme before the fix, and the failing set was one shape: a ring with no border change. Three ways to measure it wrong are named in its own comment, because all three were hit writing it — `el.focus()` does not satisfy `:focus-visible` for a button, `outline-style: auto` reports a colour Chrome does not paint, and `getComputedStyle` in the same task as the key press returns the state from before the focus |
| `checkContainersDoNotOverflow` (`check:ui`) | nothing scrolls sideways inside its own container, not only inside the page | a container with `overflow-x: auto` absorbs its own overflow, so the document never widens and the page-level check cannot see it — by construction, for every such container in the stylesheet. `.table-wrap` had been 10px over in card mode since the card layout was written, with the whole suite green. Red 24 times before the fix: four pages, three widths, two themes, and clean at the two widths where the table is still a table |
| `TestTheSidebarGroupsCarryADividerAndAnIndent` | a labelled nav group that follows another labelled group carries a rule above it, and the first one does not | the entry `carried-forward.md` held across three design reviews. Its own first version failed against a stylesheet that was correct: the "bad pattern" it searched for is a *suffix* of the correct sibling selector, so a plain substring match hit the very rule it exists to bless. Anchored on the start of a rule now |
| `TestTheActiveNavItemCarriesAnEdgeAndNotOnlyAFill` | the active item is marked by an edge, not by a background alone | `select-fill` is 1.08:1 against the surface it sits on — the same invisibility as the focus ring this release replaced. The fill confirms; the edge marks |
| `TestTheLanguageSelectIsDrawnLikeEveryOtherControl` | `appearance: none`, a `surface-raised` fill and a chevron of its own | the one native control in the interface. On the login card a `canvas` fill sits 1.03–1.06:1 from the card behind it, so the native arrow was the only thing identifying it — and `appearance: none` without a replacement arrow makes it stop reading as a select at all |
| `TestReleaseVersionDropsTheDescribeSuffix`, `TestTheVersionChipShowsTheReleaseAndTitlesTheBuild` | the topbar chip shows the release and its `title` carries the full `git describe` string | the chip rendered `v2.14.0-44-g5…` for every build off a tag, which reads as a fault in the software rather than as a build detail. Both halves are asserted: swap them and the ellipsis comes back, drop the title and a development build becomes unidentifiable from the interface |
| `TestThePageTitleTakesTheDisplayVoice` | the page title is mono 300 at 30px, stepping to 34px above 900px | 30 is a ceiling reached by measurement — 19 unbreakable characters (`Systemeinstellungen`) at 0.6em advance is 11.4em against roughly 358px inside the padding at 390px — and a size that drifts up breaks the one locale nobody on this repository reads first |
| `TestRulesIsEmptyCountsEveryField` | `shared.Rules.IsEmpty` counts every field of the struct, by reflection | it decides whether a host counts as configured, and a field it forgets reads as "nothing here" — the one direction that silently stops a firewall enforcing rules somebody wrote |

Tailwind drops rules silently and the build stays green. A stylesheet test is a
poor substitute for looking at the page — but it catches the class of failure
where the page looks right on the machine that has the old file cached.

## Behaviour that fails quietly

| Test | Protects | The incident |
|---|---|---|
| `TestStartRefusesToServeWithoutTemplates` | the web process refuses to bind if its templates are missing | it started, reported healthy, and answered `503` on every request, with one `WARN` line at startup as the only clue |
| `TestSessionIsRefusedOnceItIsOlderThanItsLifetime` | the server enforces the same lifetime the browser is told | `NewCookieStore` set the codec's max age from its own default; assigning `Options` afterwards changed only what the browser saw. Cookies were valid for thirty days while the browser dropped them after ten minutes |
| `TestLogoutSurvivesTheRevocationRecordExpiring` | a logged-out cookie stays refused | the revocation record was swept "because the cookie has expired by then". It had not: replaying it eleven minutes after signing out signed you back in |
| `TestSigningOutIsNotReachableWithASafeMethod`, `…RefusesACrossOriginPost` | `/logout` is a `POST`, so `CrossOriginProtection` covers it | it was a `GET`, which that middleware exempts by design — any page the operator had open could end their session with an `<img>` tag |
| `TestCoreWritesItsFilesForRootOnly` | the audit log and the last-apply marker are `0600` | |
| `TestShippedConfigsMatchTheStructsTheyConfigure` | `config/*.toml` — what the package installs — still parses | `config/easywall.toml` shipped the obsolete `ipv6.enabled` a release after `mode` replaced it |
| `TestNoPersonalEmailAddressesAreTracked` | no personal address in any tracked file | one was in `debian/control` and two changelog sign-offs, in a public repository, for four months |
| `TestTheNetworkEditorRefusesExactlyWhatTheCoreRefuses` | the Network page and `shared.ValidateNetworkList` accept the same set | they were three different sets — the editor's, the core's and the demo's. A blank line between two networks, a `#` note, or a bare address passed the page and was refused by the core, and the operator was told to *check core connection* |
| `TestABadNetworkInTheConfigFileStopsTheDaemon` | `docker.custom_networks` and `routing.networks` are validated when read, not only when saved | a hand-edited `10.9.0.0-24` started the daemon with no warning and produced no rule — a network listed as routable, destroyed by the forward policy |
| `TestNoEnvVarTargetsARuleField` | no `shared.CoreEnvVars`/`shared.WebEnvVars` entry names a TOML key the interface writes (`FirewallOptions`, `AcceptanceConfig`, `IPv6Config`, `DockerConfig`, `RoutingConfig`) | `acceptance.duration` looks like deployment; settable from the environment, an operator would change it in the interface, be told it was saved, and find the old value back after the next restart |
| `TestNoEnvVarTargetsAManagedKey` | no `shared.WebEnvVars` entry names one of the five secrets among `web`'s `managedKeys` (`username`, `password`, `session_key`, `totp_secret`, `recovery_codes`) | an environment variable is visible in `docker inspect` and `/proc/<pid>/environ`. `telemetry`, the sixth managed key, is not in this list — see *The configuration comes from outside*, below, for why |
| `TestEveryEnvVarNamesARealTOMLKey` | every `EnvVar.TOMLKey` is an actual field of `CoreConfig`/`WebConfig` (or `TLSConfig`, one level down) | a typo in the table would produce a variable that is documented and guarded against, and reaches nothing |
| `TestEnvOverlayNeverReachesTheConfigFile` (web) | the environment overlay is applied to the in-memory config only; `encode()`'s fallback render path never bakes an environment value back into `web.toml` | with the overlay applied in place, any `Save*` would persist an environment value permanently, with nothing recording where it came from |
| `TestEnvOverlayNeverReachesTheConfigFile` (core) | `saveLocked` restores `socket_path`, `data_dir` and `log_dir` from the file as it stood before `shared.ApplyCoreEnv`; the test walks `shared.CoreEnvVars`, so a variable added to that table and not to the restore fails here | the same defect on the half that speaks netlink, and with no fallback path needed — `saveLocked` always encodes. With `EASYWALL_CORE_DATA_DIR=/data-from-env`, one save from Options, Network or System rewrote `easywall.toml`, and the next start without the variable read `rules.json`, the apply state and the panic marker from a directory the operator never configured |
| `TestTheAdvertisedLimitsAreTheOnesTheDaemonEnforces` | `options.html`'s `min`/`max` and the schema's ranges are derived from `shared.FirewallLimits` | three sets of bounds, none in force: the page said 1–9999 everywhere, the schema said four different things, the daemon only checked `> 0` |
| `TestAnOutOfRangeLimitIsRefusedOnTheWayIn`, `…InTheFileIsClampedAndSaidOutLoud`, `TestEveryFirewallLimitIsWiredToItsOwnField` | a firewall limit cannot reach a 32-bit nftables field and wrap | `connection_limit_max = 4294967296` became `ct count over 0` — every connection from every source, dropped |
| `TestSnapshotAttributesEachChainToItsOwnFamily` (integration) | the post-incident snapshot lists a table's own chains, and says when a count could not be read | chains were matched by name only, so each table was credited with the union across families, and a failed lookup was rendered as `rules: 0` |
| `TestEnforcingIgnoresASameNamedTableInAnotherFamily` (integration) | `Enforcing()` answers about `table inet easywall` alone | it was already right, for a reason — the chain is used only for its name — that nothing stated |
| `TestNeitherConfigIsShippedAsAConffile` | neither TOML is installed under its own name, so dpkg does not track a file easywall rewrites | an unattended upgrade stopped at the conffile prompt and left the package `install ok unpacked` — postinst unrun, services never restarted |
| `TestThePackageVersionIsTheReleaseVersion` | `debian/changelog` and `shared.CurrentVersion` agree | the package version was the only version in the tree tied to nothing; the release compared it to the tag *after* publishing the images |
| `TestAnImportIsNotAbandonedWhileTheCoreIsStillValidatingIt`, `TestTheClientOutwaitsWhatTheCoreMaySpendOnNft` | the client's deadline outlasts what the core may spend in `nft` | one flat 5 s for all fifteen commands meant an import that succeeded was reported as failed, with the staged rule set already replaced |
| `TestTheDiagramPaletteIsTheDocumentationPalette` | the colours in `render-diagrams.mjs` are the tokens in `web/src/docs.css` | two copies under a "keep them in step" comment, and the diagram staleness digest cannot see a palette change |
| `TestDaemonAbsent` | `daemonAbsent`'s verdicts for its errno list (`ENOENT`, `ECONNREFUSED`, `fs.ErrNotExist`) and for the timeout check ahead of it — a defensive ordering against a future errno joining that list while also reporting `Timeout() == true`; none of today's three do, so EAGAIN's false verdict does not depend on the order — EAGAIN was never in the list to begin with | the three tests exercising the no-daemon fallback all dial a path that never existed, so hard-coding `daemonAbsent` to `return true` left the whole package green; nothing pinned the verdicts themselves, including that EACCES and a full accept backlog (EAGAIN) must not read as "no daemon" — the one distinction the CLI is allowed to write nftables on |

## Order that only the source can prove

| Test | Protects | What happened without it |
|---|---|---|
| `TestDaemonStart_SourceRestoresBeforeItListens` | `Daemon.Start` calls `RestoreCurrent` **before** `net.Listen`, and not inside a `go func` | the two runtime tests named after this guarantee — `TestDaemonStart_RestoresAtStartup` and `TestDaemonStart_NoCommandIsServedBeforeTheRestoreHasRun` — both stay green when the restore is moved into a goroutine, because the restore's audit write beats the test's dial-write-read every time. The release's headline promise was held by code structure alone, with two test names claiming otherwise |
| `TestRollback_UnderPanicStillRevertsTheRulesFile` | a rollback interrupted by panic mode still puts `Current` back and leaves `HasPendingChanges` true; only the kernel write is skipped | the panic guard returned before the file revert too, so `Current` kept the set the operator had just been cut off by. `Staged` equalled it, the dashboard reported nothing outstanding, and `resume` reinstalled it with no acceptance window — because `RestoreCurrent`'s whole justification is that `Current` has already survived one |
| `TestEveryKernelWriteIsFollowedByThePanicCheck` | every `f.nft.Apply` in the package is followed by a `panicLandedDuringWrite`, two of them per function because `nft.Apply` reports errors from work that runs after the ruleset is committed. Widened in 2.15: it used to name `firewall.go` and `restore.go`, and now globs every non-test source file in the package, so a third writer of the table cannot be added invisibly | the check was a *missing call* to begin with, and nothing noticed one going away: deleting the one in `apply` left the whole suite green. The nothing-went-unparsed half compares the per-function tally with the file-wide count, so a fourth writer of the table fails the test instead of silently escaping it |
| `TestRunSubcommand_NoDaemonFallbackNamesItselfInTheAuditLog` | the console fallback writes `console-no-daemon` as the audit user | `user` renders literally in the log table — no label map, no view function — so a rename would have left two spellings for the same event with every test still green |
| `TestDaemonTests_StartIsOnlySpawnedByTheHelper` | `startDaemonGoroutine` is the only function in the package's test sources that spawns the real `Start` in a goroutine — eight tests need it, in any spelling. Renamed and widened in 2.15 from `…StartInAGoroutineIsAlwaysWaitedFor`, which forbade only `go func() { _ = d.Start() }()` | `Stop` closes the listener and waits on `d.wg`, which covers the boot restore, the reconciler and each served connection — deliberately **not** the accept loop. So `Stop` returns while the `Start` goroutine is still coming back from `Accept` and taking the quit branch. Measured at **3 runs in 200**. Four tests carried the discarding shape at once, and on 2026-08-30 one of them, `TestDaemonStart_RestoresAtStartup`, went red under `-race` on `main` — a goroutine touching `t.TempDir()`-scoped state while `testing` tore the frame down. The helper also held a **slot in `d.wg` for exactly as long as `Start` ran**, added for 2.15's flake on `TestDaemonStart_RecordsAMarkerItCannotRead` (7 process runs in 160). That slot is **gone as of 2026-09-07**: it was a test-side workaround for a hazard the daemon carried in production, and while it existed no test could see that hazard at all. `Daemon.track` closes it in the daemon; see the row below |
| `TestDaemonStart_StopRefusesAConnectionItCannotWaitFor` | a connection accepted in the instant `Stop` begins shutting down is either registered in `d.wg` before `Stop` waits, or refused and **closed** — never `Add`ed to a counter of zero nobody is waiting on. It sets `usage.interval` to 0 explicitly, because the 300-second default keeps the ticker holding a slot for the daemon's whole life and the counter then never reaches zero | The accept loop's `d.wg.Add(1)` was exactly the ordering `sync.WaitGroup.Add`'s documentation forbids: a positive delta from a counter of zero, unordered against the `d.wg.Wait()` in `Stop`. `Wait` could return without counting a connection just accepted, so a socket request in flight during `systemctl restart easywall-core` was dropped and the web process reported the core unreachable for it. Reproduced at **76 process runs in 160** with the test-side slot removed — sixteen copies of the package sharing eight cores under `-race`, `GOMAXPROCS=8`; **0 in 256** after the fix. Restoring the bare `d.wg.Add(1)` in the accept loop puts it back at **15 in 32**; restoring the other three — the boot restore, the reconciler and the ticker — puts back a second, much rarer race at **3 in 1 280**, on `TestDaemonStart_Stop` and `…ChownPath`, which poll `os.Stat` for the socket and then proceed regardless, so `Stop` can reach `Wait` while `Start` is still inside `RestoreCurrent`. All four go through `track` for that reason and not merely for consistency. Neither `go vet`'s `waitgroup` analyser nor `golangci-lint` flagged the pattern, before or after: `vet` only reports an `Add` called from inside the goroutine it counts. The fix is not a slot for the accept loop nor a done-channel `Stop` waits on — either would make shutdown depend on `Stop` closing the listener first, the dependency the comment on `Start` rejects. It is a decision taken under the `d.mu` that `Stop` already holds before it waits |
## The acceptance window

| Test | Protects | What happened without it |
|---|---|---|
| `TestAcceptance_ShutdownBeforeTheWindowOpensDoesNotOpenOne` | A SIGTERM that lands between `beginApply` and `Acceptance.Start` is remembered rather than discarded | `Stop` cancels and then waits on the WaitGroup that tracks the apply goroutine. `Cancel` no-ops while the status is `Idle`, so the cancel was lost, the window opened after it, and `Stop` sat behind `Wait` for the full duration — past `TimeoutStopSec`, after which `SIGKILL` leaves the unconfirmed rules live. That is exactly the failure `Cancel` was written to prevent |
| `TestFirewallApply_OpensTheWindowBeforeItWritesTheKernel` | `f.acceptance.Start` precedes `f.nft.Apply` in `Firewall.apply` | `Start`'s doc comment required it and the call site did the opposite. Between the two ran a panic-marker stat, an applied-config write, a settings read and an audit write — two file writes, which on an SD card are not microseconds. For all of it the kernel held unconfirmed rules while `Status()` answered `idle`, so the interface showed no window during the one gap where a lockout is possible and invisible. Held by statement order alone, so it is read off the source |

## The second factor

| Test | Protects |
|---|---|
| `TestNoTemplateCarriesAVersionLiteral` | `base.html` said `v2` for four releases and nothing noticed |
| `TestDemoModeRefusesToWriteCredentials` | a visitor to the public demo could change the password; the list is what a new credential-writing route has to join |
| `TestAllLoginEventsMatchesTheProtocolSource` | the same shape as the `AllCommandTypes` guard, one release later |
| `TestEveryLoginEventIsLabelledDocumentedAndTranslated` / `TestNoLoginEventIsColoured` | an event that renders as raw snake_case, and colour drifting away from "what the firewall is doing" |
| `TestLoginVerify_TheSixteenthCodeAttemptDoesNotGetThrough` | the roadmap's requirement that the second step be bounded, as arithmetic that runs |
| `TestConfig_TOTPKeysSurviveTheSaveRoundTrip` | `mergeConfig` silently falling back to the encoder and taking three kilobytes of comments with it |
| `TestFirstRun2FA_SkipCreatesTheAccountWithoutAFactor` | the wizard's setup step must always offer a way past it that still creates the account. easywall runs on single-board computers with no RTC, which come up at the epoch until NTP lands; TOTP cannot verify against a clock like that. Without this branch an optional feature becomes a way of bricking the wizard on a machine that is already reachable from the network |

Worth writing down beside the last one, though nothing tests it: `keyLineRe`
matches one line, so a hand-written multi-line `recovery_codes` array makes the
merge give up and re-encode. Comments are lost, nothing is corrupted, and that
is the path's designed behaviour rather than a bug the round-trip test missed.

## A port rule can now name who may reach it

| Test | What it protects |
|---|---|
| `TestValidateRules_PortSources` | the trust boundary the web process crosses: a source that is not an address, a hostname, or an address wearing a port number is refused before it ever reaches the core |
| `TestReachable_PortSources` | the base truth table for the field: empty is anywhere, an address or network inside or outside a restriction, a bare address, and a source list holding only a comment restricts to nobody |
| `TestReachable_PortSources_AnUnrestrictedRuleWins` | a second, unrestricted rule for the same port opens it even when an earlier rule for it is restricted — the kernel's first-match order, not the more cautious of the two, which is exactly what a later edit would "fix" by mistake |
| `TestReachable_PortSources_ACustomRuleOutranksTheRestriction` | a custom rule that accepts a source-restricted port reports unknown, not a false "blocked" |
| `TestReachable_PortSources_ASecondRestrictedRuleCanCoverTheCaller` | a second restricted rule that covers the caller reports open, matching the kernel's rule-by-rule evaluation |
| `TestIntegration_Apply_PortSources` | one nft rule per source; an unrestricted port carries no address match at all |
| `TestIntegration_Apply_PortSources_AllCommentsOpensNothing` | a source list with nothing usable in it opens the port to nobody, not to everyone |
| `TestIntegration_ReachableAgreesWithTheKernel` (extended) | the reason returned for a source-restricted port agrees with what a real packet over a veth actually meets |
| `TestIntegration_SSHBruteForceDoesNotOutrankTheBlacklist` | the sshbrute chain returns instead of accepting, so it can no longer outrank the blacklist or open port 22 on its own |
| `TestCatalogueIDsAreUniqueAndStable` | a stored `service` id points at exactly one catalogue entry, forever |
| `TestEveryCataloguePortIsStorable`, `TestEverySuggestionIsKnownAndProducesSources` | every catalogue port and suggested source list passes the same validation the form does, and the suggestion's source slice cannot be mutated by a caller |
| `TestDiffPorts_SourceChangeIsAChange` | restricting an already-open port is a reported change, not an empty preview |
| `TestHandlePortsGET_RendersTheCatalogueForTheTab`, `TestHandlePortsPOST_KeepsSources`, `TestHandlePortsPOST_RejectsAnInvalidSource` | the catalogue rendered server-side is filtered to the tab's protocol, sources round-trip to the core unchanged, and an invalid source is refused with the page re-rendered still holding it |

## The configuration comes from outside

| Test | Protects | What happened without it |
|---|---|---|
| `TestAShippedDefaultDoesNotCountAsStored` | a key present in the shipped `.toml` at its default does **not** beat the environment variable naming it | the naive reading of "stored" is "the key is in the file". `-write-config` emits every default and that file is what the container image ships, so under that reading every environment variable in the product would have been silently dead on exactly the installations that use them |
| `TestEveryEnvVarRoundTripsThroughGetAndSet` | each `EnvVar`'s two closures describe the same field | the comparison that decides "did the operator set this" reads `Get` and the overlay writes `Set`; a pair pointing at different fields would make the precedence rule read one key and write another, with every test green |
| `TestNoEnvVarTargetsAManagedKey` (narrowed) | no variable names `session_key`, `username`, `password`, `totp_secret` or `recovery_codes` | unchanged in force. `telemetry` left the list in 2.12, deliberately — the reason it was there, *press Save and find the old value back after a restart*, is what the precedence inversion removed, and the demo has to report like any other installation |
| `TestEveryManagedKeyIsEitherSecretOrDeliberatelyEnvSettable` | the narrowed list above cannot fall behind `managedKeys` | it is a hand-written subset of a list that grows; a seventh managed key would otherwise be in neither and protected by nothing |
| `TestOnlyTelemetryIsBothManagedAndEnvSettable` | the intersection of `managedKeys` and the keys `shared.WebEnvVars` targets has exactly one member | the two tests above are each a hand-written list checked against the other; moving a key out of the secrets list and into the environment table in the same commit turns both green, each one reading the other's half of the same mistake as the reason it is fine |
| `TestTelemetryFromTheEnvironmentIsNeverWrittenToTheFile` | `mergeConfig` writes the file's own consent value, never the environment's | `render` is handed the live configuration, and after the overlay the live telemetry value is the variable's. One password change would have baked it into `web.toml` as a stored value — which then beats the very variable it came from, permanently |
| `TestEveryEnvKeyIsMarkedOrExplicitlyHasNoControl` | every key a variable names is either marked in the interface or listed with a reason why it has no control | a marker nobody notices is missing is the trap this release exists to remove, one release later |
| `TestTheMarkedTemplatesCallTheMarker` | a template the code claims marks a key actually calls the shared provenance block | the claim and the template can drift independently — `TestEveryEnvKeyIsMarkedOrExplicitlyHasNoControl` only checks that some template is named, not that the named template still renders the marker it is credited with |
| `TestSettingFormsSaveOnSubmitAndCanBeSubmittedWithoutAScript` | a settings form saves on submit, and has a button to submit it with | three pages posted on every `change` *and* carried a Save button that reported a write already done; the fourth, the telemetry toggle, had no button at all, so a script-free operator could move a consent switch and never store it |

## The firewall proves what it says

One incident produced this whole section: an operator's VPS went unreachable after
`docker compose up -d`, and they pasted the ruleset into Discord. For five
releases `ct state established,related accept` carried byte-reversed conntrack
masks, so it matched no packet. The unit test covering that rule had written the
defect down as expected output, and every surface easywall had reported the
firewall active.

| Test | Protects | What happened without it |
|---|---|---|
| `TestCheckRejectsAByteReversedCtStateMask`, `TestCheckAcceptsTheNativeCtStateMask`, `TestCheckRejectsACtStateBitTheKernelDoesNotDefine` | a conntrack mask is compared against the kernel's native-endian value before it reaches netlink | the defect itself, for five releases. `nft list ruleset` printed the rule correctly, because the byte order is invisible in the rendered form |
| `TestCheckKnowsEveryVerdictKindTheLibraryDefines` | every `expr.VerdictKind` the library defines is classified | the switch listed eight of eleven while this release was being planned, which would have reported `stolen`, `repeat` and `stop` as verdicts the kernel does not define — a false positive in a gate that fails the build, and a gate that cries wolf gets switched off |
| `TestCheckNamesEveryJumperToAMissingChain`, `TestCheckReportsTheRulePositionItIsAbout` | a finding names every jump site and the rule index it is about | two chains jumping to one missing target produced a single finding naming whichever was added last, and `Finding.Index` was a plain zero — so a jump at input rule 3 was audited as "input rule 0" and sent the reader to the wrong builder |
| `TestEveryRuleIsAddedThroughTheRecordingAdder` | no function in `nftables.go` reaches `.conn.AddRule` except `builtRecorder.AddRule` | `CheckRules` reads `m.built`, and only the recorder fills it. The review wrote a 2.18-shaped builder the pre-`c4dab40` way — `m.conn.AddRule` with a big-endian `ctStateNew` — and got `ct state 0x8000000` into a live kernel table with `make test` green, the integration gate green and `LastFindings()` empty. The gate's three defences all survive it: `checksRun` is still 2, the `len(m.built)` floor is met by the other rules, and the findings loop never sees the rule. An AST walk, because `nftables.go`'s own comment names `m.conn.AddRule` and a substring guard would match it |
| `TestEstablishedRuleIsTaggedAndCounted` | the established-accept rule carries an `expr.Counter` and a reserved id comment | it carried neither, and `RuleCounters` skips every rule without an id — so the one counter that is a health signal was invisible to every counter easywall reads |
| `TestReservedRuleIDsNeverReachUsage`, `TestReservedRuleIDsWrittenByAnOlderVersionArePruned` | a reserved id is easywall's own accounting and never books as a port rule | dropping `IsReservedRuleID` from `Collect`'s prune loop books `_established` as a port: *Last used* gains a phantom entry that no rules file can ever delete |
| `TestSelftestFailedOutranksUnprovableInEveryOrder`, `TestSelftestUnprovableOnlyWhenNothingSettled` | a disproved claim outranks every claim that could not be settled, whatever the list order | `RunSelftest` returned at the first unsettled claim. Claims 3 and 4 use the open port as their control, so a genuinely broken port rule left them unable to settle — and listing claim 3 first turned a broken firewall into *"this host cannot be asked"*. Two lines away, and the exact inversion this release exists to remove |
| `TestSelftestUsesNoExternalBinary` | no subprocess in the privileged path, in **any** spelling | the guard inspected `exec.Command` and `exec.CommandContext` only. `syscall.Exec`, `syscall.ForkExec`, `os.StartProcess` and an `exec.Cmd` composite literal all start a program without naming either, and all four passed a test whose own comment forbade them |
| `TestHealthAndStatusDisagreeUnderPanic` | `health` exits 2 under panic mode where `status` exits 0 | the divergence is a ruling, not an oversight — a console asking after *intent* is right to be quiet, a monitoring system asking after *health* is not. Unpinned it reads as a bug and gets "fixed" |
| `TestHealthResultCarriesNoRuleDetail` | a sentinel planted in `SelftestStamp.Detail` appears nowhere in the marshalled reply, and `selftest` has exactly four keys | `/healthz` is unauthenticated by necessity and the detail names a port number. The first version forbade the substrings `12227` and `port` — see below |
| `TestEveryHealthStateHasBothLocales`, `TestEveryHealthReasonIsListed`, `TestEveryHealthReasonHasBothLocales`, `TestEverySelftestResultHasBothLocales` | every state, reason and proof result the code can produce is listed and translated in both locales | a reason added to `computeHealth` and not to `AllHealthReasons` reaches a page with no translation, and nothing says so. The reverse direction is a dead locale key |
| `TestTheHealthClassesAreInTheStylesheet` | every class the health rendering asks for is in the **built** stylesheet | `TestEveryTemplateClassIsInTheStylesheet` cannot see the status dot: its regex skips any `class` attribute holding a template action, and the dot is `class="hero-dot {{healthTone …}}"`. A green `npm run build:css` is not proof a rule shipped, and that has cost this repository a release already |
| `TestEverySubcommandIsRunnable`, `TestUsageNamesEverySubcommand`, `TestASubcommandWithNoFunctionIsRefused` | the subcommand table, its usage text and its dispatch are one list | dispatch ended in `default: return runResume(...)`. With three commands that was terse; with five it means a command added to the list and forgotten in the switch silently runs `resume` — which puts the rules back on a machine somebody deliberately unfiltered |
| `TestCommandTimeoutKeepsGetHealthShort` | `GET_HEALTH` stays in `CommandTimeout`'s default branch | it is two netlink reads and one file read, so it queues behind nothing. Moved into the long branch beside `PANIC` it becomes a thirty-five-second poll, invisible to every caller |
| `TestHealthzIgnoresForwardingHeaders`, `TestHealthzIsLoopbackOnlyByDefault`, `TestHealthzAnEmptyListClosesItEvenToLoopback` | `health_allow` is matched against the TCP peer and never a forwarding header | 2.13's `resolveClient` walk exists so the audit log can record who signed in. Reused here, anything behind a trusted proxy reads the endpoint by writing `X-Forwarded-For: 127.0.0.1` |
| `TestCapSysAdminIsGrantedByExactlyOneUnit` | `CAP_SYS_ADMIN` appears in `easywall-selftest.service` and nowhere else | adding it to `easywall-core.service` is a one-line diff that makes the self-test work everywhere and looks like a fix. It would widen the long-lived root daemon that holds the control socket, to buy a check that runs once per upgrade |
| `TestEveryUnitIsInstalledAndManaged` | every `systemd/*.service` is installed by `debian/rules` **and** the Makefile, and enabled, started, stopped and disabled by the maintainer scripts | driven by the directory listing, not a hand-kept list. A unit nothing installs does not exist on a real machine — the same class as the package that shipped with no binaries. The install-verify job did not catch removing a unit from `debian/` either, and now does |
| `TestWatchdogIntervalIsHalfOfWhatSystemdAsksFor`, `TestPingWatchdog_TheFirstFailedTryLockStillPingsAndOnlyTheFirst`, `TestDaemonStart_TheWatchdogStopsOnAWedgeAndResumesAfterIt` | the ping interval is `WatchdogSec/2`, and one contended tick is forgiven rather than latching | the interval is the margin: a ping at t=0 and a skip at t=30 puts the next attempt at t=60+ε against a deadline of exactly t=60, which a Go ticker drifting late loses. Systemd then kills a daemon that answers perfectly well |
| `TestTheCIProofGateIsStillWiredUp` | `EASYWALL_REQUIRE_SELFTEST` is spelled the same in `selftest_required_test.go` and `test.yml` | renaming it in one place disables the gate entirely and nothing notices: the suite goes back to skipping politely and the tick stays green — the exact failure the gate exists to catch, one level further out. Checked from `internal/shared` because a guard behind the `integration` tag is not run by `make test` |

## Four ways a guard is green for the wrong reason

Nine guards in this release were green for the wrong reason. Six had one cause,
and it is a cause this repository manufactures for itself:

> In a repository whose comments name everything they protect, a substring guard
> over a file that contains comments is unreliable by construction. Skip
> comments, and anchor the needle — a quoted literal in Go, `name=` in YAML — or
> the guard passes while the thing it guards is renamed or deleted.

The seventh is the one to remember, because it was inside the helper written to
close the sixth: the helper was itself a substring search, so it survived a
*suffix* rename of the thing it guarded. Two characters fixed it. The generated
usage text is the same shape from the other side — a plain search for `panic` is
satisfied by `resume`'s own description, so deleting the panic line would have
left the guard green.

`TestHealthResultCarriesNoRuleDetail` shows the failure pointing the other way.
It forbade the substrings `12227` and `port`, and `port` is an ordinary word
here: a reason id reading `port_unreachable` would have turned it red for
something that is not a leak, and whoever hit that would have weakened the
assertion to get past it. It asserts the thing itself now — a planted sentinel,
and an exact key set.

**A checker that shares an assumption with the thing it checks.** A substring
guard goes green because the needle is too loose, so it cannot see a *change*.
This one goes green because the checker and the artefact agree, so it cannot see
a *defect*. Different mechanisms, one class: a guard that cannot see what it
exists to see.

> When one function both produces an artefact and verifies it, the verification
> is a tautology.

`scripts/render-changelog.mjs` writes `docs/_docs/changelog.md`, and
`check:changelog` is the same file with `--check`. One version regex therefore
decides both what gets written and whether what was written is right.
`CHANGELOG.md:108` read `## [2.15.1] —## [2.15.1] — 2026-09-07`; the renderer
found no version on that line, the checker found none either, and both agreed
the page was current.

**32 `<details>` sections where there should have been 34.** 2.15.1 had none of
its own and no compare link, and its entry read as part of 2.16.0's — a whole
release that was never on easywall-project.org, with every check green the whole
way. Fixed in 2.17 (`e737dcb`); the sharing is carried, because closing it is a
design decision about that script rather than a broken line.

The sibling is this release's own subject stated about Go: the unit test that
recorded the byte-reversed conntrack mask as expected output. Two sides agreeing
on something wrong is what made both blind.

**A mutation that hangs instead of failing.** Twice this release, and a hang in
CI reads as nothing at all — worse than a skip, which at least prints.

| | |
|---|---|
| `t.Cleanup` runs LIFO | `coreSocket` registered `ln.Close` before the accept-goroutine wait, so the wait ran first and blocked on an `Accept` nothing would return from. Deleting `case "health"` from the dispatch switch made every health test hang instead of fail |
| `t.Fatalf` calls `runtime.Goexit` | a test holding a mutex whose cleanup reached `Stop` deadlocked on any failure. Fixed by moving the locked section into a closure, so `defer` covers the exit |

**A parent test that reports `PASS` over skipped children.** With the CI gate
unset, `TestIntegration_SelftestProvesTheRemainingThreeClaims` printed
`--- PASS` above three `--- SKIP` lines and the package reported `ok`. Reading
the tail of a CI log is not enough; that is what `EASYWALL_REQUIRE_SELFTEST`
turns into a `t.Fatal`.

Three things in the tooling around this release reported success while doing
nothing, which is the same shape as the defect one layer down: `supervisorctl`
did not work in the image at all, `podman build` exits 0 while producing
`HealthCheck: null`, and `make docker` built nothing because the target shared
its name with the `docker/` directory and was missing from `.PHONY`. All three
were found by trying to measure rather than to assert.

## The technical documentation stays unpublished

`TestTheTechnicalDocsAreNotPublished` asserts that `docs-tech/` is outside `docs/`
and that `docs.yml` still builds from `docs/`. Jekyll publishes `docs/` and nothing
else, so this is structural rather than a list of exclusions someone has to
remember to extend.

## Adding one

The shape that works: **derive the list from the code, compare it against the
artefact.** `TestEveryConfigKeyIsDocumented` reflects over the config structs;
`TestEveryPageIsDocumented` parses the router. A test with both sides written by
hand only checks that someone updated two lists at once, which is the thing that
was already not happening.

And watch it fail before trusting it. `TestGoToolchainIsTheSameEverywhere` was
briefly "green" while not executing at all, because the deliberately broken
`toolchain` line used to test it made Go refuse to run in the first place.
