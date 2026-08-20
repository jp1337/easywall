---
version: alpha
name: easywall-graphite
description: |
  A dark-first control surface for a Linux firewall. The ground is a cool near-black
  (#0a0b0f) whose greys are bent toward blue so they harmonise with the single accent
  rather than sitting under it as dead neutral. That accent is a pale ice blue
  (#8fd3fb) and it appears in exactly three situations: what is focused, what is
  active, and the one primary action on the page. Everything else is carried by
  weight, hairline rules, and spacing.

  The discipline that shapes the whole system: green, amber and red are not part of
  the palette — they are the firewall's vocabulary. Green means a rule is live, amber
  means a change is unconfirmed, red means something was rolled back or is failing.
  Because the accent lives in the blue family, colour in this interface is never
  decorative: if something is coloured, it is telling you the state of your firewall.

  Network data — ports, addresses, CIDRs, timestamps, counters — is set in a monospace
  face with tabular figures throughout, because column alignment is how an operator
  scans a ruleset. Prose is set in a neutral grotesk. Panels are flat: borders do the
  separating, and shadow is reserved for things that genuinely float above the page.

colors:
  primary: "#8fd3fb"
  on-primary: "#071219"
  canvas: "#0a0b0f"
  surface: "#111318"
  surface-raised: "#181b22"
  rule: "#252932"
  rule-strong: "#333a47"
  control-edge: "#5e636e"
  ink: "#f1f3f6"
  ink-muted: "#a2aab8"
  ink-subtle: "#7d8593"
  accent: "#8fd3fb"
  accent-ink: "#071219"
  accent-wash: "rgba(143,211,251,0.13)"
  accent-on-wash: "#8fd3fb"
  state-ok: "#3ecf8e"
  state-warn: "#e5a54b"
  state-crit: "#f2555a"
  state-ok-on-wash: "#3ecf8e"
  state-warn-on-wash: "#e5a54b"
  state-crit-on-wash: "#f2555a"

typography:
  display:
    fontFamily: Inter
    fontSize: 26px
    fontWeight: 600
    lineHeight: 1.2
    letterSpacing: "-0.02em"
  title:
    fontFamily: Inter
    fontSize: 20px
    fontWeight: 600
    lineHeight: 1.3
    letterSpacing: "-0.018em"
  heading:
    fontFamily: Inter
    fontSize: 14px
    fontWeight: 600
    lineHeight: 1.4
    letterSpacing: "-0.01em"
  body:
    fontFamily: Inter
    fontSize: 14px
    fontWeight: 400
    lineHeight: 1.55
    letterSpacing: "0em"
  body-strong:
    fontFamily: Inter
    fontSize: 14px
    fontWeight: 550
    lineHeight: 1.55
    letterSpacing: "0em"
  body-sm:
    fontFamily: Inter
    fontSize: 13px
    fontWeight: 400
    lineHeight: 1.5
    letterSpacing: "0em"
  meta:
    fontFamily: Inter
    fontSize: 12px
    fontWeight: 400
    lineHeight: 1.45
    letterSpacing: "0em"
  section-label:
    fontFamily: Inter
    fontSize: 15px
    fontWeight: 600
    lineHeight: 1.4
    letterSpacing: "-0.01em"
  column-label:
    fontFamily: Inter
    fontSize: 12px
    fontWeight: 500
    lineHeight: 1.4
  field-label:
    fontFamily: Inter
    fontSize: 13px
    fontWeight: 500
    lineHeight: 1.45
  label:
    fontFamily: JetBrains Mono
    fontSize: 10px
    fontWeight: 500
    lineHeight: 1.4
    letterSpacing: "0.1em"
  data:
    fontFamily: JetBrains Mono
    fontSize: 13px
    fontWeight: 500
    lineHeight: 1.45
    letterSpacing: "0em"
    fontFeature: "tnum"
  data-sm:
    fontFamily: JetBrains Mono
    fontSize: 11px
    fontWeight: 400
    lineHeight: 1.4
    letterSpacing: "0em"
    fontFeature: "tnum"
  data-display:
    fontFamily: JetBrains Mono
    fontSize: 26px
    fontWeight: 600
    lineHeight: 1.1
    letterSpacing: "-0.02em"
    fontFeature: "tnum"
  countdown:
    fontFamily: JetBrains Mono
    fontSize: 40px
    fontWeight: 300
    lineHeight: 1
    letterSpacing: "-0.02em"
    fontFeature: "tnum"

rounded:
  sm: 4px
  md: 6px
  lg: 8px
  xl: 12px
  2xl: 16px
  full: 9999px

spacing:
  2xs: 4px
  xs: 8px
  sm: 12px
  md: 16px
  lg: 20px
  xl: 24px
  2xl: 32px
  3xl: 48px
  4xl: 64px

layout:
  sidebar-width: 240px
  topbar-height: 48px
  content-max: none
  form-max: 640px
  control-height: 32px
  control-height-sm: 28px
  row-height: 36px
  mobile-breakpoint: 768px

motion:
  instant: 60ms
  fast: 120ms
  slow: 200ms
  easing: cubic-bezier(0.2, 0, 0.2, 1)

components:
  button-primary:
    backgroundColor: "{colors.accent}"
    textColor: "{colors.accent-ink}"
    typography: "{typography.body-strong}"
    rounded: "{rounded.md}"
    height: 32px
    padding: 0 12px
  button-primary-hover:
    backgroundColor: "#a8ddfc"
    textColor: "{colors.accent-ink}"
    typography: "{typography.body-strong}"
    rounded: "{rounded.md}"
  button-secondary:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    borderColor: "{colors.control-edge}"
    typography: "{typography.body-strong}"
    rounded: "{rounded.md}"
    height: 32px
    padding: 0 12px
  button-secondary-hover:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.ink}"
    borderColor: "{colors.ink-subtle}"
    typography: "{typography.body-strong}"
    rounded: "{rounded.md}"
  button-danger:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.state-crit}"
    borderColor: "{colors.state-crit}"
    typography: "{typography.body-strong}"
    rounded: "{rounded.md}"
    padding: 6px 12px
  input:
    backgroundColor: "{colors.canvas}"
    textColor: "{colors.ink}"
    borderColor: "{colors.control-edge}"
    typography: "{typography.body}"
    rounded: "{rounded.md}"
    height: 32px
    padding: 0 10px
  input-focus:
    backgroundColor: "{colors.canvas}"
    textColor: "{colors.ink}"
    borderColor: "{colors.accent}"
    typography: "{typography.body}"
    rounded: "{rounded.md}"
  input-data:
    backgroundColor: "{colors.canvas}"
    textColor: "{colors.ink}"
    borderColor: "{colors.control-edge}"
    typography: "{typography.data}"
    rounded: "{rounded.md}"
    padding: 6px 10px
  card:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    borderColor: "{colors.rule}"
    typography: "{typography.body}"
    rounded: "{rounded.xl}"
  card-header:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    borderColor: "{colors.rule}"
    typography: "{typography.heading}"
    padding: 10px 14px
  tile:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    borderColor: "{colors.rule}"
    typography: "{typography.data-display}"
    rounded: "{rounded.xl}"
    padding: 12px 14px
  tile-label:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink-muted}"
    typography: "{typography.field-label}"
  table-header-cell:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink-muted}"
    borderColor: "{colors.rule}"
    typography: "{typography.column-label}"
    padding: 9px 16px
  table-cell:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    borderColor: "{colors.rule}"
    typography: "{typography.body-sm}"
    padding: 8px 14px
  table-cell-data:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    borderColor: "{colors.rule}"
    typography: "{typography.data}"
    padding: 8px 14px
  table-row-hover:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.ink}"
    typography: "{typography.body-sm}"
  chip-neutral:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.ink-muted}"
    borderColor: "{colors.rule}"
    typography: "{typography.data-sm}"
    rounded: "{rounded.sm}"
    padding: 1px 7px
  chip-accent:
    backgroundColor: "{colors.accent-wash}"
    textColor: "{colors.accent-on-wash}"
    borderColor: "{colors.accent}"
    typography: "{typography.data-sm}"
    rounded: "{rounded.sm}"
    padding: 1px 7px
  chip-ok:
    backgroundColor: "rgba(62,207,142,0.10)"
    textColor: "{colors.state-ok-on-wash}"
    borderColor: "{colors.state-ok}"
    typography: "{typography.data-sm}"
    rounded: "{rounded.sm}"
    padding: 1px 7px
  chip-warn:
    backgroundColor: "rgba(229,165,75,0.10)"
    textColor: "{colors.state-warn-on-wash}"
    borderColor: "{colors.state-warn}"
    typography: "{typography.data-sm}"
    rounded: "{rounded.sm}"
    padding: 1px 7px
  chip-crit:
    backgroundColor: "rgba(242,85,90,0.10)"
    textColor: "{colors.state-crit-on-wash}"
    borderColor: "{colors.state-crit}"
    typography: "{typography.data-sm}"
    rounded: "{rounded.sm}"
    padding: 1px 7px
  sidebar:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink-muted}"
    borderColor: "{colors.rule}"
    typography: "{typography.body-sm}"
  nav-item:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink-muted}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.md}"
    padding: 6px 10px
  nav-item-hover:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.ink}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.md}"
  nav-item-active:
    backgroundColor: "{colors.accent-wash}"
    textColor: "{colors.accent-on-wash}"
    typography: "{typography.body-strong}"
    rounded: "{rounded.md}"
  nav-section-label:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink-subtle}"
    typography: "{typography.label}"
    padding: 0 10px
  topbar:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    borderColor: "{colors.rule}"
    typography: "{typography.heading}"
  avatar:
    backgroundColor: "{colors.accent-wash}"
    textColor: "{colors.accent-on-wash}"
    borderColor: "{colors.rule}"
    typography: "{typography.data-sm}"
    rounded: "{rounded.full}"
  status-dot:
    backgroundColor: "{colors.ink-subtle}"
    textColor: "{colors.ink}"
    typography: "{typography.body-strong}"
    rounded: "{rounded.full}"
    size: 7px
  log-row:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink-muted}"
    borderColor: "{colors.rule}"
    typography: "{typography.meta}"
    padding: 7px 14px
  log-timestamp:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink-subtle}"
    typography: "{typography.data-sm}"
  countdown:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.state-warn}"
    typography: "{typography.countdown}"
  alert-warn:
    backgroundColor: "rgba(229,165,75,0.10)"
    textColor: "{colors.state-warn-on-wash}"
    borderColor: "{colors.state-warn}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.md}"
    padding: 10px 14px
  alert-crit:
    backgroundColor: "rgba(242,85,90,0.10)"
    textColor: "{colors.state-crit-on-wash}"
    borderColor: "{colors.state-crit}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.md}"
    padding: 10px 14px
  button-sm:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    borderColor: "{colors.control-edge}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.md}"
    height: 28px
    padding: 0 10px
  button-disabled:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink-subtle}"
    borderColor: "{colors.rule}"
    typography: "{typography.body-strong}"
    rounded: "{rounded.md}"
  input-hover:
    backgroundColor: "{colors.canvas}"
    textColor: "{colors.ink}"
    borderColor: "{colors.ink-subtle}"
    typography: "{typography.body}"
    rounded: "{rounded.md}"
  input-error:
    backgroundColor: "{colors.canvas}"
    textColor: "{colors.ink}"
    borderColor: "{colors.state-crit}"
    typography: "{typography.body}"
    rounded: "{rounded.md}"
  input-disabled:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.ink-subtle}"
    borderColor: "{colors.rule}"
    typography: "{typography.body}"
    rounded: "{rounded.md}"
  textarea-rules:
    backgroundColor: "{colors.canvas}"
    textColor: "{colors.ink}"
    borderColor: "{colors.rule}"
    typography: "{typography.data}"
    rounded: "{rounded.md}"
    padding: 10px 12px
  select:
    backgroundColor: "{colors.canvas}"
    textColor: "{colors.ink}"
    borderColor: "{colors.control-edge}"
    typography: "{typography.body}"
    rounded: "{rounded.md}"
    height: 32px
    padding: 0 10px
  lang-select:
    backgroundColor: "{colors.canvas}"
    textColor: "{colors.ink}"
    borderColor: "{colors.control-edge}"
    typography: "{typography.meta}"
    rounded: "{rounded.sm}"
    height: "{control-height-sm}"
    padding: 0 6px
  lang-submit:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    borderColor: "{colors.control-edge}"
    typography: "{typography.meta}"
    rounded: "{rounded.sm}"
    height: "{control-height-sm}"
    padding: 0 8px
  toggle:
    backgroundColor: "{colors.control-edge}"
    textColor: "{colors.ink}"
    rounded: "{rounded.full}"
    width: 34px
    height: 19px
  toggle-on:
    backgroundColor: "{colors.accent}"
    textColor: "{colors.accent-ink}"
    rounded: "{rounded.full}"
    width: 34px
    height: 19px
  checkbox:
    backgroundColor: "{colors.canvas}"
    textColor: "{colors.ink}"
    borderColor: "{colors.control-edge}"
    rounded: "{rounded.sm}"
    size: 16px
  checkbox-checked:
    backgroundColor: "{colors.accent}"
    textColor: "{colors.accent-ink}"
    rounded: "{rounded.sm}"
    size: 16px
  fieldset:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    borderColor: "{colors.rule}"
    typography: "{typography.body}"
    rounded: "{rounded.xl}"
    padding: 14px 16px
  fieldset-legend:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    typography: "{typography.field-label}"
  link:
    backgroundColor: "transparent"
    textColor: "{colors.accent-on-wash}"
    typography: "{typography.body}"
  badge:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.ink-muted}"
    borderColor: "{colors.rule}"
    typography: "{typography.data-sm}"
    rounded: "{rounded.sm}"
    padding: 1px 7px
  tooltip:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.ink}"
    borderColor: "{colors.rule}"
    typography: "{typography.meta}"
    rounded: "{rounded.md}"
    padding: 5px 9px
  alert-neutral:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.ink}"
    borderColor: "{colors.rule}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.md}"
    padding: 10px 14px
  tab:
    backgroundColor: "transparent"
    textColor: "{colors.ink-muted}"
    typography: "{typography.body-sm}"
    height: 32px
    padding: 0 12px
  tab-active:
    backgroundColor: "transparent"
    textColor: "{colors.ink}"
    typography: "{typography.body-strong}"
    height: 32px
    padding: 0 12px
  empty-state:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink-subtle}"
    typography: "{typography.body-sm}"
    padding: 40px 24px
  section-head:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    borderColor: "{colors.rule}"
    typography: "{typography.section-label}"
    padding: 13px 16px
  module:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    borderColor: "{colors.rule}"
    typography: "{typography.body}"
    rounded: "{rounded.xl}"
  module-active:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    borderColor: "{colors.accent}"
    typography: "{typography.body}"
    rounded: "{rounded.xl}"
  module-params:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.ink-muted}"
    borderColor: "{colors.rule}"
    typography: "{typography.body-sm}"
    padding: 10px 15px
  switch-row:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    borderColor: "{colors.rule}"
    typography: "{typography.body-strong}"
    padding: 12px 16px
  switch-row-hover:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.ink}"
    typography: "{typography.body-strong}"
  toolbar:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink-muted}"
    borderColor: "{colors.rule}"
    typography: "{typography.body-sm}"
    padding: 11px 16px
  save-bar:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink-muted}"
    borderColor: "{colors.rule}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.xl}"
    padding: 11px 16px
  aside-card:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink-muted}"
    borderColor: "{colors.rule}"
    typography: "{typography.meta}"
    rounded: "{rounded.xl}"
    padding: 15px 16px
  status-panel:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    borderColor: "{colors.rule}"
    typography: "{typography.title}"
    rounded: "{rounded.2xl}"
    padding: 18px 20px
  step-marker:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.ink-muted}"
    borderColor: "{colors.rule}"
    typography: "{typography.data-sm}"
    rounded: "{rounded.full}"
  activity-row:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    borderColor: "{colors.rule}"
    typography: "{typography.body-sm}"
    padding: 9px 16px
