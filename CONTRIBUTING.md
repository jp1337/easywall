# Contributing to easywall

Thank you for your interest in contributing! This document explains how to get
started and what we expect from contributions.

## Development Setup

```bash
git clone https://github.com/jp1337/easywall.git
cd easywall

# Install Go 1.25+
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
3. Open a PR with the subject: `feat(i18n): add <language> translation`

## Code Style

- Follow standard Go conventions (`gofmt`, `golangci-lint`)
- No comments explaining *what* code does — only *why* when non-obvious
- No half-finished features behind flags — implement completely or not at all

## Styling

The user interface follows [`DESIGN.md`](DESIGN.md) in the repository root. It is the
single source of truth for colour, typography, spacing, radii, motion and components —
read it before touching anything visual.

- **Never write a colour into a template.** Use the tokens: `bg-surface`,
  `text-ink-muted`, `border-rule`, and so on. Tailwind generates these from the
  `@theme` block in `web/src/app.css`.
- **Green, amber and red are reserved for firewall state** — live, unconfirmed, rolled
  back. They are never decorative, and there is no informational colour.
- **The accent is rationed** to what is focused, what is active, and the one primary
  action on a page.
- **A control's outline uses `control-edge`; a container's uses `rule`.** WCAG 2.1
  SC 1.4.11 wants 3:1 for anything you can operate, and in this system the fill of a
  field sits ~1.05:1 from the panel behind it — the border is the whole affordance.
  Hover on a control goes to `ink-subtle`, because `rule-strong` is *weaker* than
  `control-edge` and would fade the outline you just pointed at.
- **Every visible string goes through `{{T "key"}}`,** with the text added to *both*
  `locales/en.json` and `locales/de.json`. Hardcoding English is how a bilingual
  product ends up half-translated.
- Both themes must work. Check light mode as well as dark before opening a PR.
- **Sentence case, in Inter.** Panel headings and table column heads are language, not
  data. The tracked uppercase mono `label` role is retired everywhere except the sidebar's
  two nav dividers — do not reintroduce it.
- **A table must reflow.** Below 720px rows become labelled cards, which works only if
  every `<td>` carries a `data-label`. If you add a column, add the attribute — and if you
  build rows in `app.js` too, read the label out of the `<thead>` so it stays translated.
- **One heading per thing.** A page title followed by a card titled the same thing is the
  duplicate-heading bug, not structure.
- There is no third-party component library. If you need a component that does not
  exist, add it to `DESIGN.md` first, then implement it from those tokens.
- **Verify by rendering, not by reading the CSS.** Every significant defect in this
  interface — a clipped port number, a class name that no longer existed, an arrow that
  wrapped onto its own line — was invisible in the stylesheet and obvious in a screenshot.
  Load the pages you touched, in both themes, at a phone width as well as a desktop one.
- Rebuild the stylesheets after changing them — the compiled files are committed:

  ```bash
  npm run build:css        # web/static/style.css
  npm run build:docs-css   # docs/assets/css/style.css
  ```

Validate the design system itself with `npx @google/design.md lint DESIGN.md`. Some
warnings are expected and are explained inside the file.

## Security Issues

Please see [SECURITY.md](SECURITY.md) — do not open public issues for
security vulnerabilities.
