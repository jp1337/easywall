---
version: alpha
name: easywall-graphite
description: |
  A dark-first control surface for a Linux firewall. The ground is a cool near-black
  (#0a0b0f) whose greys are bent a few degrees toward blue, so the chrome reads as one
  considered surface rather than as dead neutral. There is no accent hue. What is
  focused, what is selected and the one primary action on the page are carried by fill,
  edge and weight — action-fill, select-edge and focus-ring, every one of them ink.
  Everything else is carried by hairline rules and spacing.

  The discipline that shapes the whole system: green, amber and red are the entire
  palette, and they are the firewall's vocabulary. Green means a rule is live, amber
  means a change is unconfirmed, red means something was rolled back or is failing.
  Nothing else on the screen is coloured, and that is what makes the rule readable in
  both directions: if something is coloured, it is telling you the state of your
  firewall, and a screen with no colour on it is a screen with nothing to report.

  Network data — ports, addresses, CIDRs, timestamps, counters — is set in a monospace
  face with tabular figures throughout, because column alignment is how an operator
  scans a ruleset, and so is the name of the page you are standing on. Prose is set in
  a neutral grotesk. Panels are flat: borders do the separating, and shadow is reserved
  for things that genuinely float above the page.

colors:
  primary: "#f1f3f6"
  on-primary: "#0a0b0f"
  canvas: "#0a0b0f"
  surface: "#111318"
  surface-raised: "#181b22"
  rule: "#252932"
  rule-strong: "#333a47"
  control-edge: "#5e636e"
  ink: "#f1f3f6"
  ink-muted: "#a2aab8"
  ink-subtle: "#7d8593"
  action-fill: "#f1f3f6"
  action-ink: "#0a0b0f"
  select-fill: "#181b22"
  select-edge: "#f1f3f6"
  focus-ring: "#f1f3f6"
  state-ok: "#3ecf8e"
  state-warn: "#e5a54b"
  state-crit: "#f2555a"
  state-ok-on-wash: "#3ecf8e"
  state-warn-on-wash: "#e5a54b"
  state-crit-on-wash: "#f2555a"

typography:
  display:
    fontFamily: JetBrains Mono
    fontSize: 34px
    fontWeight: 300
    lineHeight: 1.2
    letterSpacing: "-0.01em"
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
    backgroundColor: "{colors.action-fill}"
    textColor: "{colors.action-ink}"
    typography: "{typography.body-strong}"
    rounded: "{rounded.md}"
    height: 32px
    padding: 0 12px
  button-primary-hover:
    backgroundColor: "{colors.ink-muted}"
    textColor: "{colors.action-ink}"
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
    borderColor: "{colors.focus-ring}"
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
  chip-select:
    backgroundColor: "{colors.select-fill}"
    textColor: "{colors.ink}"
    borderColor: "{colors.select-edge}"
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
    backgroundColor: "{colors.select-fill}"
    textColor: "{colors.ink}"
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
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.ink}"
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
    backgroundColor: "{colors.action-fill}"
    textColor: "{colors.action-ink}"
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
    backgroundColor: "{colors.action-fill}"
    textColor: "{colors.action-ink}"
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
    textColor: "{colors.ink}"
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
    # Amended 2026-09-11: this read borderColor: "{colors.select-edge}" and the
    # code has never painted a border here. app.css marks an enabled module with
    # box-shadow: inset 2px 0 0 — the same device as the active nav item, which
    # is why an operator reads the two as the same kind of "this one is on".
    # 2.16 only swapped the token in this entry and left the property, so the
    # mismatch predates it. The code is right: consistency with a shipped
    # pattern beats a token entry nobody implemented.
    edgeShadow: "inset 2px 0 0 {colors.select-edge}"
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

One rule governs this system. It is not a stylistic preference — breaking it makes the
interface lie about the firewall:

1. **Colour means state.** `state-ok`, `state-warn` and `state-crit` are the entire
   palette. Focus, selection and the primary action are carried by fill, edge and
   weight. A screen with no colour on it is a screen with nothing to report.