---

## Overview

easywall is an administrative surface, not a document. It is scanned and operated:
an operator arrives to answer a question — *is the firewall up, what is open, what
changed, is anything unconfirmed* — acts on the answer, and leaves. Every decision
below serves reading speed and unambiguous state.

**Token names are theme-independent.** The front matter carries the dark theme, which
is the default. Light mode binds different values to the same names; see the table in
*Colors*. Nothing outside those two token tables should hard-code a colour.

Two rules govern this system. They are not stylistic preferences — breaking either one
makes the interface lie about the firewall:

1. **Colour outside the blue family means state.** Green, amber and red belong to the
   firewall: live, unconfirmed, rolled back or failing. They are never used to
   decorate, brand, or draw attention to something that is merely new.
2. **The accent is rationed.** Ice blue marks what is focused, what is active, and the
   single primary action on a page. A page with ice blue in five places has no accent
   at all.

A consequence worth stating explicitly: **there is no informational colour.** Earlier
iterations tinted "settings saved" and "rules imported" log entries sky blue and
indigo. Those hues sit inside the accent's family, so a coloured log tag became
ambiguous — was it a state, or just emphasis? Informational events are now neutral
chips. Only the three states get colour.

## Colors

The greys are bent toward blue. A pure neutral grey under an ice-blue accent reads as
unconsidered — two unrelated systems in one frame. Bending the neutrals a few degrees
toward the accent makes the chrome and the accent read as one decision.

