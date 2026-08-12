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
| `TestCoreWritesItsFilesForRootOnly` | the audit log and the last-apply marker are `0600` | |
| `TestShippedConfigsMatchTheStructsTheyConfigure` | `config/*.toml` — what the package installs — still parses | `config/easywall.toml` shipped the obsolete `ipv6.enabled` a release after `mode` replaced it |
| `TestNoPersonalEmailAddressesAreTracked` | no personal address in any tracked file | one was in `debian/control` and two changelog sign-offs, in a public repository, for four months |

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
