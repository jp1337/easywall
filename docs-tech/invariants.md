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
| `TestDocsVersionMatchesRelease` | `docs/_config.yml` `version:` equals `shared.CurrentVersion` | the sidebar badge was hardcoded `v2.4` and drifted a patch release behind |
| `TestEveryEnvVarIsDocumented` | every `shared.CoreEnvVars`/`shared.WebEnvVars` name appears in `docs/_docs/environment.md`, and the page names nothing else | the operator complaint that started the environment-variable feature was "there is no list" — a page that drifts from the code recreates exactly that |
| `TestTheEnvironmentPageIsInTheNav` | `docs/_config.yml`'s `nav:` links to `/docs/environment/` | a page reachable only by its URL is, for a page whose whole point is being findable, the same as not adding it |

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
| `TestGermanTranslationsAreNotCopiedEnglish` | a German value is not the English one pasted across |
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
| `TestNoEnvVarTargetsAManagedKey` | no `shared.WebEnvVars` entry names one of `web`'s `managedKeys` — the six the interface writes back through `mergeConfig` | four of those keys are secrets (credentials, session key, TOTP secret, recovery codes), and an environment variable is visible in `docker inspect` and `/proc/<pid>/environ` |
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