### Dark (default)

| Token | Value | Role |
|---|---|---|
| `canvas` | `#0a0b0f` | Page ground; the deepest surface |
| `surface` | `#111318` | Panels, sidebar, topbar, tiles |
| `surface-raised` | `#181b22` | Row hover, neutral chips, nested fills |
| `rule` | `#252932` | Hairline borders — the primary separator |
| `rule-strong` | `#333a47` | Input hover, stronger divisions |
| `control-edge` | `#5e636e` | Boundary of an unchecked/off control |
| `ink` | `#f1f3f6` | Primary text |
| `ink-muted` | `#a2aab8` | Secondary text, inactive navigation |
| `ink-subtle` | `#7d8593` | Labels, timestamps, captions |
| `accent` | `#8fd3fb` | Focus, active, primary action |
| `accent-ink` | `#071219` | Text and icons on an accent fill |
| `accent-wash` | `rgba(143,211,251,0.13)` | Active navigation, accent chips, avatar |
| `state-ok` | `#3ecf8e` | Rule live, daemon reachable, protection on |
| `state-warn` | `#e5a54b` | Unconfirmed change, acceptance window running |
| `state-crit` | `#f2555a` | Rolled back, unreachable, validation failed |

### Light

| Token | Value | Role |
|---|---|---|
| `canvas` | `#fcfcfd` | Page ground |
| `surface` | `#ffffff` | Panels, sidebar, topbar, tiles |
| `surface-raised` | `#f4f6f9` | Row hover, neutral chips |
| `rule` | `#e4e8ee` | Hairline borders |
| `rule-strong` | `#cdd4de` | Input hover, stronger divisions |
| `control-edge` | `#868a93` | Boundary of an unchecked/off control |
| `ink` | `#0f1116` | Primary text |
| `ink-muted` | `#5a6270` | Secondary text |
| `ink-subtle` | `#666e7b` | Labels, timestamps, captions |
| `accent` | `#0f7bab` | Focus, active, primary action |
| `accent-ink` | `#ffffff` | Text and icons on an accent fill |
| `accent-wash` | `rgba(15,123,171,0.09)` | Active navigation, accent chips, avatar |
| `accent-on-wash` | `#0d6b95` | Text sitting **on** `accent-wash` |
| `state-ok` | `#12855c` | Rule live |
| `state-warn` | `#96620d` | Unconfirmed change |
| `state-crit` | `#cf2d38` | Rolled back or failing |
| `state-ok-on-wash` | `#0f714e` | Text on a 10% `state-ok` wash |
| `state-warn-on-wash` | `#88590c` | Text on a 10% `state-warn` wash |
| `state-crit-on-wash` | `#ba2832` | Text on a 10% `state-crit` wash |

