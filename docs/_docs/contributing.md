---
layout: default
title: Contributing
description: Setup in three commands, the rules a review will check, and where the design system lives.
---

# Contributing

Full guidelines: [CONTRIBUTING.md](https://github.com/jp1337/easywall/blob/main/CONTRIBUTING.md).
This page is the short version.

```bash
git clone https://github.com/jp1337/easywall.git && cd easywall
go mod download
make test lint
```

| | |
|---|---|
| Toolchain | whatever `go.mod`'s `toolchain` line says — never write a version anywhere else |
| Commits | [Conventional Commits](https://www.conventionalcommits.org/): `feat`, `fix`, `security`, `docs`, `chore`, `refactor`, `test` |
| Coverage | must not fall below 80% |
| Anything visual | goes through [`DESIGN.md`](https://github.com/jp1337/easywall/blob/main/DESIGN.md) first |

## Rebuilding the committed assets

Three build outputs are committed, so a release needs no Node toolchain. CI
rebuilds each one and fails on any difference.

```bash
npm run build:css         # web/static/style.css      — the application
npm run build:docs-css    # docs/assets/css/style.css — this site
npm run build:diagrams    # docs/assets/diagrams/*.svg — two per source
```

`npm run check:diagrams` fails if a `.mmd` source changed without a re-render.
`npx @google/design.md lint DESIGN.md` validates the design system; some warnings
are expected and explained inside the file.

## What the review checks

| | |
|---|---|
| **Colour means state** | Green, amber and red are the firewall's vocabulary — live, unconfirmed, rolled back. A count is not a state |
| **The accent is rationed** | What is focused, what is active, the one primary action |
| **Controls vs containers** | A control's outline is `control-edge` (3:1, WCAG 1.4.11); a container's is `rule` |
| **Both themes, three widths** | Light and dark, at 1600 / 900 / 390 px |
| **Sentence case, Inter** | The tracked uppercase mono `label` role survives only in the sidebar dividers |
| **Tables reflow** | Below 720px rows become cards, which works only if every `<td>` has a `data-label` |
| **One heading per thing** | A page title plus a card titled the same is the duplicate-heading bug |
| **Every string translated** | {% raw %}`{{T "key"}}`{% endraw %} into *both* `locales/en.json` and `locales/de.json`, attributes included |
| **Screenshots follow the interface** | A page you changed gets its `docs/assets/img/screens/*` retaken, both themes, in the same pull request |

**Render what you changed.** Every defect worth catching in this interface was
invisible in the stylesheet and obvious in a screenshot — a clipped port number, a
class that no longer existed, a whole page on a white background.

## Adding a language

1. Copy `locales/en.json` to `locales/<lang>.json`
2. Translate the values; leave every `id` alone
3. Translate `language_name` into that language's **own** name — `Deutsch`, not `German`
4. PR titled `feat(i18n): add <language> translation`

Nothing else is needed — the switch is built from whatever `locales/*.json` contains.
Three inline forms have to survive into your language:

| In the message | Renders as |
|---|---|
| `` `443` `` | `<code>443</code>` |
| `*before*` | `<em>before</em>` — emphasis that carries meaning |
| `{}` | a link, **in your word order** |

## Documentation

Maximum information, minimum text: reach for a diagram before a paragraph, a table
before a list of sentences, and a screenshot before a description of a screen.

This site is written for whoever runs easywall. Notes for whoever *maintains* it —
the CI pipeline, the packaging traps, the guard tests — live in `docs-tech/` in the
repository and are deliberately not published here.

## Security issues

**Not** as a public issue, and **not** on Discord —
[GitHub Security Advisories](https://github.com/jp1337/easywall/security/advisories/new).

## Asking rather than reporting

Anything that is a question rather than a defect belongs on
[Discord]({{ site.discord }}) — including "is this supposed to happen?", which is
often how a defect starts.

