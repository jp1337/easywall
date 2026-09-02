# CI and release

Six workflows. The question worth asking about each is not *does it pass* but
*what would it catch* — this repository has shipped a package with no executables,
a container built by an untested compiler, and a version string the linker never
wrote, all behind green ticks.

| Workflow | Runs on | What it proves |
|---|---|---|
| `test.yml` | push to `main`/`develop`, every PR | unit tests with `-race`, an integration suite against a real kernel, that the committed assets match their sources, and that the interface works in a browser |
| `build.yml` | push, every PR | both binaries cross-compile, the image builds, and the `.deb` **installs, starts and answers** on both architectures |
| `security.yml` | push to `main`, every PR, Mondays 08:00 UTC | CodeQL, `govulncheck`, `gosec` |
| `docs.yml` | changes under `docs/` | the site builds (PR) and deploys to Pages (main) |
| `publish-edge.yml` | push to `main` | multi-arch image to three registries as `:edge` and `:sha-…` |
| `release.yml` | a `v*.*.*` tag | GoReleaser, then one `.deb` per architecture uploaded as a release asset, then one Discord embed |

Pull-request runs cancel their predecessors (`concurrency` with
`cancel-in-progress` gated on `github.event_name == 'pull_request'`). Deliberately
not on `main`: a cancelled run there could skip publishing an edge image, and the
point of a push to `main` is that it completes.

Runner minutes are free — standard *and* arm64 runners are unmetered for public
repositories. The 2,000-minute quota applies to private ones. Wall-clock is worth
optimising; cost is not.

## What each job is actually for

### `test.yml`

| Job | Note |
|---|---|
| `test` | `-race`, coverage uploaded to Codecov. The profile is checked non-empty by a step that *can* fail, because the upload step cannot (`continue-on-error`) |
| `test-integration` | `sudo go test -tags integration` over `./internal/core/...` and, since 2.13, `./internal/web/...` — `-timeout 300s` for both packages together. Namespaced via `CLONE_NEWNET` in `TestMain`; the only place `nftables.go`'s rule builders execute (without this job they measure 0%) and the only place the trusted-proxy resolution sees a kernel-assigned peer rather than one a test wrote into a struct |
| `assets` | rebuilds both stylesheets and diffs them, runs `check:diagrams`, and re-copies `htmx.min.js` from `node_modules` and diffs that. Three committed artefacts, three freshness checks |
| `ui` | Playwright against demo mode. Catches what no Go assertion can: the forwarding editor storing `1E+04` as port 1, a console error, a sideways scroll, in both themes |
| `lint` | `golangci-lint` |

The integration coverage profile is written under `sudo` and then `chown`ed back —
a profile the uploader cannot read is an upload that silently does not happen.

### `build.yml`

`build-binaries` cross-compiles for `linux/amd64` and `linux/arm64` and asks the
amd64 binary for its `--version`, because `-ldflags -X` writes to a variable and
does nothing at all when handed a constant.

`build-deb` is a matrix of two **native** runners:

```yaml
- arch: amd64
  runner: ubuntu-24.04
- arch: arm64
  runner: ubuntu-24.04-arm
```

Nothing is cross-built: `debian/control` says `Architecture: any` and
`debian/rules` calls a plain `go build`, so the runner decides. That is what makes
the rest of the job possible — it installs the package, checks the ownership of
every path, starts both services, connects to the socket **as the `easywall`
user**, fetches `/firstrun` over HTTPS, and compares `--version` against the
package version. A cross-built package could do none of that.

Two checks earn their place specifically:

- `dpkg-deb --field … Architecture` **and** the ELF machine type of the binary
  inside it. A package named `arm64` carrying amd64 binaries is the one mistake a
  file name cannot show.
- Several steps need `sudo` to read paths that are deliberately closed
  (`/etc/easywall` is `0750 root:easywall`). A plain `stat` there reports
  "Permission denied" for the arrangement working exactly as intended.

No npm in the package jobs: `web/static/style.css` is committed and the `assets`
job already fails if it does not match its source. Rebuilding it here would also
drag `@tailwindcss/oxide` for arm64 in for nothing.

### `security.yml`

`setup-go` runs **before** `codeql-action/init`, and the order is load-bearing:
`init` puts a wrapper `go` on `PATH` to trace the build, and a later `setup-go`
prepends the real toolchain so the wrapper is never called. CodeQL reports that as
a warning, not a failure — so the job stayed green while the analysis was not
seeing the build it was pointed at.

`gosec` runs with `-tests -tags integration`. `-tests` is what makes `-tags` do
anything: everything behind the integration tag is a `_test.go` file, and gosec
skips test files unless asked. Measured: 41 files either way without it, 148 with.
Test-file findings are then filtered out of the SARIF with `jq` — they are printed
in the log first, so the number is visible if anyone ever wants to act on them.

### `docs.yml`