### Why the `-on-wash` tokens exist

A chip whose text and background share a hue behaves differently in the two themes, and
the difference is not symmetrical. In dark mode a 10–13% wash *darkens* the surface away
from the light text, so contrast stays high (7.4:1 to 8.7:1 measured). In light mode the
same wash *lightens* the surface toward the dark text, pulling contrast down — the naive
values landed at 4.07:1 to 4.42:1 and missed WCAG AA.

Lowering the wash opacity until it passed would have required roughly 3%, which is
invisible. So light mode instead deepens the text: the `-on-wash` tokens are the state
hues darkened until they clear 5.2:1 against their own wash, leaving headroom rather than
sitting on the 4.5 line. In dark mode the `-on-wash` tokens are simply aliases of the base
colours, since no correction is needed there.

Use `state-ok` for a standalone dot or a word on `surface`; use `state-ok-on-wash` only
when the text sits inside a tinted chip or alert.

### Why `control-edge` is separate from `rule-strong`

`rule` and `rule-strong` are decorative separators — they divide a table into rows and a
page into panels, and they are allowed to be quiet. The boundary of an *interactive* control
is not decorative: WCAG 2.1 SC 1.4.11 requires 3:1 against the adjacent surface for exactly
this, because an unchecked checkbox and an off toggle are only recognisable by their outline.

Measured with `rule-strong`, the off-state toggle track reached **1.6:1** and the unchecked
checkbox border **1.7:1** — comfortably illegible. `control-edge` is tuned to clear 3:1 in
both themes. This matters more here than in most products: the options page carries eleven
protection-module toggles, and an operator has to be able to see at a glance which
protections are *off*. A page where only the enabled toggles are visible is worse than no
page at all, because it reads as "everything is fine".

**Amended 2026-08-03: this applies to every control, not only the two that got it first.**
Toggles and checkboxes were moved to `control-edge` when this section was written; text
fields, secondary buttons and selects were left at `rule` and quietly failed the same
criterion. Measured against the surface each one actually sits on:

| Boundary | Dark | Light |
|---|---|---|
| `input` border at `rule` | 1.35:1 | 1.20:1 |
| `btn` border at `rule` | 1.28:1 | 1.23:1 |
| the same borders at `control-edge` | 3.26:1 | 3.37:1 |

The fill cannot rescue them: a field's `canvas` background sits **1.03–1.06:1** from the
`surface` of the card it is on, so the border carries the entire affordance. On the login
page the password field was, for practical purposes, an unmarked rectangle.

One consequence is that hover can no longer be `rule-strong` — at 1.28–1.72:1 that is
*weaker* than the new resting state, so hovering a field faded its outline. Control hover
states use `ink-subtle`. Containers (`card-interactive`, `module`) keep
`rule → rule-strong`: they are not controls, and for them it is still a strengthening.

`button-disabled` and `input-disabled` stay at `rule` deliberately — see the note below on
inactive components.

> **Deliberate schema extensions.** The official linter reports three groups of warnings
> against this file, all expected:
> - `borderColor` is not in the component schema. In a system where hairline borders — not
>   shadow — do the separating, dropping border colour from the token layer would move the
>   single most structural decision into prose where tooling cannot see it.
> - `layout` and `motion` are not recognised token maps. Both are documented here because
>   control heights and transition durations are design decisions, not implementation
>   details; component entries carry literal values so nothing depends on the extension
>   resolving.
> - The contrast rule cannot composite alpha: it reads `accent-wash` as opaque and compares
>   it against itself, reporting 1.00:1. Composited over its real surface the same pairs
>   measure 7.4:1 to 8.7:1. Verify contrast by compositing, not by trusting that rule.