**Amended 2026-09-08: rule 2 is deleted, and deliberately not renumbered into
silence.** It read *"The accent is rationed — ice blue marks what is focused, what is
active, and the single primary action on a page. A page with ice blue in five places
has no accent at all."* 2.16 removed the accent, so there is no accent left to ration.
The rationing rule existed to stop a second hue competing with the three states; rule 1
now achieves that by there being no second hue. A reader who arrives remembering rule 2
should find out here what became of it, not find a file that was always right.

**A diff is structural, never chromatic.** The apply screen marks additions,
removals and edits with `+`, `-` and `~` in the mono column and neutral chips, and
the diff itself uses no hue at all. Green and red are firewall state: a new
blacklist entry is not good news and a removed port is not a failure. Of what
this release added to that screen, only the reachability verdict carries a
state colour — `state-ok`, `state-warn`, `state-crit`, each with the dot *and*
the word — alongside the acceptance status card's own dot, which already used
the same three roles before this release.

A consequence worth stating explicitly: **there is no informational colour.** Earlier
iterations tinted "settings saved" and "rules imported" log entries sky blue and
indigo. Neither hue was one of the three states, so a coloured log tag became
ambiguous — was it a state, or just emphasis? Informational events are now neutral
chips. Only the three states get colour.

## Colors

The greys are bent toward blue. The original reason was harmony with an ice-blue
accent, and that accent is gone; the bend stays, because a pure neutral grey under
green, amber and red reads as unconsidered in exactly the same way — two unrelated
systems in one frame. With the three state hues now the only colour on the screen, what
they sit on matters more than it did, not less.

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
| `action-fill` | `#f1f3f6` | Fill of the one primary action on a page |
| `action-ink` | `#0a0b0f` | Text and icons on an action fill |
| `select-fill` | `#181b22` | The confirming fill under a selected item |
| `select-edge` | `#f1f3f6` | The edge that marks a selected item |
| `focus-ring` | `#f1f3f6` | Where the keyboard is |
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
| `action-fill` | `#0f1116` | Fill of the one primary action on a page |
| `action-ink` | `#ffffff` | Text and icons on an action fill |
| `select-fill` | `#f4f6f9` | The confirming fill under a selected item |
| `select-edge` | `#0f1116` | The edge that marks a selected item |
| `focus-ring` | `#0f1116` | Where the keyboard is |
| `state-ok` | `#12855c` | Rule live |
| `state-warn` | `#96620d` | Unconfirmed change |
| `state-crit` | `#cf2d38` | Rolled back or failing |
| `state-ok-on-wash` | `#0f714e` | Text on a 10% `state-ok` wash |
| `state-warn-on-wash` | `#88590c` | Text on a 10% `state-warn` wash |
| `state-crit-on-wash` | `#ba2832` | Text on a 10% `state-crit` wash |

### Why five tokens carry three values

**Four of these carry the same value as `ink`, and the fifth the same as
`surface-raised`.** That is not a duplication to tidy away. `action-fill`, `select-edge`
and `focus-ring` answer three different questions — *what is the one action here*,
*which one is selected*, *where am I* — and a system that answers them with one token
cannot change one answer without changing the other two. The names are roles; the values
happen to agree today.

Measured against the surface each one lands on:

| pair | Dark | Light |
|---|---|---|
| focus ring / `surface` | 16.72:1 | 18.88:1 |
| action fill / `surface` | 16.72:1 | 18.88:1 |
| *before 2.16:* accent fill / `surface` | 11.40:1 | 4.73:1 |
| `select-fill` / `surface` | **1.08:1** | **1.08:1** |

The last row is the reason `select-edge` exists at all. A fill at 1.08:1 is the same
invisibility as the focus ring this release replaced, which measured 1.31:1 composited —
a difference no operator can see, in either theme, on either ground. `select-edge`
carries "this one is active"; `select-fill` only confirms it once the edge has been
found. Any implementation that marks the active element with a background alone is
wrong, and `TestTheActiveNavItemCarriesAnEdgeAndNotOnlyAFill` is what says so.

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
both themes. This matters more here than in most products: the options page carries
fourteen protection-module toggles, and an operator has to be able to see at a glance which
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