| Job | Note |
|---|---|
| `build` | pull requests only; permissions are read-only, so it cannot reach Pages even by mistake |
| `deploy` | push to `main` only; the same build, then `actions/deploy-pages` |

Both run the identical `bundle exec jekyll build` with `JEKYLL_ENV=production` — the
same command and environment as what ships, or the pull-request job would prove
something about a build nobody ships.

#### The search index

Pagefind runs in both jobs — in `deploy` because that is what ships, in `build`
because a pull request that breaks indexing has to fail on the pull request. Both
call the same composite action, `.github/actions/build-search-index/action.yml` —
the repository's first, chosen over two fifteen-line copies of the same steps that
could drift apart, and named in both of `docs.yml`'s `paths:` filters, or a pull
request that edits it runs nothing.

It sits **after** the "site is not empty" check and **before** the Pages upload.
After, because Pagefind reports an empty site as `Found 0 files matching` and
loses the crafted `no landing page was built` diagnostic; before, because the
upload is what publishes, and an index built afterwards is not in it.
`TestTheSearchIndexIsBuiltBeforeThePagesUpload` holds both jobs to that.

It is **not** a committed build output, unlike `web/static/style.css`,
`docs/assets/css/style.css` and the diagrams. It is derived from `_site`, which is
not in the repository, so there is nothing to diff a commit against; the assertion
below is its only protection.

A fourth step checks that the twelve Pagefind class names `web/src/docs.css`
overrides still exist in the shipped `pagefind-ui.css`. The overlay's height,
scroll and excerpt typography cannot be reached through Pagefind's custom
properties, so they are styled by class name — the same trade Astro Starlight
makes with the same engine. A rename would ship the search in Pagefind's own
defaults with every Go guard still green, because those read our stylesheet and
not theirs.

Three flags, each preventing something that was actually observed:

| Flag | Without it |
|---|---|
| `--glob "docs/**/*.html"` | the marketing landing page competes with the documentation — it ranked for `argon2id` and for `port forwarding` — and the 23 old-path redirect stubs get indexed as pages |
| `--root-selector "main.content"` | the topbar's text is indexed on all 26 pages at once. `<nav>` and `<footer>` are already excluded by default, so this is about everything else outside the content column |
| `--force-language en` | the language is inferred per file. It was inferred as two, and a search in one index could not see the other half of the site |

**The version is pinned exactly**, not `pagefind@1`. The pin itself lives in
`.github/actions/build-search-index/action.yml`; Renovate's `pagefind`
customManager in `renovate.json` is what keeps that line current — see
[dependencies](dependencies.md). A floating range built the published index from
whatever 1.x npm resolved that morning, and the loader depends on the bundle's *JS
API* as well as on the index format: the `window.PagefindUI` global, five option
names, and `pagefind-highlight.js` being an ES module. The count assertion below
counts fragments and can see none of that.

A glob matching nothing fails the step outright: Pagefind 1.5.2 exits 1, measured
rather than assumed. What the count check afterwards catches instead is a glob
that matches *some* pages but not all — a narrowed `--glob "docs/features/**/*.html"`
indexed 13 of 26 without the build failing.

**A limitation, not a defect.** `open a port` finds the ports page (rank 3, with a
sub-result excerpted from "When it does not work"). `how do I open a port` finds
nothing there: Pagefind ANDs every query term, and `how` and `I` occur nowhere on
`docs/_docs/features/ports.md`. No weighting was added — the spec rules it out —
and no page's wording was changed to work around it. A question-shaped query
finding nothing is the engine behaving as built, not a bug to fix.

### `release.yml`

```
tag v*.*.*
   ├── goreleaser ──► tarballs (amd64, arm64), checksums, images :latest/:vX.Y.Z
   ├── debian ──────► easywall_amd64.deb   (native amd64 runner)
   │                  easywall_arm64.deb   (native arm64 runner)
   └── announce ────► one embed in Discord   (needs both, best-effort)
```

The `.deb` is built with `dpkg-buildpackage`, deliberately **not** GoReleaser's
`nfpm`. `debian/` is the one description of this package — permissions, postinst,
units, logrotate — and `build.yml` installs *that* package on every pull request. A
second definition in `.goreleaser.yaml` would be a second thing to keep in step,
and the release would ship the copy nobody tests. Two definitions of one artefact
is how a package comes to contain no binaries.

Before uploading, each leg checks that the artefact contains both binaries, that
its version matches the tag, and that its architecture and ELF type match the leg
that built it. The asset names are stable — `easywall_amd64.deb`,
`easywall_arm64.deb` — because the documented install command asks for exactly
those under `/releases/latest/download/`.

That command had always been a 404: no release before 2.5.0 carried a `.deb` at
all. It was built by CI and kept as a seven-day artefact that needed a GitHub
login to fetch.

#### The `announce` job