> **Disabled controls** sit at 4.17:1 in light mode, below AA. This is intentional and
> permitted — WCAG exempts inactive components, and reduced contrast is the affordance that
> communicates "disabled". Do not "fix" it by darkening the text.

Note the inversion between themes: in dark mode `surface` is **lighter** than `canvas`,
in light mode it is **whiter** than a faintly grey canvas. In both cases the panel
advances and the page recedes — the relationship is preserved even though the direction
flips. Do not "simplify" this by making light-mode canvas pure white; the panels would
lose their edge.

The accent shifts hue slightly between themes on purpose. A pale ice blue that sings on
near-black turns illegible on white, so light mode uses a deeper, more saturated blue at
the same role. Both clear 4.5:1 against their own surface.

## Typography

Two families, split by what the text *is*:

- **Inter** for language — headings, labels in sentence case, descriptions, button text.
  A neutral grotesk chosen for its small-size clarity and its genuinely tabular figures.
- **JetBrains Mono** for network data and for uppercase micro-labels. Ports, IPv4/IPv6
  addresses, CIDR prefixes, timestamps, counters and raw nftables rules are set in it,
  always with tabular figures.

Monospace here is not an aesthetic reference to terminals. It is functional: an operator
comparing `10.0.1.0/24` against `10.0.11.0/24`, or scanning a port column for an outlier,
needs digits and glyphs to occupy identical width so misalignment is visible. Proportional
figures actively hide the differences that matter.

**Self-host both faces.** easywall runs on machines that frequently have no outbound
internet access, and it is an administrative interface where a third-party request is
inappropriate on principle. Fonts must ship with the binary, not load from a CDN.

### Scale

| Role | Size / Weight | Used for |
|---|---|---|
| `display` | 26 / 600 | Page titles |
| `title` | 20 / 600 | Section titles, auth card heading |
| `heading` | 14 / 600 | Panel headers, topbar title |
| `body` | 14 / 400 | Default UI text |
| `body-strong` | 14 / 550 | Active nav, button labels, emphasised state |
| `body-sm` | 13 / 400 | Table cells, dense forms |
| `meta` | 12 / 400 | Log detail, helper text |
| `section-label` | 15 / 600, sentence case | Panel and card headings |
| `column-label` | 12 / 500, sentence case | Table column headers |
| `label` | 10 / 500, `0.1em`, uppercase, mono | Sidebar nav section dividers only |
| `data` | 13 / 500, mono, tnum | Ports, addresses, in-table values |
| `data-sm` | 11 / 400, mono, tnum | Chips, timestamps, counters |
| `data-display` | 26 / 600, mono, tnum | Tile values |
| `countdown` | 40 / 300, mono, tnum | Acceptance-window timer |

Headings carry negative tracking (`-0.01em` to `-0.02em`); it tightens multi-word titles
without making them look condensed. `label` runs the other way at `+0.1em` — uppercase at
10px needs the air or it sets as a smudge.

**`label` used to be the general-purpose heading device**, specified here for table headers,
tile labels and nav sections alike. Rendering the built interface retired it. A single page
carried six or more of these — `ATTACK PROTECTION`, `PORT`, `SSH PROTECTION`,
`ACCEPTANCE STATUS` — and at that density the effect was not restraint but a dated
enterprise console: tracked capitals at 10px take measurably longer to read than sentence
case at 12, and repeated as the only structural device they became the interface's loudest
mannerism. Panel headings now use `section-label` and column headers `column-label`, both
sentence case in Inter.

The uppercase mono `label` survives in exactly one place: the sidebar's *Rules* and *System*
dividers. There it separates groups rather than titling content, it appears twice, and the
smallness is the point.

## Layout

The frame is fixed by the `layout` tokens: a `sidebar-width` of 240px, a `topbar-height` of
48px, and content that runs the **full remaining width**.

An earlier draft of this document capped content at 1100px, reasoning that a rule row
scanned across 2000px defeats the purpose of the table. Rendering it disproved that: with a
fixed 240px sidebar already absorbing the left edge, the cap left a dead band down the right
of every wide display while the description column — the one field that actually wants
room — was the one being truncated. Width belongs to the tables.

Only genuinely text-shaped containers narrow themselves, via `content-narrow` at `form-max`
640px: single-column forms and prose, where a 1600px measure is unreadable. A table is not
text and does not take that class.

Interactive controls share one height. `control-height` (32px) applies to buttons, inputs
and selects; `control-height-sm` (28px) is the compact variant used in table toolbars;
`row-height` (36px) governs table and log rows. Consistent heights are what make a dense
page look deliberate instead of assembled — mismatched control heights are the single most
common reason an admin interface feels sloppy.

Spacing runs on a 4px base. Use `xs` (8px) inside a component, `sm`–`md` (12–16px) between
related elements, `xl` (24px) between sections. Sibling groups are laid out with flex or
grid and `gap` — never per-element margins, which collapse unpredictably and double up.

Wide content is the norm here, not the exception. Every table lives in its own
`overflow-x: auto` container so the page body never scrolls sideways.

Below 768px the sidebar becomes an overlay drawer.

**Below 720px tables reflow into labelled cards.** An earlier revision of this document said
the opposite — keep the columns, scroll sideways — on the reasoning that a rule read as a
list of labelled fields loses the row relationship that makes it a rule. Rendering the port
table at 390px settled it: the port column collapsed until `22` displayed as `2:` and every
description truncated mid-word. A rule whose identifier is unreadable has no relationship
left to preserve. Each row becomes one bordered card, its column heading travelling with
each value as that value's label, so the row still reads as a single unit.

One field per row is the exception: a free-text description gets its own full-width line with
the label above it, because 62% of a phone's width is not a usable measure for prose.

### Content grid