**Amended 2026-09-08: removing the accent strengthens the options page rather than
weakening it.** The objection this section raises against itself above — that a page
where only the enabled toggles are visible reads as "everything is fine" — is the one
2.16 had to answer, because the *on* toggle was an accent fill. Measured against the
surface the track sits on, the accent-filled *on* state reached 11.40:1 in dark mode and
**4.73:1** in light. Ink-filled it reaches 16.72:1 and 18.88:1. The *off* state is
unchanged, still resting on `control-edge` at 3:1. Both halves of the page's question —
which protections are on, which are off — are therefore answered at least as clearly as
before, and in light mode the *on* state became roughly four times clearer. Verified by
rendering /options in both themes, not computed from the token table.

> **Deliberate schema extensions.** The official linter reports three groups of warnings
> against this file, all expected:
> - `borderColor` is not in the component schema. In a system where hairline borders — not
>   shadow — do the separating, dropping border colour from the token layer would move the
>   single most structural decision into prose where tooling cannot see it.
> - `layout` and `motion` are not recognised token maps. Both are documented here because
>   control heights and transition durations are design decisions, not implementation
>   details; component entries carry literal values so nothing depends on the extension
>   resolving.
> - The contrast rule cannot composite alpha: it reads a state wash — `chip-ok`'s
>   `rgba(62,207,142,0.10)`, and the two beside it — as opaque and compares it against
>   itself, reporting 1.00:1. Composited over its real surface the same pairs measure
>   7.4:1 to 8.7:1. Verify contrast by compositing, not by trusting that rule.

> **Disabled controls** sit at 4.17:1 in light mode, below AA. This is intentional and
> permitted — WCAG exempts inactive components, and reduced contrast is the affordance that
> communicates "disabled". Do not "fix" it by darkening the text.

Note the inversion between themes: in dark mode `surface` is **lighter** than `canvas`,
in light mode it is **whiter** than a faintly grey canvas. In both cases the panel
advances and the page recedes — the relationship is preserved even though the direction
flips. Do not "simplify" this by making light-mode canvas pure white; the panels would
lose their edge.

**The accent used to shift hue between themes on purpose**, because a pale ice blue that
sings on near-black turns illegible on white, so light mode carried a deeper, more
saturated blue in the same role. That tuning went with the accent. The tokens that
replaced it do not shift — they invert: `action-fill`, `select-edge` and `focus-ring` are
`ink` in both themes, which is near-white on the dark ground and near-black on the light
one. A role defined as *the strongest thing the palette has* needs no per-theme
correction; it follows the ink, and the ink is already inverted.

## Typography

Two families, split by what the text *is*:

**Inter for what is read. JetBrains Mono for what is identified.** Sentences,
descriptions, button labels and form labels are read. Ports, addresses, counters,
timestamps, the countdown and **the name of the page you are standing on** are
identified. Nothing between the two extremes of the scale is mono.

- **Inter** — headings within a page, labels in sentence case, descriptions, button
  text. A neutral grotesk chosen for its small-size clarity and its genuinely tabular
  figures.
- **JetBrains Mono** — network data, the page title, and uppercase micro-labels. IPv4
  and IPv6 addresses, CIDR prefixes, timestamps, counters and raw nftables rules are set
  in it, always with tabular figures.

**Amended 2026-09-08.** The rule used to read *Inter for language, JetBrains Mono for
network data*, which put the page title in Inter alongside every sentence beneath it. A
page title is not language an operator reads: it is the label of where they are
standing, scanned the way a port number is scanned, and it is the one string on the
screen that answers *which page is this*. The countdown had already proved the voice
works — 40/300 mono, the largest type in the system — and it appeared on exactly one
screen in the whole product. The title carries it to every page.

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
| `display` | 30–34 / 300, mono, `-0.01em` | Page titles |
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

**`display` is a range because its lower bound is a ceiling reached by measurement.**
Above 900px the scale gets the 34px it wants. Below that the phone governs, and the
constraint is not the longest title: that is 23 characters — *Changer le mot de passe* —
and it wraps at its spaces. The constraint is the longest **unbreakable** title, 19
characters, `Systemeinstellungen`, a German compound with no break point anywhere in it.
JetBrains Mono advances 0.6em, so 19 characters is 11.4em, and roughly 358px is available
inside the padding at a 390px viewport, which puts the ceiling at 31.4px on paper, and 31px measured against the loaded face — the string is 342px at 30px against 362px available, so it fits with 20px to spare. 30px is that
number with the rounding taken off. Verify it by rendering /settings in German at 390px,
not by re-doing the arithmetic.

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

