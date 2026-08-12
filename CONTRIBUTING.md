# Contributing to easywall

Thank you for your interest in contributing! This document explains how to get
started and what we expect from contributions.

## Development Setup

```bash
git clone https://github.com/jp1337/easywall.git
cd easywall

# Install Go 1.26+  (the toolchain go.mod pins; workflows read it from there)
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
   `German`. It is what the switch in the sidebar shows, whatever language the
   interface is currently in.
4. Open a PR with the subject: `feat(i18n): add <language> translation`

Nothing else is needed: the switch is built from whatever `locales/*.json`
contains. `TestLocaleFilesAreAtParity` and `TestTemplatesOnlyUseTranslatedKeys`
will tell you about anything you missed.

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
| **Every visible string goes through `T`** | Into *both* `locales/en.json` and `locales/de.json`, `placeholder`, `aria-label` and `title` included. A sentence with a link or a `code` span stays *one* message and uses `richText`. Text `app.js` builds needs its key in `clientStringKeys` |
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

Maximum information, minimum text. Reach for a diagram before a paragraph, a table
before a list of sentences, and a screenshot before a description of the screen. A
thorough page nobody finishes is worth less than a short one that gets read.

Diagrams are `.mmd` sources in `docs/_diagrams/`, rendered to committed SVGs — see the
[README](docs/_diagrams/README.md) there for how to reference one.

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