2.8.0 shipped and was announced by hand, hours later. An announcement that
depends on somebody remembering is one that is eventually forgotten, and a
release nobody hears about is most of a release wasted.

It posts one embed — the version, a link to the release notes, and the sentence
about the acceptance window — to the webhook in the repository secret
`DISCORD_WEBHOOK`. Three things about how it is written are deliberate:

| | |
|---|---|
| `continue-on-error: true` | It runs *after* `goreleaser` and `debian`, so the release is complete and its assets are uploaded before this starts. A Discord outage is a workflow warning, not a red release |
| The secret is mapped to job-level `env` | The `secrets` context is **not** available in a step's `if:`; only `env` is. `if: ${{ secrets.DISCORD_WEBHOOK != '' }}` is silently always false, so the job would never post — exactly the forgotten announcement it exists to prevent. The step tests `env.WEBHOOK` instead, which also means a fork with no webhook skips rather than fails |
| `jq -n --arg` builds the payload | Nothing interpolated can break out of the JSON. Shell string concatenation here would make the tag name an injection point |

The `jq` construction is the part that can actually be wrong, and it cannot be
exercised without cutting a release, so check it by hand instead — see Step 3 of
the 2.9 implementation plan, which runs the same `jq -n` with `TAG=v2.9.0` and
asserts the title and URL.

**The Ko-fi post is not here, and cannot be.** Ko-fi has no writing API: posts
are made through the browser while signed in, so that step is a person's, not a
workflow's. Documenting it here as automated would send whoever reads this next
looking for a job that does not exist.

### `publish-edge.yml`

Native build per platform, pushed **by digest** with no tag; a merge job then
creates the manifest in all three registries. Cross-building with QEMU inside one
buildx invocation is dramatically slower.

GHCR must succeed, Docker Hub must succeed if its token is configured, Quay is
best-effort and downgrades to a workflow warning.

The `deploy-demo` job runs on a **self-hosted** runner inside an intranet. The
workflow triggers on `push: branches: [main]` only. Never add `pull_request` or
`pull_request_target` to it, or to any job running on `self-hosted` — that turns
any fork's pull request into arbitrary code execution on that runner. It is gated
behind the repository variable `DEPLOY_DEMO`, because a job whose labels no runner
answers does not fail: it queues for 24 hours and reports as cancelled.

## What protects `main`

Set 2026-09-01, after a red `Test` run reached `main` and nothing stopped it.

| | |
|---|---|
| Required status checks | the thirteen below, `strict: false` |
| Enforced for administrators | **yes** — the rules apply to the owner too |
| Force pushes, deletions | refused |
| Required reviews | none; this is a one-maintainer repository and a self-approval is theatre |

```
Test (ubuntu-24.04)     Lint     Integration tests (nftables / CAP_NET_ADMIN)
Generated assets are current     Interface in a browser
CodeQL     gosec     govulncheck
Binaries (linux/amd64)     Binaries (linux/arm64)
Debian Package (amd64)     Debian Package (arm64)     Docker Image
```

**`Site builds` and `Deploy to Pages` are deliberately absent.** `docs.yml` is
path-filtered, so on a pull request that touches no `docs/**` file those checks
never report at all — and a required check that never reports leaves the pull
request blocked forever rather than failing it. The same reasoning excludes
`publish-edge.yml`'s jobs, which run on `main` and never on a pull request.
`codecov/patch` and `codecov/project` are left informational: a change that
legitimately lowers coverage should be arguable, not refused.

`strict: false` on purpose. Requiring a branch to be up to date before merging
costs a full re-run of all thirteen on every merge, and Renovate rebases its
branches whenever `main` moves.

**What this does not do.** It would not have caught the incident that prompted
it. The pull request's own run was green and the `main` run of the same tree was
red — a flake in `TestDaemonStart_RestoresAtStartup`, fixed by `startTestDaemon`
and guarded by `TestDaemonTests_StartInAGoroutineIsAlwaysWaitedFor`. Required
checks stop a *knowingly* red merge. Noticing a red `main` is a separate problem
and is still unsolved.

## Cutting a release

The release commit goes through a pull request like everything else. Direct
pushes to `main` are refused since the protection above, for the owner as well —
a brand-new commit has no passing checks, which is exactly what the rule says.

1. On a branch: `CHANGELOG.md`, move `[Unreleased]` to the new version with
   today's date.
2. `debian/changelog`: a new entry — `debian/rules` takes the package version from
   it, and it once sat at 2.0.0 for five releases.
3. `internal/shared/version.go` and `docs/_config.yml`'s `version:` —
   `TestDocsVersionMatchesRelease` fails if they disagree.
4. Open the pull request, let the thirteen run, merge it.
5. Tag `vX.Y.Z` on the resulting `main` commit and push the tag. Tags are not
   branches; the protection does not apply to them.
6. Read the release run's log rather than the tick, and download one `.deb` to
   confirm it contains what it should.