The uppercase mono `label` survives in one role: separating groups in a sidebar rather than
titling content. In the application that is the *Rules* and *System* dividers, twice. The
documentation site's sidebar uses the same device for the same reason — five groups, each
one a `<summary>` that opens and closes — and two of them are called *Rules* and *System*
on purpose: a reader looking at Ports in easywall finds its page under the label the product
put it under. The smallness is the point in both.

**The documentation sidebar adds a hairline and an indent.** `label` at `text-subtle` and a
nav link at `text-muted` sit twelve steps apart per channel — distinct on paper, the same
grey once rendered, and with the label the *lighter* of the two the label read as the
weaker element rather than the divider it is. Colour alone does not carry the separation, so
each group now opens with a `rule`-weight `border-top` above the label and its links sit
indented, so the eye reads container-then-contents instead of two rows of equal-weight text.
The first group carries no border — it follows the ungrouped Home and Overview links, not
another group, and a rule there divided nothing.

### Measure

**Explanatory body text is capped, in `ch`, and the cap is never `none`.** A sentence an
operator reads gets a ceiling on its measure, stated in `ch` so it follows the loaded face
rather than a viewport. Tables and data are not text and take no cap — that is the Layout
section's rule and it stands. The ceiling is per element, and every one of them is between
52 and 68:

| Element | Cap | What it carries |
|---|---|---|
| `.hero-note`, `.verdict-note` | 60ch | The sentence explaining a state, on the dashboard and on the apply screen. Translated, and unbounded in length |
| `.apply-lead` | 52ch | The lead-in above the three-step sequence, one authored paragraph per verdict |
| `.page-subtitle` | 68ch | One authored line of orientation under the page title. It binds: German `apply_subtitle` is 107 characters and wraps |

**Added 2026-09-09, from a measurement rather than a reading.** `.hero-note` and
`.verdict-note` had no cap at all, and this document had no rule for them to violate — the
gap was here, not in the stylesheet. `.hero-note` used to carry `dashboard_hero_active`, 46
characters, and a measure nobody had to think about. 2.17 put the health reason in it.
Rendered on `/dashboard` at a 1920px viewport the note measured **927px, carrying 151
characters on one line** — roughly 143 characters of prose. The guidance is under 80.
German is worse: `health_reason_stateful_dead` is 177 characters.

**60ch is the largest cap whose longest rendered line stays under 80 characters.** Measured
in Chromium over every string those two selectors can carry, in every locale that ships,
counting the characters in each line box the browser actually produced. Both selectors set
at 13px, so the pixel column is that size:

| Cap | Renders at | Longest line |
|---:|---:|---:|
| 52ch | 416px | 69 chars |
| 56ch | 448px | 74 chars |
| 58ch | 464px | 75 chars |
| **60ch** | **480px** | **79 chars** |
| 62ch | 496px | 82 chars |
| 64ch | 512px | 83 chars |
| 68ch | 544px | 85 chars |

**Do not re-derive a cap from the `ch` metric.** One `ch` is the advance of `0`, and Inter
resolves that to 8px at 13px, but prose averages nearer 6px per character. A `ch` cap is
therefore always looser than its number reads, and the arithmetic misses by two or three
characters — which is the whole margin here. The number to trust is the rendered one.

A cap is not a width. `dashboard_hero_active` still sets at its own 285px, and the healthy
reason still fits on one line. Nor does a cap fight the phone: 480px is wider than a whole
390px viewport, so it never binds there. German wrapping to three lines is the cap working,
because three short lines read better than one line of 143 characters. Verify it by
rendering `/dashboard` and `/apply` in German, not by re-doing the arithmetic.

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

Most pages here carry few rows — eight ports, four forwards, fourteen switches. Run that alone
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
"still happening": the pending status dot and the acceptance-window countdown.

**Amended 2026-09-02.** Not both at once. Wherever the pending dot sits beside a
running countdown — the acceptance screen, and the topbar chip that carries the
same countdown on every other page — the dot does not pulse, because the
countdown is already the motion that means "still happening" and says it more
precisely. Permitting both loops was never requiring them. Everywhere else the
dot pulses as before.

A spinner that outlives its request is a lie about the state of the system.

