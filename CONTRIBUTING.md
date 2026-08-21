# Contributing to easywall

Thank you for your interest in contributing! This document explains how to get
started and what we expect from contributions.

## Development Setup

```bash
git clone https://github.com/jp1337/easywall.git
cd easywall

# Install Go 1.27+  (the toolchain go.mod pins; workflows read it from there)
# https://go.dev/dl/

# Download dependencies
go mod download

# Build
make build

# Run tests
make test

# Lint (requires golangci-lint)
make lint
```

## Commit Messages

We use **Conventional Commits** to enable automatic changelog generation:

```
<type>(<scope>): <short description>

[optional body]
[optional footer]
```

### Types

| Type | When to use |
|---|---|
| `feat` | New feature |
| `fix` | Bug fix |
| `security` | Security fix or improvement |
| `docs` | Documentation only |
| `chore` | Build, CI, dependencies |
| `refactor` | Code restructuring, no behaviour change |
| `test` | Adding or fixing tests |

### Examples

```
feat(core): add TCP RST flood protection
fix(web): correct flash message key for import errors
security(auth): increase argon2id memory parameter
docs(docker): clarify coexistence setup options
chore(deps): update golang.org/x/crypto to latest
```

Breaking changes: add `BREAKING CHANGE:` in the footer.

## Pull Request Process

1. Fork the repository and create a feature branch:
   ```bash
   git checkout -b feat/my-feature
   ```

2. Write tests for new functionality. Coverage must not drop below **80%**.

3. Run the full test suite: `make test lint vuln`

4. Open a pull request against `main`. The PR description should explain
   the motivation, what changed, and how to test it.

5. Wait for CI to pass and a maintainer review.

## Adding a Language

1. Copy `locales/en.json` to `locales/<lang>.json`
2. Translate all values (keep the `id` fields unchanged)
3. Translate `language_name` into that language's **own** name — `Deutsch`, not
   `German`; `Français`, not `French`. It is what the switch in the sidebar
   shows, whatever language the interface is currently in.
4. Add an entry to `locales/status.json`: `"<lang>": { "reviewed": false }`
5. Open a PR with the subject: `feat(i18n): add <language> translation`

Nothing else is needed to make it appear: the switch is built from whatever
`locales/*.json` contains.

**A partial translation is welcome.** Only `en` and `de` are held at exact
parity — `StrictLangs` in `internal/web/locales.go`. Every other language may
have gaps: a key you leave out renders the English string, and the gap is
*reported* rather than hidden, because a gap nobody can see is
indistinguishable from no gap. `go test ./internal/web/ -run TestLocaleCoverage`
prints the percentage per language. Two things it deliberately does not count:
an empty string, and a value byte-identical to the English one — otherwise the
number could be raised by pasting.

**`reviewed: false` is not an insult.** It is the difference between "somebody
wrote this" and "somebody who speaks it has read it", and the switch and the
coverage report both say which. `fr.json` shipped this way. Send a second PR
flipping the flag once you have read it end to end — that is a review, and it is
worth as much as the translation.

**Read [`docs-tech/i18n-review.md`](docs-tech/i18n-review.md) first.** It
collects the thirty-odd sentences where a wrong word changes what the firewall
promises — which list is consulted first, what the acceptance window undertakes,
what panic mode does not end. The rule for those: you may rephrase freely, but
you may not change what the sentence *claims*. A window that "keeps" a change in
one language and "undoes" it in another describes a different product depending
on which language you read.

Three forms appear inside translations, and all three must survive into your
language:

| In the message | Renders as | Note |
|---|---|---|
| `` `443` `` | `<code>443</code>` | literals: ports, CIDRs, nftables statements |
| `*before*` | `<em>before</em>` | emphasis that carries meaning, not decoration |
| `{}` | a link | one per link, **in your language's word order** |

The `{}` slots are filled in the order the template passes them, but you decide
where they sit in the sentence — German puts "Die Sperrliste wird zuerst
geprüft" with the link first where English has it third. `{{.Name}}` placeholders
are values the page substitutes and must be kept verbatim.

## Code Style

- Follow standard Go conventions (`gofmt`, `golangci-lint`)
- No comments explaining *what* code does — only *why* when non-obvious
- No half-finished features behind flags — implement completely or not at all

## Styling

The interface follows [`DESIGN.md`](DESIGN.md) — one source of truth for colour,
typography, spacing, radii, motion and components. Read it before changing anything
visual. There is no third-party component library: tokens are declared once in the
`@theme` block of `web/src/app.css` and Tailwind generates the utilities, so a template
names `bg-surface`, never a colour.

