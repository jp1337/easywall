# easywall — the short version

nftables through a web interface that cannot lock you out. Two processes:
`easywall-core` runs as root and speaks netlink; `easywall-web` runs unprivileged
and reaches the core over a Unix socket with a typed JSON protocol. The web process
has no path to the kernel — that is the whole design, and it is why a bug in form
parsing is not a firewall bug.

Every apply reverts itself after 120 seconds unless it is confirmed.

## Where things are

| | |
|---|---|
| `cmd/easywall-core`, `cmd/easywall-web` | the two entry points; both take `-config`, `-write-config` and `-version`, nothing else |
| `config/` | the two commented TOML defaults — the files the package installs, `go:embed`ed by `config/embed.go` so `-write-config` can produce them. The only embed in the tree; no asset is embedded |
| `internal/core/` | daemon, nftables rule building, the acceptance timer, the audit log |
| `internal/web/` | HTTP handlers, sessions, i18n, the demo mock |
| `internal/shared/` | the socket protocol, config structs, validation — and most of the repository's guard tests |
| `web/templates/`, `web/src/`, `web/static/` | Go templates, the Tailwind source, the **committed** build output |
| `locales/*.json` | every visible string. `en` and `de` stay at exact parity; any other language may have gaps — a missing key renders `en` and is counted by the coverage report, never hidden |
| `docs/` | the published site — easywall-project.org. **Only this directory is published** |
| `docs-tech/` | this documentation. For whoever maintains the repository; never published |
| `debian/`, `systemd/`, `docker/` | packaging |
| `.github/workflows/` | six workflows; what each proves is in [ci-and-release](docs-tech/ci-and-release.md) |

## Commands

```bash
make build            # both binaries into bin/
make test lint        # what CI runs first
go test ./internal/...            # unit tests, including the guard tests
sudo go test -tags integration ./internal/core/...   # against a real kernel

npm run build:css     # web/static/style.css      — committed
npm run build:docs-css# docs/assets/css/style.css — committed
npm run build:diagrams# docs/assets/diagrams/     — committed, two SVGs per source
npm run check:ui      # drives the interface in a browser against demo mode
```

## Rules, and what happened without them

Each of these cost a release. They are not style preferences.

| Rule | What it prevented, after the fact |
|---|---|
| **Verify UI by rendering it, not by reading CSS** | A clipped port number, a class that no longer existed, a documentation site with no background — all invisible in the diff, all obvious in a screenshot |
| **Both themes, three widths** (1600 / 900 / 390) | A mobile layout scaled up reads as a website from 2011 |
| **Re-take the screenshots in the same change** | `docs/assets/img/screens/*` is documentation; a stale one describes an interface that no longer exists |
| **A generated file is rebuilt and diffed, never assumed** | Tailwind drops a rule silently and the build stays green — grep the built file |
| **`-ldflags -X` writes to a `var`** | `CurrentVersion` was a `const`, so every release binary reported the literal in the source and the flag reported nothing |
| **Plan larger changes first, and read the code in full** | Keyword-grepping produced confident answers about code that did not say what the grep suggested |
| **A test is verified by breaking the code, not by reading it** | 2.12's reviews found seven tests that passed for the wrong reason — a sample equal to the shipped default, a fixture where the precedence rule carried the test, a key appended past a `[tls]` header into the wrong table, a recomputation with no guard. All seven were green, and all seven were found by mutating the implementation |
| **A fix carries its documentation** | Both locales, the schema, the UI copy — or the next audit finds the mismatch you created |
| **One source for the Go toolchain** | Five places disagreed for months; see [dependencies](docs-tech/dependencies.md) |

## The deeper documentation

| | |
|---|---|
| [ci-and-release](docs-tech/ci-and-release.md) | the six workflows, what each one actually proves, and the path from a tag to the release assets |
| [packaging](docs-tech/packaging.md) | `debian/`, the trap that shipped a package with no binaries, the permission layout, the capability story |
| [dependencies](docs-tech/dependencies.md) | Renovate's configuration rule by rule, and why the `go` directive is left alone |
| [invariants](docs-tech/invariants.md) | every guard test, what it protects, and the incident that produced it |
| [protocol](docs-tech/protocol.md) | the socket protocol: twenty commands, and the one field that is not typed |
| [threat-model](docs-tech/threat-model.md) | session internals, the deliberate refusal of `X-Forwarded-For`, what is not defended |

Contributor-facing rules — commit format, review checklist, adding a language —
are in [CONTRIBUTING.md](CONTRIBUTING.md), and the design system is
[DESIGN.md](DESIGN.md). Both are for humans sending a pull request; the pages
above are for whoever has to keep the machinery working.