Honour `prefers-reduced-motion: reduce` by dropping all transitions and both loops to a
static state — the pending dot stays visibly amber, it simply stops pulsing. The
information must never live in the animation alone.

## Components

### Buttons

One primary action per view, filled with `action-fill` and lettered in `action-ink`: the
strongest pairing the palette has, which is why it needs no hue to be the loudest object
on the page. Hover moves the fill to `ink-muted` rather than reaching for a second tint —
`action-ink` on it measures 8.41:1 in dark mode and 6.15:1 in light, so the hover state
is a change of weight the button survives rather than a colour it borrows.

Everything else is `button-secondary`: surface fill and a 1px `control-edge` border —
hairline in weight, but not in contrast, since that border is the only thing separating
the button from the panel behind it. Destructive actions — *Roll back now*, *Delete
rule* — use `button-danger`: a bordered button in `state-crit`, never a filled red block.
A filled red button is the loudest object on the page and invites the misclick it is
warning about.

### Chips and tags

Chips carry either state or emphasis, never both. `chip-ok` / `chip-warn` / `chip-crit`
state the firewall's condition. `chip-select` marks a qualifier the operator chose, such
as a scope restriction — `select-fill` behind it and a `select-edge` border, the same
vocabulary the active navigation item speaks. It was `chip-accent` until 2.16, and the
rename is the point: what the chip marks is a selection, and never was a hue.
`chip-neutral` is everything else, including all informational audit-log actions.

### Tables

The workhorse. Header cells use `column-label`; value cells carrying network data use `data`
with tabular figures. Rows separate with a single `rule` hairline and lift to
`surface-raised` on hover. Right-align nothing except pure counts — port numbers read better
left-aligned against a left-aligned description.

**A data column is sized by its widest documented value, measured rendered rather than
added up on paper.** `.col-flex` absorbs the remainder.

That was never written down, and each instance therefore came back as a defect. `.col-toggle`
sat at 150px bought entirely by its header string until the header was allowed to wrap and
the true floor turned out to be 94px. `.col-sources` is 25rem because the catalogue's own
default measures 307px rendered and clipping it drops `fc00::/7`, the range that exists so
IPv6 does not lock anyone out. `.col-port` at 120px against a documented `8000:9000` needing
122 was the third, and `carried-forward.md` called it a decision this file did not answer.
It is answered here now, and it was never a zero-sum choice: `.col-flex` is `width: auto`,
so every pixel a data column gives back goes to the description an operator is scanning.

Every table sits under a `toolbar`: the count on the right, the add action beside it, and a
filter field where a set can grow past a screenful. Filtering happens in the browser — the
rows are already there, and a round trip would discard unsaved edits.

A destructive row action rests at 55% opacity and comes fully forward on row hover or
`:focus-within`. Never at zero: a control that only exists on hover cannot be found by
someone tabbing through, and on a touch screen there is no hover at all — below the reflow
breakpoint it is always at full strength.

### The dashboard's two ranks

Six tiles were never six of the same thing. Three are ways into the host — its addresses,
its interfaces, whether the daemon answers — and three change what the rules are. One
grid claimed otherwise, and produced a page whose most consequential controls sat beside
its least. Rank one keeps the tiles. **Rank two is a list** — no border, no radius, no
icon — because three numbers are not three states, and a card is what this system uses
for something that has one.

A tile is an `<a>`. It carries no *Manage →* affordance of its own, and that link was
added and removed six times across the design reviews for the same reason each time: the
whole tile is already the control, so the arrow is a second copy of it sitting inside it.

The unused-port finding used to live inside the tile's note, at 12px. It now takes its
own line under a hairline in the tile's footer, because something an operator is meant to
act on does not belong in the smallest type on the page. It is positioned rather than
added as a fourth grid child: the tiles share a subgrid spanning a fixed row count, and a
conditional child would give three tiles two different heights.

### Protection modules

A firewall protection is either on or off and may carry its own parameters. As rows in one
long list — which is how it was first built — fourteen of them took 1700px of scroll and
answered none of the page's actual question: *which protections are active right now.*

**Fourteen cards, eleven with parameters.** Both numbers appear in this file and they count
different things; `options.html` renders fourteen `class="module"` cards and eleven
`class="module-params"` blocks. *Eleven toggles* was wrong in three places until 2026-09-11.

