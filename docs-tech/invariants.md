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

## One source for a version

| Test | Protects |
|---|---|
| `TestGoToolchainIsTheSameEverywhere` | `go.mod`'s `toolchain` line, the Dockerfile tag, `debian/control` and six prose pins agree — and no workflow spells a version out |
| `TestTheCSRFClaimNamesTheReleaseItArrivedIn` | the five sentences saying `CrossOriginProtection` arrived in Go 1.25 are **not** treated as version pins |
| `TestRenovateEditsOnlyTheGoPinsItShould` | Renovate's own regexes, run against the tree, capture only the toolchain |

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
| `TestDocsStylesheetKeepsLoadBearingRules`, `…CodeBlockHasASingleFrame`, `…InlineCodeIsNotThemeScoped` | the documentation site's stylesheet after a Tailwind rebuild |
| `TestVersionedStaticAssetsCarryTheReleaseInTheirURL`, `TestStaticFilesSayHowLongTheyMayBeKept` | an upgrade actually changes the stylesheet URL |

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