Most pages here carry few rows — eight ports, four forwards, eleven switches. Run that alone
across a 1360px canvas and the columns stretch absurdly; rendering showed `SSH (admin)`
sitting in a 950px-wide cell. The earlier conclusion — that the fix was to cap the page —
was wrong twice over: a cap left a dead band down the right, and the width was never the
problem. Having nothing to put in it was.

Rule and settings pages therefore run a two-column `page-grid`: the work column, and a 320px
context column carrying what an operator editing firewall rules wants at hand anyway — the
syntax a field accepts, what a list does, what happens on save, which setting elsewhere this
one depends on. Below 1180px the context column drops beneath the work column. Content still
runs full width; it simply has something to say across it.

## Elevation & Depth

This system is **flat by default**. Borders separate; shadow does not. A panel sitting on
the page gets `rule`, not a drop shadow. Depth is reserved for things that genuinely float
above the document and must be read as temporarily obscuring it:

| Level | Dark | Light | Used for |
|---|---|---|---|
| `flat` | none | none | Panels, tiles, tables, sidebar — the default |
| `raised` | `0 1px 2px rgba(0,0,0,0.35)` | `0 1px 2px rgba(15,17,22,0.06)` | Dropdowns, select menus |
| `lift` | `0 2px 8px -2px rgba(0,0,0,0.45)` | `0 2px 8px -2px rgba(15,17,22,0.12)` | Hover state of an interactive card or tile |
| `overlay` | `0 8px 24px -8px rgba(0,0,0,0.55)` | `0 8px 24px -8px rgba(15,17,22,0.14)` | Modals, mobile sidebar drawer, sticky save bar |

Flat remains the resting state, and the rule that shadow means "floats above the document"
still holds for `overlay`. `lift` is the one addition: a card that is itself a link or a
control may take it **on hover only**, together with a 1px upward shift. It is not depth as
decoration — it is the surface answering the pointer, and a static panel never gets it.
Under `prefers-reduced-motion` the shadow and border still respond; the shift does not.

Dark-mode shadows are built from black; light-mode shadows from `ink` at low opacity, which
keeps them in the page's blue-grey family instead of muddying it with neutral grey.

## Shapes

| Token | Value | Used for |
|---|---|---|
| `sm` | 4px | Chips, log tags, small badges |
| `md` | 6px | Buttons, inputs, selects, nav items |
| `lg` | 8px | Small inset surfaces inside a card |
| `xl` | 12px | Panels, cards, tiles, modals, auth card |
| `2xl` | 16px | The status panel on dashboard and apply |
| `full` | 9999px | Status dots, avatar |

**Controls stay tight; containers do not.** Buttons and inputs keep `md`, so the density of
a dense form is untouched — that was the real substance of the earlier rule that "radii stay
tight", and it holds. What did not hold was applying it to panels: at 8px, a card read as a
box drawn around content rather than a surface holding it, and with a flat fill and a
hairline border as the only other devices, every page resolved into a grid of rectangles.
Containers moved up one step. It is the cheapest available change of register.

## Motion

Motion in this interface exists to explain a change, never to decorate one. An operator
applying firewall rules is watching for confirmation, not for choreography.

| Token | Value | Used for |
|---|---|---|
| `instant` | 60ms | Hover feedback on rows, buttons, nav items |
| `fast` | 120ms | Colour and border transitions, chip and badge changes |
| `slow` | 200ms | Drawer open/close, panel expansion, flash message entry |
| `easing` | `cubic-bezier(0.2, 0, 0.2, 1)` | Everything — fast out, settled in |

Nothing animates for longer than 200ms. Nothing loops except the two indicators that mean
"still happening": the pending status dot and the acceptance-window countdown. A spinner
that outlives its request is a lie about the state of the system.

Honour `prefers-reduced-motion: reduce` by dropping all transitions and both loops to a
static state — the pending dot stays visibly amber, it simply stops pulsing. The
information must never live in the animation alone.

## Components

### Buttons

One primary action per view, filled with `accent`. Everything else is `button-secondary`:
surface fill and a 1px `control-edge` border — hairline in weight, but not in contrast, since
that border is the only thing separating the button from the panel behind it. Destructive actions — *Roll back now*, *Delete rule* — use
`button-danger`: a bordered button in `state-crit`, never a filled red block. A filled red
button is the loudest object on the page and invites the misclick it is warning about.

### Chips and tags

Chips carry either state or emphasis, never both. `chip-ok` / `chip-warn` / `chip-crit`
state the firewall's condition. `chip-accent` marks a qualifier the operator chose, such
as a scope restriction. `chip-neutral` is everything else, including all informational
audit-log actions.

### Tables

The workhorse. Header cells use `column-label`; value cells carrying network data use `data`
with tabular figures. Rows separate with a single `rule` hairline and lift to
`surface-raised` on hover. Right-align nothing except pure counts — port numbers read better
left-aligned against a left-aligned description.

Every table sits under a `toolbar`: the count on the right, the add action beside it, and a
filter field where a set can grow past a screenful. Filtering happens in the browser — the
rows are already there, and a round trip would discard unsaved edits.

A destructive row action rests at 55% opacity and comes fully forward on row hover or
`:focus-within`. Never at zero: a control that only exists on hover cannot be found by
someone tabbing through, and on a touch screen there is no hover at all — below the reflow
breakpoint it is always at full strength.

### Protection modules

A firewall protection is either on or off and may carry its own parameters. As rows in one
long list — which is how it was first built — fourteen of them took 1700px of scroll and
answered none of the page's actual question: *which protections are active right now.*

Each is a card in an `auto-fill` grid at `minmax(330px, 1fr)`. Name and switch in the header,
one line saying what the module does, and parameters below a hairline **inside the card** —
not on a darker band underneath it, which read as a detached second row. An active module
carries a 2px `accent` inset on its left edge: the same device that marks the active nav
item, so "this one is live" speaks one vocabulary throughout. The header is a `<label>`
wrapping its own switch, so the whole card top is the hit area.

The state is never carried by that edge alone — the switch itself shows its position, which
is what a colour-blind operator and a screenshot both rely on.