Each is a card in an `auto-fill` grid at `minmax(330px, 1fr)`. Name and switch in the header,
one line saying what the module does, and parameters below a hairline **inside the card** —
not on a darker band underneath it, which read as a detached second row. An active module
carries a 2px `select-edge` inset on its left edge: the same device that marks the active
nav item, so "this one is live" speaks one vocabulary throughout. The header is a `<label>`
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

An action carries colour when, and only when, it says something about what the firewall is
doing. Saving a rule set stages it and changes nothing that is live, so it stays neutral
however important it feels. `internal/web/server.go`'s `auditActionTones` is the list, and
`TestOnlyFirewallStatesCarryATone` holds a copy of it with the judgement written out, so an
addition is a deliberate edit rather than a diff nobody reads.

**Amended 2026-09-08.** This paragraph read *"Only the four `apply_*` actions carry colour"*
and had been wrong for two releases: the code coloured eleven — the four applies,
`rollback_failed`, three `boot_*`, two `panic_*` and `resume_restore_skipped` — and
`features/audit-log.md` listed all eleven under a heading that counted them. 2.17 adds
`health_degraded` as the twelfth. The rule was never a count of `apply_*`; it is the sentence
above, and stating it as a list of four is what let the file go stale while remaining
plausible. It also disagreed with the *Status* section below, which reserves nothing for
`apply_*` in particular. `docs-tech/carried-forward.md` carries two more disagreements of
this shape; this one is closed rather than joining them, because 2.17 is the release that
changed the table.

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

**Amended 2026-09-02.** The `pending` state carries no icon. It used to show a
48px circular clock glyph, which above a running clock is a drawing of the thing
standing beside the thing; the countdown takes that slot and the signature gets
its weight without anything being added. `accepted`, `rolled_back` and `idle` keep
their icons.

Five defaults were considered and refused, each by a rule already in this file: a
depleting ring (the system is flat, separated by hairlines), a green→amber→red
ramp (rule 1 — `state-crit` means rolled back, unreachable, validation failed), a
pulsing glow (decoration; the digits are the motion), a progress bar (says what
the number says, less precisely), and confirm-large/rollback-as-a-link (forbidden
above, in those words).

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
| Focus | `focus-ring` | `canvas` | Plus a 2px `focus-ring` outline at 2px offset |
| Error | `state-crit` | `canvas` | Message below in `state-crit`, never colour alone |
| Disabled | `rule` | `surface-raised` | Text drops to `ink-subtle`; inactive, so exempt |

Focus is never removed. It is a **border change plus an outline** — not a glow, and never
colour alone, because an operator navigating by keyboard has to see where they are on a
dense page of ports.

**Amended 2026-09-08.** Focus was measured across all 13 pages by tabbing to every
focusable element and compositing the indicator against the backdrop it actually lands
on, rather than against the token it nominally sits over. The failing set was one shape:
a ring with no border change under it. `.toggle`, `.checkbox`, `.theme-toggle`, `.link`,
the three editor textareas and `.f-ssh` — 66 elements per theme — drew a translucent ring
that composited to 1.31–1.34:1. Every control whose border moved on focus already cleared
3:1. That is the argument for the border change being part of the specification rather
than a nicety layered on top of the outline: the outline is what the eye finds, and the
border is what survives when the outline lands on a surface it cannot separate from.

`fieldset` groups related settings and carries its legend in `label` type. This is the
dominant structure on the options and settings pages; a flat list of forty controls with no
grouping is unusable, and the grouping is what makes the protection modules scannable.

### Toggles and checkboxes

The protection modules are the largest cluster of controls in the product — fourteen toggles
on one page, eleven of which carry their own parameters. A toggle means "this protection is
on"; it is the only place besides the primary button where `action-fill` appears as a fill,
and that is deliberate: an operator scanning the options page should be able to see at a
glance how much protection is enabled.

Checkboxes are for selection, toggles are for state. Never use a toggle for something that
only takes effect after pressing Save — the toggle's own animation promises immediacy.
Where a toggle sits behind an explicit apply step, pair it with the unconfirmed-change
indicator rather than letting it imply the rule is already live.

### Navigation

