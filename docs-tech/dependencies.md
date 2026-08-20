# Dependencies and the Go toolchain

Renovate raises every update pull request. There is no `dependabot.yml` — GitHub's
"Dependabot security updates" setting is switched off in favour of
`vulnerabilityAlerts` here, so one tool raises the pull request rather than two
raising it for the same advisory. The alerts themselves stay on; they are what
Renovate reads.

## Why the migration happened

The Go toolchain was written down in five places and they had drifted apart:

| Where | Said |
|---|---|
| `go.mod` | `go 1.25.0` |
| ten `go-version:` pins across four workflows | `1.25` |
| `Dockerfile` | `golang:1.26-alpine` |
| `debian/control` | `golang-go (>= 1.21)` |
| four documentation pins | `1.25+` |

So the published container was compiled by a Go version no test ever ran against.

The cause was mechanical rather than careless: **Dependabot understands a Docker
tag and none of the other four.** The `go` directive
([dependabot-core#9527](https://github.com/dependabot/dependabot-core/issues/9527))
and the `toolchain` directive
([#13520](https://github.com/dependabot/dependabot-core/issues/13520)) are open,
untriaged feature requests, and there is no regex manager for a workflow input.
The one place it could reach walked off on its own.

## The single source

**`go.mod`'s `toolchain` line.** `actions/setup-go` reads it in preference to the
`go` directive, so no workflow spells a version out — they all say
`go-version-file: go.mod`. Everything else follows it through the custom managers
below, in one pull request.

```
go 1.25.0          ← the oldest Go this code compiles with. Renovate does not touch it
toolchain go1.X.Y  ← what we build with. Renovate keeps this current
```

The two are allowed to differ and move for different reasons. The `go` directive
is a claim about the *source* — it changes when the code starts using a newer API,
which is what took it to 1.25 for `http.NewCrossOriginProtection`. Raising it costs
every consumer, and Go's own guidance is to leave it alone. `renovate.json` states
that explicitly with `enabled: false` on the `golang` depType, so nobody adds
`rangeStrategy: bump` later without meeting the reason.

## renovate.json, rule by rule

| Piece | What it is for |
|---|---|
| `extends` | `config:recommended`, `:dependencyDashboard`, `:semanticCommits` — the house style shared with the other repositories |
| `schedule` | `after 2am and before 5am`, Europe/Berlin. `vulnerabilityAlerts` overrides it with `at any time` |
| patch / pin / digest | auto-merged once CI is green |
| minor and major | wait for a person. This is a firewall; the blast radius of a bad dependency is the host's packet filter |
| `groupName: "Go toolchain"` | every place the toolchain is written moves in **one** pull request, and never auto-merges whatever the update type says |
| three `customManagers` | `debian/control`, the prose pins with a trailing `+`, and the four pins without one. All three use `versioning: docker`, for the reason below |
| `prBodyNotes` on htmx and mermaid | both need a manual step after merging, and CI fails without it |

The prose manager is anchored on the trailing `+` (`Go 1.X+`) precisely so it
cannot touch the five sentences about `net/http.CrossOriginProtection`. Those say
which release the API *arrived* in, and that stays 1.25 for ever.
`TestTheCSRFClaimNamesTheReleaseItArrivedIn` states which sentences are off limits.

### A validating config is not a correct config

Renovate's own configuration had two false matches on its first run, and neither is
visible to `renovate-config-validator`:

- `README.md` carried a comment saying badges "cannot drift the way a hand-written
  Go 1.25+ could" — a sentence warning about a stale pin, with a stale pin in it.
  The prose manager matched it, so Renovate had found a dependency that would be
  out of date for ever.
- `debian/control` repeated its own `Build-Depends` line verbatim in the comment
  above it, so the same file matched twice.

`TestRenovateEditsOnlyTheGoPinsItShould` parses `renovate.json`, applies its own
regexes to every tracked file, and requires each captured value to be the current
toolchain. Anything else is either a pin left behind or a sentence Renovate would
edit into a lie.

### A detected dependency is not an updated dependency

The arrangement above went in with 1.26.6 already in every file, so nothing
exercised it. The first real bump — 1.26.6 → 1.27.0 — moved `go.mod` and the
`Dockerfile` and left **every one of the seven derived pins behind**, and the pull
request looked healthy: the dependency dashboard listed six regex dependencies,
none of them carried a `skipReason`, and no warning appeared anywhere. Two
separate faults, neither visible without reading a Renovate debug log:

**`versioning: npm` on a two-component pin.** The managers captured `1.26` and
`extractVersion` reduced the datasource's `1.27.0` to `1.27`. Neither is a valid
semver version, so npm versioning treated the current value as a range with no
satisfying release and returned `"updates": []` — a found dependency with an
empty candidate set, which reads exactly like an up-to-date one. `docker`
versioning treats a truncated version as a version, and is what these use now.
A local `renovate --platform=local --dry-run=lookup` is the way to see this: it
prints each dependency's `updates` array, and the fix took the run from four
flattened updates to ten.

**A file pattern aimed at a page that had moved.** The Jekyll restructure put the
manual-installation page under `docs/_docs/`, and left a four-line `redirect_to`
stub at the old path. `renovate.json` still named the old path, so Renovate read
the stub, found no version in it, and reported nothing — `No dependencies found
in file for custom regex manager`, at debug level, in a log nobody reads.

The second fault is the one worth generalising: **a manager that reaches nothing
is indistinguishable from a manager that works.** `TestRenovateEditsOnlyTheGoPinsItShould`
could not catch it, because it validates the values a pattern captures
and a dead pattern captures none. `TestEveryRenovateFilePatternReachesAPin` states
the other half — every `managerFilePatterns` entry has to match a tracked file,
and those files have to contain at least one of that manager's `matchStrings`.

While fixing it, an eighth pin turned up on the documentation landing page —
"Go 1.26, no runtime dependencies beyond the kernel" in the *from source* card,
one card below the `<strong>` badge that *was* managed. No manager and no test
knew about it. It has both now.

## Manual steps some updates need

| Dependency | After merging |
|---|---|
| `htmx.org` | `cp node_modules/htmx.org/dist/htmx.min.js web/static/htmx.min.js` — the served copy is committed and the `assets` job diffs it |
| `mermaid` | `npm run build:diagrams` — the version is folded into each diagram's digest, so an upgrade makes every SVG stale by design |
| the Go toolchain | `npm run build:diagrams`, because `docs/_diagrams/install-choice.mmd` names the Go version |

## Writing a version down anywhere new

Don't, unless you also add it to `renovate.json` **and** to
`TestGoToolchainIsTheSameEverywhere`. A pin no tool knows about is the exact defect
this whole arrangement exists to prevent. That is why nothing in `docs-tech/`
quotes a version number.
