# Diagrams

One `.mmd` file per picture. `npm run build:diagrams` renders each to two SVGs —
`<name>-light.svg` and `<name>-dark.svg` — under `docs/assets/diagrams/`, in the
palette from `DESIGN.md`. Both are committed; `npm run check:diagrams` fails if a
source changed without a re-render.

Pre-rendered rather than drawn in the browser: mermaid's runtime is 3.5 MB, the
docs site makes no third-party requests, and an SVG cannot shift the layout after
paint.

Reference one from a page through the include, which emits both variants and
lets CSS show the one matching the current theme:

```liquid
{% include themed-figure.html base="/assets/diagrams/NAME" ext="svg"
   alt="Describe what the diagram shows" %}
```

In `README.md`, which is read on github.com rather than served by Jekyll, keep
`<picture>` and plain relative paths. There is no include and no theme toggle
there, and github.com does set `prefers-color-scheme`, so the media query is the
right signal in that one place:

```html
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/diagrams/NAME-dark.svg">
  <img src="docs/assets/diagrams/NAME-light.svg" alt="Describe what the diagram shows">
</picture>
```

The `alt` text is not optional. It is what a screen reader gets instead of the
picture, and what everyone gets if the image fails to load.

## Why not `<picture>`

The docs site used `<picture>` with `<source media="(prefers-color-scheme: dark)">`
until v2.4.2. That media query reports the reader's **operating system**, while the
site's theme is a `data-theme` attribute set by the sidebar toggle. The two disagree
as soon as anyone uses the toggle, and the layout's attempt to reconcile them by
reassigning `img.src` could not work: a matching `<source>` always wins over the
`src`. A reader on a dark OS who chose the light documentation got dark diagrams on
a white page, permanently.

The include sidesteps the media query entirely — both variants are in the markup and
CSS picks one from `[data-theme]`, the same signal that themes everything else on the
page. `loading="lazy"` keeps the hidden variant off the wire, since it never enters
the viewport.