### Switch rows

For a setting that is a plain on/off with no parameters. Name, one-line description, switch
at the trailing edge, the whole row a `<label>`. Rows separate with a `rule` hairline and
take `surface-raised` on hover so the hit area is visible before the click.

The description belongs *under* the name, never above it, and never between the name and the
control. Both were tried; both made the caption read as a separate item.

### Audit log

Actions are stored as identifiers (`apply_rolledback`) and displayed as language
(*Rules rolled back*). Tone is derived from the action in the template layer, not matched in
CSS: the stylesheet once keyed on `rules_applied` and `rules_rolled_back`, names only the
demo client produced, so in production no entry was ever tinted and a rolled-back apply —
the most consequential line in the log — rendered neutral grey.

Only the four `apply_*` actions carry colour, because only they describe what the firewall is
doing: accepted is `ok`, started is `warn`, rolled back and failed are `crit`. Saving a rule
set stages it and changes nothing that is live, so it stays neutral however important it feels.

Timestamps display as clock time for today and `2 Jan 15:04` before that, with the full
RFC 3339 value on the element's `title`. A log full of `2026-08-03T15:19:33+02:00` is a
database dump, not a page.

### Status

State appears in two forms simultaneously: a dot (colour) and a word (text). Colour alone
fails for colour-blind operators and in screenshots pasted into tickets; the word alone
scans too slowly. The dot for a live or pending state carries a soft ring; a resolved or
idle state does not, so movement in the interface always means "something is still
happening".

### The acceptance window

easywall's most consequential screen. Rules are live but unconfirmed, and a timer is
counting down to automatic rollback. It gets the largest type in the system — `countdown`
at 40px in `state-warn` — and both actions are offered plainly: confirm, or roll back now.
Never style the confirm button as the calm default and the rollback as a subdued link. An
operator who reached this screen because they locked themselves out needs the escape route
to be as findable as the confirmation.

### Forms

Inputs sit on `canvas` inside a `surface` card — recessed relative to their container.
Fields that accept network data (ports, addresses, CIDRs) use `input-data` in monospace, so
a mistyped address is visible while typing rather than after saving.

Every input carries five states, and all five must be implemented — a field that only has
default and focus will silently fail the moment a rule does not validate:

| State | Border | Fill | Note |
|---|---|---|---|
| Default | `control-edge` | `canvas` | 3:1 per SC 1.4.11 — the fill is 1.03:1 from the card |
| Hover | `ink-subtle` | `canvas` | Must read stronger than default, so not `rule-strong` |
| Focus | `accent` | `canvas` | Plus a 2px `accent-wash` outline at 1px offset |
| Error | `state-crit` | `canvas` | Message below in `state-crit`, never colour alone |
| Disabled | `rule` | `surface-raised` | Text drops to `ink-subtle`; inactive, so exempt |

Focus is never removed. It is a **border change plus an outline** — not a glow, and never
colour alone, because an operator navigating by keyboard has to see where they are on a
dense page of ports.

`fieldset` groups related settings and carries its legend in `label` type. This is the
dominant structure on the options and settings pages; a flat list of forty controls with no
grouping is unusable, and the grouping is what makes the protection modules scannable.

### Toggles and checkboxes

The protection modules are the largest cluster of controls in the product — eleven toggles
on one page. A toggle means "this protection is on"; it is the only place besides the accent
button where `accent` appears as a fill, and that is deliberate: an operator scanning the
options page should be able to see at a glance how much protection is enabled.

Checkboxes are for selection, toggles are for state. Never use a toggle for something that
only takes effect after pressing Save — the toggle's own animation promises immediacy.
Where a toggle sits behind an explicit apply step, pair it with the unconfirmed-change
indicator rather than letting it imply the rule is already live.

### Language and theme switches

Both live in the sidebar footer, above *Logout*, because they are preferences rather than
navigation. The language switch also appears on the login and first-run cards: an operator
who cannot read the interface cannot sign in to change it, and the setting is useless
behind the door it locks.

The language switch used to be one small button per installed locale rather than a
`select`, on the reasoning that a `select` submitting on change needs a script, and the one
screen where this control matters most is the one you have not signed in to yet. That held
for two locales. It stopped holding once a third and fourth were roadmapped: the endonyms of
the languages that follow French run to roughly 720px, which wraps the button row into a
four- or five-row block at the foot of a 240px sidebar — in order to let somebody who cannot
read the interface change its language.

It is now a `select` (`select`, above), built so the JavaScript reason never applied in the
first place rather than accepted as a cost:

1. **The submit button is real markup**, drawn beside the select and hidden only once
   JavaScript has announced itself. It is set via `data-js` in the nonced head script — the
   same one that sets `data-theme`, and for the same reason: `app.js` loads at the end of
   `<body>`, so a flag set there would let the button render, be seen, and then vanish. With
   a script running, the select posts itself on `change` and the button is never seen; without
   one, the button is the only way to submit and it was never absent.
2. **The current language is still visible without opening anything** — the select shows the
   chosen `<option>` closed, exactly as the chip showed the active button.
3. **A `select` also gives 390px viewports the native OS picker**, which is the best mobile
   behaviour available and costs nothing extra.

Each locale supplies its own name through a `language_name` key, so an `<option>` always
reads as its language's endonym whatever the interface is currently set to — `Deutsch`, not
`German` — and there is still no "Language:" label to precede it: the field would only be
useful to someone who can already read the interface.

Drawn only when more than one locale is installed. A single option that cannot change
anything is a control that lies about having a choice.

### Links and badges

`link` uses `accent-on-wash` — the deepened accent — because a link sits in running text
where the pale ice blue would not carry enough contrast in light mode. Links underline on
hover; they are never distinguished by colour alone.

A `badge` is a neutral count or marker. A `chip` carries state or a chosen qualifier. If you
find yourself reaching for a coloured badge, you want a chip.

### Tooltips

