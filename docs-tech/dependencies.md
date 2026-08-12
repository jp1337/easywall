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
toolchain go1.26.5 ← what we build with. Renovate keeps this current
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
| three `customManagers` | `debian/control`, the prose pins with a trailing `+`, and the three pins without one |
| `prBodyNotes` on htmx and mermaid | both need a manual step after merging, and CI fails without it |

The prose manager is anchored on the trailing `+` (`Go 1.26+`) precisely so it
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