Sidebar groups open with a `rule`-weight hairline above their label, and their links are
indented beneath it, so the eye reads container-then-contents instead of two rows of
equal-weight text. The device is the documentation sidebar's, for the reason this file
already gives about it under *Typography*: `label` at `ink-subtle` and a nav link at
`ink-muted` sit twelve steps apart per channel — distinct on paper, the same grey once
rendered, and with the label the *lighter* of the two it read as the weaker element
rather than as the divider it is. Colour never carried that separation, and after 2.16
there is less colour available to carry anything.

**The first labelled group carries neither device.** It follows the ungrouped *Dashboard*
link rather than another group, and a rule there divides nothing. The rule is therefore
written between labelled siblings rather than on a class, so a fourth group added later
gets both devices without anyone having to remember them.

The active item takes a `select-fill` background, `ink` text at 550 weight, and a 2px
`select-edge` inset on its left edge. The inset sits out in the list's own gutter, so it
marks the group's left edge rather than the text's, which is what makes the indent read
as containment rather than as a second margin. The fill on its own marks nothing — see
the 1.08:1 measurement under *Colors*.

`carried-forward.md` held this across three design reviews as deliberately not touched.
2.16 closes it.

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
`German`; `Français`, not `French` — and there is still no "Language:" label to precede it: the field would only be
useful to someone who can already read the interface.

**The select is drawn, not left native.** It takes `appearance: none`, a chevron built
from two borders on a pseudo-element — which takes `currentColor`, so it follows the theme
with no second asset to keep in step and nothing for `style-src 'self'` to object to — and
a `surface-raised` fill. The fill is not decoration: this file already names the failure
under *Why `control-edge` is separate from `rule-strong`*, that a field's `canvas`
background sits 1.03–1.06:1 from the `surface` of the card it is on. In the sidebar that
was survivable. On the login card, where this control also appears, it left the select
recognisable only by its native arrow — which is exactly the arrow `appearance: none`
takes away.

Drawn only when more than one locale is installed. A single option that cannot change
anything is a control that lies about having a choice.

### Links and badges

`link` is `ink` with a **permanent** underline; the underline carries the affordance and
hover thickens it. The old reasoning here — `accent-on-wash`, the deepened accent, because
a pale ice blue in running text lacks contrast in light mode — went with the accent, and
with it went the last argument for a link being a colour at all. A link in running text
that is only a hue fails the same way a state that is only a dot fails.

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

`web/static/icon.svg` is the source of the mark's geometry.
`docs/assets/img/icon.svg` is a byte-identical copy, because Jekyll serves the
site out of `docs/` and the application serves its own assets out of
`web/static/`. `TestTheMarkHasOneGeometry` keeps the two identical, so the
copy cannot drift into a second mark.

`web/static/icon.svg` is applied three ways:

| Context | Mechanism | Colour |
|---|---|---|
| App chrome (sidebar, auth cards) | CSS `mask` on a `<span>` | `ink` |
| Favicon (SVG) | Browser loads the file directly | Adapts via `prefers-color-scheme` inside the file |
| Favicon (PNG fallback), docs, README | Raster or `<img>` | Baked: `#0f7bab`, the one tone legible on both light and dark tab bars |

The mask matters. An SVG loaded through `<img>` is a separate document and **cannot inherit
`currentColor`** — an early `.brand-icon { color: var(--accent) }` rule was silently doing
nothing. A mask ignores the file's own fill and takes its colour from `background-color`,
so one file serves every theme without duplicating the geometry.

**Amended 2026-09-08.** The app chrome's mark is `ink`, because there is no accent left
to apply to it. The SVG favicon still adapts through `prefers-color-scheme` inside the
file. **The baked raster favicon and the documentation's copy of the mark stay
`#0f7bab`**, and this is stated rather than left to be discovered: they already differed
from the app chrome in dark mode, where the chrome drew the mark in pale ice blue and the
raster in a deeper one, so 2.16 did not open the divergence, only changed which colour
sits on the other side of it. Re-rendering the raster set and the OG image was not in this
release's scope; it is listed under *Known Gaps*.

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

## The documentation site

The site at easywall-project.org and the application share these tokens and both
typefaces, and they arrive at pages that look deliberately unalike. The reason is stated
here so the difference does not read as an accident: **the application reserves colour
for state, and the documentation has no state.** Nothing on a page of prose is live,
unconfirmed or rolled back. Having nothing to report, it reports nothing, and so it has
no colour.