Tooltips explain a control, they never carry information that exists nowhere else — a
tooltip is invisible on touch, unreachable by many keyboard paths, and absent from a
screenshot pasted into a bug report. Anything an operator needs in order to decide belongs
on the page.

## Logo

The mark is **Bond**: running-bond masonry, three courses, seven stretchers, drawn on the
same 24-unit grid as the interface icons. It keeps the wall from the previous logo — the
metaphor users already associate with easywall — and drops the flame, which was the more
obvious half of the pun and the half that did not survive being small.

**It is always one colour.** No gradient, no glow, no background plate, no outline cut. The
previous mark carried a gradient, a Gaussian glow and a baked-in `#0d1117` panel, which is
why it could never sit on a light surface and why it dissolved into grey below about 24px.

### Where the colour comes from

`web/static/icon.svg` is the single source of geometry. It is applied three ways:

| Context | Mechanism | Colour |
|---|---|---|
| App chrome (sidebar, auth cards) | CSS `mask` on a `<span>` | `accent` |
| Favicon (SVG) | Browser loads the file directly | Adapts via `prefers-color-scheme` inside the file |
| Favicon (PNG fallback), docs, README | Raster or `<img>` | Baked: `#0f7bab`, the one tone legible on both light and dark tab bars |

The mask matters. An SVG loaded through `<img>` is a separate document and **cannot inherit
`currentColor`** — the previous `.brand-icon { color: var(--accent) }` rule was silently
doing nothing. A mask ignores the file's own fill and takes its colour from
`background-color`, so one file serves every theme without duplicating the geometry.

### Rules

- Minimum size **16px**. Below that the courses merge; use the wordmark alone instead.
- Clear space on all sides is **half the mark's height**.
- Never re-draw the mark at a different brick count to "fit" a size. The proportions are the
  identity.
- Never place the mark on a coloured or photographic ground. It has no plate of its own by
  design; it needs `canvas` or `surface` behind it.
- Never combine it with the state colours. The mark is brand, not status.

### Wordmark

"easywall" is **not part of the SVG.** In the application it is HTML text set in the UI
typeface, so it inherits the type system and needs no font embedded in an image. The
previous `logo.svg` shipped a `<text>` element referencing IBM Plex Sans, which rendered
differently on every machine that did not have that font installed.

Where a self-contained lockup is genuinely required — README header, OG image — the wordmark
must be **converted to outlines** at export time. The OG image at
`web/static/og-image.png` is generated from `og-image.svg`; regenerate it whenever the
typeface changes.

## Do's and Don'ts

### Do

- Reserve green, amber and red for firewall state, always and only.
- Set every port, address, CIDR, timestamp and counter in monospace with tabular figures.
- Give every state both a colour and a word.
- Keep one primary action per view.
- Let hairline borders do the separating; keep panels flat.
- Self-host fonts; assume the machine has no internet access.
- Pair focus states with a visible border change, not just a glow.
- Implement all five input states. The error state is the one that matters most and the one
  most often skipped.
- Keep every interactive control on a shared height token.
- Attribute every state change in the audit log with a timestamp and a user.
- Make destructive and bulk actions ask once before acting.

### Don't

- Don't tint informational events. "Settings saved" is neutral, not blue.
- Don't use the accent for branding moments, empty-state art, or section decoration.
- Don't add a fourth state colour. If a new condition needs expressing, map it onto ok /
  warn / crit or express it in words.
- Don't reflow tables into stacked cards on mobile — a rule read as loose fields is no
  longer a rule.
- Don't fill destructive buttons. Border them.
- Don't let content run full-bleed on wide displays; the 1100px cap is what keeps a row
  scannable.
- Don't introduce gradients or glows. The one exception the previous system allowed — an
  atmospheric body gradient — was invisible at operating brightness and cost a repaint.
- Don't animate anything past 200ms, and don't loop anything that isn't genuinely ongoing.
- Don't auto-dismiss an error or a rollback notice. The operator acknowledges it.
- Don't put decision-relevant information in a tooltip.
- Don't use a toggle for a setting that only applies after Save.

## Known Gaps

- **The acceptance countdown is specified but cannot be built.** This document defines a
  `countdown` role at 40px for the acceptance window, and asks that *roll back now* be
  offered as plainly as *confirm*. The interface can deliver neither yet, and no timer is
  faked in its place. `shared.FirewallStatus` carries `active`, `acceptance`, `has_pending`
  and `last_apply` — no deadline — and `core.Acceptance` holds only a `time.Timer`, not the
  instant it fires, so there is no remaining-seconds value to render. There is also no
  rollback endpoint: the routes are `apply/start`, `apply/confirm` and `apply/status`.
  Until the core exposes a deadline and a rollback command, the apply screen states the
  escape route in words — doing nothing restores the previous rules — because that is what
  is actually true.
- **The audit log records almost no detail.** Every entry carries its action, rule type,
  user and timestamp, and the interface translates all of them. The `detail` column is a
  different matter: `internal/core` writes it empty for every save, import and apply,
  passing something only twice — the token `timeout` on a rollback, and the raw nftables
  error on a failure. So the column an operator would look at to answer "what changed?"
  is a dash almost every time. The demo used to paper over this with prose fixtures
  ("added 8443 (staging)"), which both made the product look more informative than it is
  and put English on screen that no locale could reach; its details now look like the
  core's. Recording what changed is a core change, not a design one, but it is the single
  most valuable thing the audit log is missing.
- **Charts.** There is no data-visualisation language yet. If traffic graphs or connection
  histories arrive, they will need a categorical palette that does not collide with the
  three state colours — a genuinely hard constraint given how much of the spectrum is
  already spoken for.
- **Density modes.** A comfortable/compact toggle has not been designed. The current scale
  targets a single density.
- **Empty and error illustration.** Empty states are currently type-only. Whether easywall
  wants illustration at all is undecided.
- **Light-mode accent distinctiveness.** `#0f7bab` sits close to the previous system's
  `#0891b2`. The light theme will therefore feel less changed than the dark one; revisit if
  the continuity is unwanted.
