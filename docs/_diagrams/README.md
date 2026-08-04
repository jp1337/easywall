# Diagrams

One `.mmd` file per picture. `npm run build:diagrams` renders each to two SVGs —
`<name>-light.svg` and `<name>-dark.svg` — under `docs/assets/diagrams/`, in the
palette from `DESIGN.md`. Both are committed; `npm run check:diagrams` fails if a
source changed without a re-render.

Pre-rendered rather than drawn in the browser: mermaid's runtime is 3.5 MB, the
docs site makes no third-party requests, and an SVG cannot shift the layout after
paint.

Reference one from a page like this — `<picture>` picks the theme, and the docs
site's own toggle swaps it too:

```html
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="{{ '/assets/diagrams/NAME-dark.svg' | relative_url }}">
  <img src="{{ '/assets/diagrams/NAME-light.svg' | relative_url }}" alt="Describe what the diagram shows">
</picture>
```

In `README.md`, which is read on github.com rather than served by Jekyll, use
plain relative paths instead of the `relative_url` filter:

```html
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/diagrams/NAME-dark.svg">
  <img src="docs/assets/diagrams/NAME-light.svg" alt="Describe what the diagram shows">
</picture>
```

The `alt` text is not optional. It is what a screen reader gets instead of the
picture, and what everyone gets if the image fails to load.

## One trade-off worth knowing

`<source media="(prefers-color-scheme: dark)">` follows the reader's **operating
system**, and the site's toggle can disagree with it. The layout corrects the `src`
after load, which means a reader whose OS is light and whose site theme is dark fetches
each diagram twice — one request is cancelled in flight.

The alternative is setting the source from JavaScript alone, which shows nothing at all
without it. A cancelled request for a few kB is the cheaper failure.