Links there carry an underline and weight rather than a hue — the same affordance the
application's `link` uses, for the same reason. `h1` takes the mono display voice, so a
documentation page announces itself the way an application page does. `h2` and `h3`
inside an article stay Inter, because a long-form page set in mono headings throughout
stops reading as prose and starts reading as a table.

An inline `<code>` may break, and only when it cannot fit a line on its own:
`overflow-wrap: break-word`. A code **block** never wraps — it scrolls. The
distinction is that an identifier a reader retypes must not gain a line break
they cannot see, and a 56-character Go test name must not push a phone-width
page sideways.

## Do's and Don'ts

### Do

- Reserve green, amber and red for firewall state, always and only.
- Set every port, address, CIDR, timestamp, counter and page title in monospace with
  tabular figures.
- Give every state both a colour and a word.
- Keep one primary action per view.
- Let hairline borders do the separating; keep panels flat.
- Self-host fonts; assume the machine has no internet access.
- Pair focus states with a visible border change, not just a ring. The ring is what the
  eye finds; the border is what survives a backdrop the ring cannot separate from.
- Implement all five input states. The error state is the one that matters most and the one
  most often skipped.
- Keep every interactive control on a shared height token.
- Attribute every state change in the audit log with a timestamp and a user.
- Make destructive and bulk actions ask once before acting.

### Don't

- Don't tint informational events. "Settings saved" is neutral, not blue.
- Don't reintroduce a hue for branding moments, empty-state art, or section decoration.
  Green, amber and red are the palette, and each of them already means something.
- Don't mark the active element with a background alone. `select-fill` is 1.08:1 against
  `surface` in both themes; `select-edge` is what an operator actually sees.
- Don't add a fourth state colour. If a new condition needs expressing, map it onto ok /
  warn / crit or express it in words.
- Don't fill destructive buttons. Border them.
- Don't cap the content width to keep a row scannable. The cap was tried and reverted;
  see *Layout*. Width belongs to the tables.
- Don't introduce gradients or glows. The one exception the previous system allowed — an
  atmospheric body gradient — was invisible at operating brightness and cost a repaint.
- Don't animate anything past 200ms, and don't loop anything that isn't genuinely ongoing.
- Don't auto-dismiss an error or a rollback notice. The operator acknowledges it.
- Don't put decision-relevant information in a tooltip.
- Don't use a toggle for a setting that only applies after Save.

## Known Gaps

- ~~**The acceptance countdown is specified but cannot be built.**~~ **Built in
  2.14.** `shared.FirewallStatus` carries `acceptance_remaining` and the protocol
  carries `CANCEL_ACCEPTANCE`, so the screen renders a real deadline and offers a
  real rollback. The paragraph that stood here across four releases described the
  gap accurately and understated how small it was — `Acceptance.Cancel` had
  existed since 2.7 and only lacked a route. See
  `docs-tech/specs/2026-09-02-2.14-the-window-shows-that-it-is-running.md`.
- **Charts.** There is no data-visualisation language yet. If traffic graphs or connection
  histories arrive, they will need a categorical palette that does not collide with the
  three state colours — a genuinely hard constraint given how much of the spectrum is
  already spoken for.
- **Density modes.** A comfortable/compact toggle has not been designed. The current scale
  targets a single density.
- **Empty and error illustration.** Empty states are currently type-only. Whether easywall
  wants illustration at all is undecided.
- **The mark is two colours across three surfaces.** The app chrome draws it in `ink`;
  the baked raster favicon, the OG image and the documentation's copy stay `#0f7bab`.
  They already differed before 2.16, and re-rendering the raster set and the OG image was
  not in this release's scope.
- **`.callout-info`'s blue stays.** *Info* is a state, and *colour means state* does not
  forbid a state from having a colour — so the callout set (info / warning / success)
  keeps its blue, amber and green washes. What was wrong was that all three washes and
  edges were literals rather than tokens: `--callout-info-wash` / `-edge`,
  `--callout-warning-wash` / `-edge` and `--callout-success-wash` / `-edge`, declared
  alongside the rest of the palette, now carry them.
