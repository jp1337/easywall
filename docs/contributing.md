---
layout: default
title: Contributing
description: How to contribute to easywall — setup, commit conventions, PR process.
---

# Contributing

Thank you for your interest in contributing to easywall!

Full contribution guidelines are in [CONTRIBUTING.md](https://github.com/jp1337/easywall/blob/main/CONTRIBUTING.md) in the repository root.

## Quick Start

```bash
git clone https://github.com/jp1337/easywall.git
cd easywall
go mod download
make test
make lint
```

## Working on the interface

The interface follows [`DESIGN.md`](https://github.com/jp1337/easywall/blob/main/DESIGN.md)
in the repository root — the single source of truth for colour, typography, spacing,
motion and components. Read it before changing anything visual.

There is no third-party component library. Tokens are declared once in the `@theme`
block of `web/src/app.css`, and Tailwind generates matching utilities from them, so
a template never names a colour directly — it uses `bg-surface`, `text-ink-muted`,
`border-rule` and so on.

Five rules the review will check:

- Green, amber and red are reserved for firewall state: live, unconfirmed, rolled
  back. A count is not a state.
- The accent marks what is focused, what is active, and the one primary action.
- Both themes must work. Check light mode as well as dark.
- Headings and column heads are sentence case in Inter. The uppercase mono `label`
  role survives only in the sidebar's nav dividers.
- Every table cell carries a `data-label`, because below 720px rows reflow into
  labelled cards and the attribute is what labels them.

Render what you changed. Every defect worth catching here was invisible in the
stylesheet and obvious in a screenshot — check a phone width as well as a desktop one.

The compiled stylesheets are committed, so rebuild them after any change:

```bash
npm install
npm run build:css        # web/static/style.css
npm run build:docs-css   # docs/assets/css/style.css
```

`npx @google/design.md lint DESIGN.md` validates the design system itself. Some
warnings are expected and are explained inside the file.

## Commit Convention

We use **Conventional Commits**: `feat:`, `fix:`, `security:`, `docs:`, `chore:`, `refactor:`, `test:`

## Reporting Security Issues

Please use [GitHub Security Advisories](https://github.com/jp1337/easywall/security/advisories/new) — not public issues. See [SECURITY.md](https://github.com/jp1337/easywall/blob/main/SECURITY.md).