| Rule | Why |
|---|---|
| **Never write a colour into a template** | Use the tokens — `bg-surface`, `text-ink-muted`, `border-rule` |
| **Green, amber and red mean firewall state** | Live, unconfirmed, rolled back. Never decorative. There is no informational colour |
| **The accent is rationed** | What is focused, what is active, the one primary action on a page |
| **Controls use `control-edge`, containers use `rule`** | WCAG 2.1 SC 1.4.11 wants 3:1 for anything you can operate, and a field's fill sits ~1.05:1 from the panel behind it. Hover goes to `ink-subtle` — `rule-strong` is *weaker* than `control-edge` and would fade the outline you just pointed at |
| **Both themes work** | Check light as well as dark before opening a PR |
| **Sentence case, in Inter** | The tracked uppercase mono `label` role survives only in the sidebar's two nav dividers |
| **Every table reflows** | Below 720px rows become labelled cards, which works only if every `<td>` carries a `data-label`. Rows built in `app.js` read the label out of the `<thead>` so it stays translated |
| **One heading per thing** | A page title followed by a card titled the same is the duplicate-heading bug |
| **Every visible string goes through `T`** | Into *both* `locales/en.json` and `locales/de.json`, `placeholder`, `aria-label` and `title` included — those two only, since they are the pair held at parity; a further language renders English until somebody translates the key. A sentence with a link or a `code` span stays *one* message and uses `richText`. Text `app.js` builds needs its key in `clientStringKeys` |
| **New component? `DESIGN.md` first** | Then implement it from those tokens |

**Verify by rendering, not by reading the CSS.** Every significant defect in this
interface was invisible in the stylesheet and obvious in a screenshot — a clipped port
number, a class that no longer existed, an arrow that wrapped onto its own line, a
documentation site with no background at all. Load the pages you touched, in both
themes, at a phone width as well as a desktop one.

Rebuild the committed assets after changing a source:

```bash
npm run build:css        # web/static/style.css      — the application
npm run build:docs-css   # docs/assets/css/style.css — the documentation site
npm run build:diagrams   # docs/assets/diagrams/     — one SVG per theme
```

`npm run check:diagrams` fails if a `.mmd` source changed without a re-render.
`npx @google/design.md lint DESIGN.md` validates the design system itself; some
warnings are expected and are explained inside the file.

## Documentation

There are two kinds, and they are kept apart on purpose.

| | For | Where |
|---|---|---|
| **User documentation** | whoever runs easywall | `docs/` — published as easywall-project.org |
| **Technical documentation** | whoever maintains this repository | [`docs-tech/`](docs-tech/) and [`CLAUDE.md`](CLAUDE.md) — **not** published |

`docs/` is the entire Jekyll source, so nothing outside it can reach the site. That
is what keeps the second kind unpublished, rather than a list of exclusions someone
has to remember to extend, and `TestTheTechnicalDocsAreNotPublished` holds it.

**User pages: maximum information, minimum text.** Reach for a diagram before a
paragraph, a table before a list of sentences, and a screenshot before a description
of a screen. A thorough page nobody finishes is worth less than a short one that gets
read. Diagrams are `.mmd` sources in `docs/_diagrams/`, rendered to committed SVGs —
see the [README](docs/_diagrams/README.md) there for how to reference one.

**Technical pages: the rule *and* the incident behind it.** Without the incident a
rule gets optimised away at the next rewrite — which is how the Debian package came
to ship with no binaries in it. Never write a version number there; nothing updates
it, and `go.mod` is one file away.

A new page in the interface needs a page in `docs/`:
`TestEveryPageIsDocumented` derives the list from the router and fails on a route it
has never heard of.

## Dependencies and the Go toolchain

Renovate raises the update pull requests; there is no `dependabot.yml`. Patch updates
merge themselves once CI is green, minor and major ones wait for a person.

**The Go toolchain lives in exactly one place: the `toolchain` line in `go.mod`.**
`actions/setup-go` reads it in preference to the `go` directive, so no workflow spells a
version out — they all say `go-version-file: go.mod`. Four other places quote that
version (the `Dockerfile` tag, `debian/control`, the install instructions, the
install-choice diagram) and Renovate moves all of them in one pull request.

`TestGoToolchainIsTheSameEverywhere` fails if any of them disagree. It exists because
they did: the Dockerfile spent months on Go 1.26 while every test ran on 1.25, and the
only tool watching could read a Docker tag and nothing else.

The `go` directive is a different thing and moves for a different reason — it is the
oldest Go this code compiles with, so it changes when the code starts using a newer API,
not when a new Go appears.

## Security Issues

Please see [SECURITY.md](SECURITY.md) — do not open public issues for
security vulnerabilities.
