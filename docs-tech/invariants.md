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

## The preview reports everything it can change

| Test | Protects | What it would have shipped |
|---|---|---|
| `TestDiffRulesReachesEveryRuleSet` | every field of `shared.Rules` is reached by `DiffRules` | a seventh rule set the apply screen silently omits: the operator reads "what changes", sees six sections, and the seventh applies anyway |
| `TestDiffConfigReachesEveryOption` | every leaf of `FirewallOptions` and `NetworkSettings` is reached by `DiffConfig`, or is named in `skippedConfigKeys` with a reason | the same defect one struct over, and the one that produced this release: option changes were in no pending calculation at all, so `/options` said "apply to activate" while `/apply` said there was nothing to apply |
| `TestIntegration_ReachableAgreesWithTheKernel` | `shared.Reachable`'s verdict equals what a real packet from a real source address meets | the chain order is duplicated between `nft.Apply` and `Reachable` by construction. Without a real packet the lockout warning is an assertion about nftables written in Go, and the two have disagreed before — that is what `nftables_semantics_test.go` exists for |
| `TestCoreWritesItsFilesForRootOnly` (extended) | `applied-config.json` is 0600, beside the audit log and the last-apply marker it now shares the assertion with | it holds the machine's whole firewall configuration and only the core reads it; the web process asks over the socket |
| `TestLogEventPayloadCarriesNoFreeText` (extended) | `LogEventPayload.Proxied` stays a `bool` | true only when resolution fell back to the peer, false when it named a client; a string here would be a way for the web process to write arbitrary text into the core's own log through a field that looks like a flag |

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
| `TestEveryKernelWriteIsFollowedByThePanicCheck` | every `f.nft.Apply` in `firewall.go` and `restore.go` is followed by a `panicLandedDuringWrite`, two of them per function because `nft.Apply` reports errors from work that runs after the ruleset is committed | the check was a *missing call* to begin with, and nothing noticed one going away: deleting the one in `apply` left the whole suite green. The nothing-went-unparsed half compares the per-function tally with the file-wide count, so a fourth writer of the table fails the test instead of silently escaping it |
| `TestRunSubcommand_NoDaemonFallbackNamesItselfInTheAuditLog` | the console fallback writes `console-no-daemon` as the audit user | `user` renders literally in the log table — no label map, no view function — so a rename would have left two spellings for the same event with every test still green |

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
