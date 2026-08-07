# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

Documentation site — six rendering defects, all of them visible only on screen:

- **Every highlighted code block was drawn as a box inside a box.** kramdown nests `div.highlighter-rouge > div.highlight > pre.highlight`, and the stylesheet gave a background, a border and a radius to *two* of them. It went unnoticed because the border colour measured about 1.05:1 against the fill, so each frame was individually almost invisible — the pair only read as a doubled edge. The frame now lives on the outer wrapper alone
- **Code blocks had no panel at all in light mode.** They were filled with `--surface` (`#ffffff`) on a `#fcfcfd` page. They now use `--surface-2` and `--border` in both themes
- **Light mode painted grey rectangles behind parts of every code block.** A `[data-theme="easywall-light"]` copy of the inline-code rule outranked the `.content-body pre code` reset on specificity (0,3,1 against 0,2,2), so the inline chip's background followed `<code>` into `<pre>`; being an inline box, it painted per line box. The override set the same value as the rule it was shadowing — it existed only to cause this
- **Bullets and numbers were missing site-wide.** Tailwind's preflight sets `list-style: none` and nothing restored it, which cost `configuration.md` the numbering of its language-priority list, where the order is the content
- **Diagrams and screenshots could show the wrong theme.** They were selected by `<picture>` with `prefers-color-scheme` — the operating system — while the site theme is a `data-theme` attribute set by the sidebar toggle. The layout tried to reconcile the two by reassigning `img.src`, which cannot work: a matching `<source>` always outranks the `src`. A reader on a dark OS who chose the light documentation got dark diagrams on a white page, with no way to fix it. Both variants are now in the markup and CSS picks one; `loading="lazy"` keeps the hidden one off the wire
- **Fourteen tables rendered a blank grey band** where the headerless markdown idiom (`| | |`) left kramdown emitting a `<thead>` of empty cells

### Changed

- **Diagrams are legible now.** They were stretched to the 880px text measure regardless of their own size, which scaled the 1597px deployment flow to 0.59 and its labels to about eight pixels, while blowing the narrow rule-order flow up nearly 2×. They now keep their intrinsic size, step outside the text measure when the viewport allows, scroll instead of shrinking on a phone, and are rendered at 17px rather than 14px
- `rule-states.mmd` used `<i>…</i>` in three node labels. The renderer sets `htmlLabels: false` — required, since mermaid's `<foreignObject>` output is not valid XML — so the published diagram read `<i>what you are writing</i>`, literally. `apply-flow.mmd` became a flowchart, which lays out in two thirds of the height and without the empty quadrant the state-diagram note placement left behind
- Callouts are no longer set in italic. That is fine for one line and slow to read across the multi-line ones on `security.md`, which are the paragraphs a reader most needs to get right
- Inline code is no longer accent-coloured. On `configuration.md` it made a page of keys and values read as a page of links, and left the real links with nothing to stand out against
- Reference tables scroll on a phone instead of squeezing to one word per line

Documentation site — layout and content:

- **Headings sat at a different left edge from the content below them.** Prose was held to an 880px measure centred in the available area, while tables and diagrams stepped outside it and centred on the same axis. Everything now runs full width between the sidebar and the contents column, sharing one left edge
- **The landing page's call to action had its three elements on two centre lines**, 104px apart, and a chapter divider drawn inside the card. Both came from the generic `h2` and `p` rules: those are unlayered, the card's rules are in `@layer components`, and an unlayered rule beats a layered one whatever the specificity — so raising the selector inside the layer changed nothing
- **The sidebar version badge read `v2.4`.** It was hardcoded in the layout and a patch release behind. It now comes from `docs/_config.yml`, as do the hero badge and the two pinning examples in `docker.md`, which named `v2.4.0`
- **A Playwright storage-state file was published** at `/assets/img/screens/state.json`, committed by accident with the screenshot set in 2.4.1. It holds a session cookie for a local demo instance, long expired and never valid off that host, but it had no business being served. Removed, and `state.json` is now ignored

### Removed

- **"How the public demo stays current"**, and its diagram. It documented how the project's own demo host is deployed — registries, the update daemon, the restart timer, the hostname — which is operational detail about someone's infrastructure rather than documentation for a reader. The parts about resetting your *own* demo remain; two other pages that leaked the same detail are reworded

### Added

- **An on-page contents column** on wide viewports, built from the rendered headings so it cannot drift from the page, and absent on pages with fewer than three. Earns its place on the long reference pages — `configuration.md` runs to about 5,600px
- `internal/web/docs_style_test.go` asserts that load-bearing rules survive into the built documentation stylesheet. Nothing had ever checked that file, and it has now broken twice in a way no build could catch — once when removing daisyUI took the page background with it, once when a mistyped comment terminator silently deleted the rule that hides the non-current theme's images. Each assertion was confirmed to fail with the defect reintroduced

## [2.4.1] - 2026-08-04

### Fixed

- **Dark mode on the documentation site.** daisyUI's base layer had been supplying the page background and `color-scheme`; removing daisyUI in 2.4.0 took them with it and nothing replaced them, so the site rendered dark components on a browser-white page. Light mode looked correct by coincidence, which is how it shipped
- **Every documentation diagram with a line break failed to load.** mermaid emits HTML inside a `<foreignObject>` for multi-line labels, and an unclosed `<br>` makes the file invalid XML. An SVG used as an `<img>` source is parsed strictly, so the whole picture silently became alt text. The renderer now parses each SVG before writing it and fails the build if it would not load
- **`go/bad-redirect-check`.** The last guard before a `Location` header tested for a leading slash and a second slash but relied on a backslash check twenty lines earlier. All three conditions now live in one function, `isLocalPath`, tested directly. The redirect target is rebuilt from a parsed path and query rather than the caller's string, which also closes percent-encoded slashes that a raw prefix check would have passed
- The documentation site was loading **mermaid from a CDN** on every page — the only third-party request on a site built to make none — configured with a palette and font that 2.4.0 removed. No page contained a diagram, so it had never rendered anything

### Changed

- **The documentation was rewritten around pictures.** 17,000 words with no diagrams and no screenshots became 12,000 with 32 diagram references and 12 screenshots of the real interface, in both themes. Diagrams are named `.mmd` sources in `docs/_diagrams/`, pre-rendered to one SVG per theme by `npm run build:diagrams`; `npm run check:diagrams` fails if a source changed without a re-render
- **`csrf_key` is gone from the shipped `web.toml`.** Nothing has read it since `net/http.CrossOriginProtection` replaced the token scheme. An existing config keeping the key is unaffected — it is ignored
- CI action bumps: `actions/download-artifact` 4 → 8, `docker/login-action` 3 → 4, `actions/checkout` 6 → 7, `codecov/codecov-action` 6 → 7

### Documentation corrections

Eight statements that were not true, several of them security-relevant:

- **The audit log does not record logins.** `security.md` listed `login_success`, `login_failed` and `logout` among its event types. Nothing writes them, and nothing ever did — a reader relying on that page would believe failed logins were on record. Use `journalctl -u easywall-web` instead
- **The audit log's JSON shape** was documented as `{time, event, user, ip, scope, reason}`. The core writes `{time, action, rule_type, detail, user}`
- **CSRF is not `gorilla/csrf`** — that is not a dependency. It is Go 1.25's `net/http.CrossOriginProtection`, checking `Origin` and `Sec-Fetch-Site`. The claim appeared twice
- **`csrf_key`** was documented as a required secret and requested by the manual install
- **"Rule injection is structurally impossible"** overstated it: true of the apply path, but custom rules do reach `nft -f -`, over stdin, inside the privileged core
- **Certificates** are ECDSA P-256 only, not "RSA-4096 / ECDSA P-256"
- **The docs stack** is Jekyll, not MkDocs Material
- **The demo indicator** is a neutral chip, not an amber banner — amber is reserved for firewall state

Also now stated rather than omitted: the audit log's `detail` column is empty for every save and apply, so the column that should answer *what changed* is almost always a dash.

[2.4.1]: https://github.com/jp1337/easywall/compare/v2.4.0...v2.4.1

## [2.4.0] - 2026-08-03

### Added

- **Public demo at [easywall.wdkro.de](https://easywall.wdkro.de)** — login with `demo` / `demo`. Auto-redeployed on every successful CI build of `main` via the new `publish-edge` workflow + Watchtower hook on the demo host. Container restarts every 6 hours to wipe accumulated visitor state. Linked from the hero CTA and the bottom CTA card on the homepage
- **Multi-registry container publishing** — every release is now pushed to **GitHub Container Registry, Docker Hub, and Quay.io** simultaneously. Same multi-arch (`linux/amd64` + `linux/arm64`) image, byte-for-byte identical across all three mirrors. Pull from whichever is closest to your environment
- **`:edge` rolling tag for the public demo** — new `.github/workflows/publish-edge.yml` builds and publishes after every successful CI on `main`, then triggers Watchtower on the demo host to pull the new digest. The four-tag scheme (`:latest` for releases, `:vX.Y.Z` for pinning, `:edge` for nightly, `:sha-<commit>` for rollback debugging) is documented on the Docker installation page
- Documentation site (jp1337.github.io/easywall) now uses **the same stack as the app**: Tailwind CSS v4 with the same palette, fonts and `easywall-dark`/`easywall-light` theme tokens as the running web UI. Previous 693-line hand-rolled stylesheet replaced by `web/src/docs.css` compiled to `docs/assets/css/style.css` via `make docs-css`
- **Demo mode** — set `demo_mode = true` in `web.toml` and `easywall-web` runs against an in-memory mock instead of the Unix socket. No `easywall-core` process, no root privileges, no nftables dependency. The state machine seeds itself with realistic example data and supports every page (rules, options, settings, system, audit log, apply/accept/rollback). Designed for hosting a public demo so visitors can explore the UI without affecting a real firewall. State resets when the process restarts. A topbar banner makes the demo status visible on every page
- **Language switch in the interface** — one button per installed locale in the sidebar footer, and on the login and first-run cards, so an operator who cannot read the interface can still change it before signing in. The choice is stored in an `easywall_lang` cookie for a year and outranks `Accept-Language`. Each locale names itself through a `language_name` key, so a language always appears under its own name. Adding `locales/<lang>.json` is all it takes for it to appear
- **The interface is fully translated.** Every visible string — page copy, context cards, placeholders, `aria-label`s, empty states, toasts, audit actions, validation messages — goes through the message catalogue in English and German. Sentences containing a link or a literal stay one message with `{}` and `` ` `` markers so a translator controls word order
- `<html lang>` now reports the language actually served instead of always `en` (WCAG 2.1 SC 3.1.1)

### Changed

- **The interface was rebuilt on a written design system.** `DESIGN.md` in the repository root is now the single source of truth for colour, typography, spacing, radii, motion and components, validated with `@google/design.md`. daisyUI is gone: it contributed 14 components against 107 hand-written rules, and every exact requirement in the spec — control heights, the focus ring, control outlines — was an override. Tailwind v4 stays, with the tokens declared once in `@theme` so a template names `bg-surface`, never a colour. The compiled stylesheet dropped from 95 KB to 48 KB
- **Graphite + ice palette, dual theme.** Green, amber and red are reserved for firewall state — live, unconfirmed, rolled back — so colour in the interface always means something about the firewall. The accent marks only what is focused, what is active, and the one primary action
- **Fonts are self-hosted.** Inter and JetBrains Mono are subset and served from `web/static/fonts/`, so `style-src` and `font-src` are now `'self'` with no exceptions. The interface no longer makes a third-party request, and typography survives on an air-gapped host
- Pages are full-width with a context column beside the rule editors, tables reflow into labelled cards below 720px, and the protection modules on the options page are a self-sizing card grid rather than a single column of two-storey rows
- `language` in `web.toml` is now the *fallback* locale rather than the default. An explicit choice in the interface wins, then `Accept-Language`, then this setting

### Fixed

- **Live validation rendered unstyled.** The blacklist, whitelist and custom-rule editors emitted `alert-success`, `alert-error`, `alert-info` and `alert-soft` — daisyUI class names that stopped existing when daisyUI was removed. Every validation response an operator has seen since was a box with no colour, on the three pages where "did that parse?" is the entire question. The tests asserted the same dead names and passed throughout
- **The audit log's colour coding never worked in production.** The stylesheet keyed on `rules_applied` and `rules_rolled_back`, names only the demo client produced. The core writes `apply_accepted`, `apply_rolledback`, `apply_started` and `apply_failed`, so a rolled-back apply — the most consequential line in the log — rendered neutral grey
- **The demo told every visitor their nftables syntax was valid.** It has no `nft` binary and answered "no errors" whatever was typed. It now reports the checker as unavailable, which is true. The documentation claimed a notice already made this clear; it did not
- **Text fields and secondary buttons failed WCAG 2.1 SC 1.4.11.** Their borders measured 1.20–1.35:1 where the criterion requires 3:1, and the field fill sits ~1.05:1 from the panel behind it, so the border carried the whole affordance. On the login page the password field was effectively an unmarked rectangle. Toggles and checkboxes had already been fixed; every other control was missed
- **`docs/features/blacklist.md` stated twice that the whitelist overrides the blacklist.** The code drops blacklisted sources first, as the same page's own ordering table shows. A reader trusting the prose could have locked themselves out
- The audit log filter searched only the stored identifier, so typing the wording shown on screen returned nothing
- Entry counters in the list editors counted comment and blank lines as entries
- The theme switch had no accessible state and a label that named neither of its two positions; it is now a `role="switch"` labelled "Light mode"
- Light-mode `ink-subtle` failed AA on two of three grounds; raised to `#666e7b`
- `--form-max` was referenced by `.content-narrow` and `.form-stack` but never declared, so neither had any effect
- The first-run screen printed its own subtitle again as a notice; it now carries the one thing an operator cannot look up once locked out — that this is the only account, and resetting it needs shell access

[2.4.0]: https://github.com/jp1337/easywall/compare/v2.3.0...v2.4.0

## [2.3.0] - 2026-05-03

### Added

- DaisyUI 5.5 component library + HTMX 2.0 are now part of the web UI build. All 15 templates use DaisyUI primitives (cards, buttons, alerts, fieldsets, toggles, tables, badges, tabs, steps); the custom CSS in `web/src/app.css` now contains only layout-specific chrome
- New "Aurora Operator" color palette — analogous cool-tone scheme with deep slate-blue chrome and cyan-400/teal-400 accents in dark mode, white + cyan-600/teal-600 in light mode. Status colors: emerald (success), amber (warning), rose (error), sky (info). Replaces the previous orange/navy complementary pair which created visual tension
- Custom rules now validate **live** as you type — the textarea sends the content to `POST /custom/validate` (HTMX, 600ms debounce) and per-line syntax errors appear inline without a form submit. Falls back to a soft notice when the core daemon is unreachable
- Blacklist & whitelist editors now validate **live** as you type — invalid IPv4/IPv6/CIDR entries are listed by line number under the textarea via the shared `POST /iplist/validate` HTMX endpoint
- Audit log page now has a **search filter** — type in the search box above the table and rows are filtered live (case-insensitive substring match across action / rule type / detail / user) via `GET /log/filter`. The filter operates on the loaded 200 entries; older history is read directly from `audit.log`
- `/options`, `/settings`, and `/system` now **auto-save on change** — toggle a switch or change a numeric input and the form is silently submitted via HTMX, with a small toast notification appearing in the bottom-right corner ("Saved" / "Save failed"). The traditional Save button is still present for graceful degradation when JavaScript is disabled
- Custom rules syntax validation also runs on save — the web UI validates raw nftables rules via `nft --check`; per-line errors are displayed inline in the editor (was already in 2.2 release flow, now used by the live-validation endpoint too)
- Tailwind CSS v4 UI — the web interface now uses a purpose-built "Operator Interface" design with Outfit UI font and JetBrains Mono for IPs/rules; replaces the previous IBM Plex stylesheet
- `make css` target and CI steps compile the Tailwind source in `web/src/app.css` to `web/static/style.css` during build

### Fixed

- Custom rules in `state.Current.Custom` are now actually applied to the nftables kernel after the typed rules flush; previously the slice was stored and validated but never passed to `nft`

[2.3.0]: https://github.com/jp1337/easywall/compare/v2.2.0...v2.3.0

## [2.2.0] - 2026-04-28

### Added

- Audit log viewer (`GET /log`) — the core's per-change `audit.log` is now accessible from the web UI in a table showing timestamp, action, rule type, detail, and user; most-recent entries first (up to 200)
- Dashboard rule-count cards — TCP port count, UDP port count, blocked IPs (blacklist), and allowed IPs (whitelist) are now shown as stat-cards on the dashboard, each linking to the relevant management page
- `GET/POST /system` — acceptance window duration and enabled flag are now configurable from the web UI without editing `easywall.toml`

## [2.1.0] - 2026-04-27

### Added

- Firewall protection options (`[firewall]` config section) are now editable directly from the web UI via `POST /options`; changes are persisted atomically to `easywall.toml`
- `GET/POST /password` — administrators can change their password from the web UI without editing config files
- `GET/POST /settings` — IPv6 support flags and Docker network integration settings (`[ipv6]`, `[docker]` config sections) are now editable from the web UI
- Option toggle switches on the Options and Network Settings pages now update their status icon live when toggled (no page reload required)

### Fixed

- IPv6 CIDR rules in blacklist and whitelist now correctly generate nftables expressions with `NFPROTO_IPV6` protocol-family guards in the `inet` table; previously IPv6 CIDRs were silently skipped
- IPv6 single-address whitelist entries now produce an accept rule (the branch was missing entirely)
- Docker custom networks using IPv6 CIDRs are now handled correctly
- CSP nonce added to the inline theme-init `<script>` in `login.html` and `firstrun.html`; the script was previously blocked by the `script-src` policy on those pages
- Removed remaining inline `style=` attributes from auth templates that were blocked by `style-src` without `'unsafe-inline'`
- Removed unused htmx CDN script from base template; the script tag was blocked by CSP and no `hx-*` attributes were used anywhere
- Apply status polling no longer stops at `accepted` state; the backend resets to `idle` immediately after acceptance, so the UI now transitions naturally without getting stuck

### Changed

- CSP `script-src` and `style-src` no longer contain `'unsafe-inline'`; inline scripts use per-request nonces instead
- GoReleaser Docker configuration migrated from deprecated `dockers` + `docker_manifests` to `dockers_v2`
- CI build workflow updated: Debian package step uses `-d` to skip Go build-dependency check and artifacts are moved to `dist/` before upload

## [2.0.0] - 2026-04-26

### Added

- Complete rewrite of easywall from Python to Go (requires Go 1.25, no Python dependency)
- Two-process architecture: `easywall-core` (root, nftables via netlink) and `easywall-web` (unprivileged HTTPS UI)
- Unix socket IPC between core and web processes with typed JSON commands
- Three-state rules system (current / staged / backup) to prevent administrator lockouts
- Two-step activation safety window: rules auto-rollback if not confirmed within configurable timeout
- Argon2id password hashing
- HTTPS-only web interface with auto-generated ECDSA P-256 self-signed certificates (auto-renewed 30 days before expiry)
- Per-IP login rate limiting (5 attempts per 10 minutes)
- Comprehensive security headers (HSTS, CSP, X-Frame-Options, Permissions-Policy)
- CSRF protection via Go 1.25 `net/http.CrossOriginProtection`
- nftables backend via netlink — only touches `table inet easywall`, Docker chains are not modified
- Protection modules: SSH brute-force, SYN flood, ICMP flood, port scan detection, invalid packet drop, bogon filter, connection limit, TCP RST flood, broadcast/multicast/anycast drop, and logging
- IPv6 support with configurable ICMPv6 type allowlist
- Docker bridge network auto-detection and whitelisting
- Structured audit log of all rule changes
- Rule import/export as JSON
- i18n support (English and German)
- Docker Compose and systemd deployment support

### Changed

- Configuration format changed from INI/YAML to TOML
- Rules storage changed from YAML files to a single JSON file with three-state versioning
- nftables replaces iptables as the kernel firewall backend

## [0.3.1] - 2021-02-17

[Full Changelog](https://github.com/jp1337/easywall/compare/v0.3.0...v0.3.1)

### Changed

- Remove `--show-progress` from shell scripts and fix issue #26

## [0.3.0] - 2020-09-30

[Full Changelog](https://github.com/jp1337/easywall/compare/v0.2.4...v0.3.0)

### Added

- Ports can now have a description. In future versions you will be able to edit this description. Currently you can only delete the port and add a new description.
- CodeQL analysis of GitHub enabled. This is a beta test of Github.
- Python tests prepared for Python 3.9
- It is now recognized when adding a port, if it is already present.
- A new pip3 module pyyaml is now required. This should be installed automatically during the update.

### Changed

- Ports page in the web interface visually redesigned for the new port description
- The update script no longer updates to the master branch, but to the last release
- The Feature-Policy HTTP Header is deprecated and was replaced by Permissions-Policy.
- Buffer overflow problem solved with very large HTTP header in request
- Problem solved, if values were written in capital letters in the configuration
- Tests rewritten for use with the new Rules Handler

### Removed

- Rules are no longer stored in the rules folder but in config/rules.yml. The folder structure under rules can therefore be deleted. There is no import of old rules, because easywall is still in beta status.

## [0.2.4] - 2020-09-06

[Full Changelog](https://github.com/jp1337/easywall/compare/v0.2.3...v0.2.4)

### Added

- Security headers of the demo page are checked for correctness and actuality.
- Information about what to do after the installation of easywall to adjust the access data.
- Class documentation automatically generated and added to the dosc folder
- If no user name and password is set in the configuration file, the First Run Wizard is automatically displayed in the web interface
- After saving the options in the web interface, the tab you saved will be displayed.
- Login attempts and the lockout time for too many failed logins can now be configured under "Web Interface".
- bindip and bindport option with the info that these are debug variables

### Changed

- The bindip and bindport options have been replaced by the UWSGI start parameters
- Error messages when saving the options are now displayed correctly
- Fixed several errors when starting the web interface in debug mode

## [0.2.3] - 2020-08-28

[Full Changelog](https://github.com/jp1337/easywall/compare/v0.2.2...v0.2.3)

### Changed

- Problems with the installation fixed
- Installation guide improved
- Problems at startup under Ubuntu 18.04 solved

## [0.2.2] - 2020-08-24

[Full Changelog](https://github.com/jp1337/easywall/compare/v0.2.1...v0.2.2)

### Added

- Readme and documentation improved
- Added quick start guide to documentation
- APT package and repository guide added to installation documentation
- New security and general HTTP headers added
- Installation shellscripts strongly improved

### Changed

- Inline Javascript moved to separate file

## [0.2.1] - 2020-08-22

[Full Changelog](https://github.com/jp1337/easywall/compare/v0.2.0...v0.2.1)

### Added

- easywall is now also available as installable Debian package
- easywall is now also available on pypi and can be installed over it
- Massive improvement of GitHub workflows
- Improve automated testing through GitHub workflows
- There is now an FAQ documentation, which will be filled with time
- The web server now sends headers to harden the application such as no permission for frames
- 403 Error page added and web errors generally improved
- The web configuration is now also checked for missing entries
- flask-ipban dependency added
- pypi package information improved and completed
- Unit Tests significantly improved and the tools for Core and Web Tests combined

### Changed

- After 10 incorrect login attempts on the web interface by default, the attacker address is blocked
- The log settings were moved to a separate configuration file "log.ini" in the "config" folder
- The SSL settings were hardened - only current browsers can be used
- The easywall_web folder was moved to the easywall folder as "easywall/web

## [0.2.0] - 2020-07-20

[Full Changelog](https://github.com/jp1337/easywall/compare/v0.1.0...v0.2.0)

### Added

- GitHub sponsorship was activated for the project
- A large number of configuration entries have been added
- Blocked connections can be logged by iptables
- Connections from blacklisted senders can be logged
- Broadcast, multicast and anycast packets can be blocked
- SSH brute force prevention was added. Attention! The feature is in alpha state and untested
- ICMP flood prevention has been implemented. The feature is also in alpha state
- Drop Invalid Packages was implemented. This is also an Alpa version
- Port Scan Prevention has been implemented. The feature is currently unstable in my tests
- IPv6 Router Advertisement connections can be allowed or prohibited
- IPv6 Neighbor Advertisement packets can also be allowed or prohibited
- Installation and update documentation has been improved
- easywall is now programmed completely typed thanks to mypy
- Ports can now be forwarded from the local system. Note that both the source and destination ports must be opened. This is because this is only a nat forwarding and not a FORWARDING forwarding
- The translations have been significantly improved thanks to deepl.com
- Username and password for the web interface can be changed directly in the web interface
- It is recognized if configuration entries are missing. This is especially important in this version, because we have added some variables. You will be notified about the differences in the web interface
- The start page of the web interface has been completely reworked. In the future I imagine a tag cloud from the open ports
- The options page in the web interface now contains almost all settings from the files

### Changed

- Python 3.5 is no longer supported, because no typing of variables is possible
- The detection from the first start has now been changed to a detection at every start. This has proven to be useful, as more rule types may be added in the future.
- The configuration files are reloaded each time a variable is called. This is needed to activate changes from the web interface immediately.
- An additional Python package "natsort" is required. The package offers the possibility to sort the ports naturally.
- The allowed ICMPv4/v6 types are now strongly restricted.

Allowed ICMPv4 types:

- 0 echo-reply
- 3 destination-unreachable
- 11 time-exceeded
- 12 parameter problem

Allowed ICMPv6 types:

- 1 destination-unreachable
- 2 packet-too-big
- 3 time-exceeded
- 4 parameter problem
- 128 echo request
- 129 echo-reply

After explicit configuration the following ICMPv6 types are allowed additionally:

- 133 router solicitation
- 134 router advertisement
- 135 neighbor solicitation
- 136 neighbor advertisement

## [0.1.0] - 2020-06-21

[Full Changelog](https://github.com/jp1337/easywall/compare/v0.0.4...v0.1.0)

### Added

- This version is almost completely tested by unit tests.
- The documentation was completely revised and can now be found in the `docs` folder.
- The configuration has been shortened and simplified.
- The installation, uninstallation and an update can now be carried out via scripts.
- The web interface installation now creates self-signed SSL certificates and can only be used over HTTPS.

### Changed

- create a setup.py and setup.cfg file for publishing
- create a requirements.txt file with all the requirements
- create github actions testing and linting
- implement custom rules feature
- create unit tests for all classes in easywall folder
- create unit tests for all classes in web folder
- rework all classes in easywall folder
- rework all classes in web folder
- set up a demo server
- write documentation for development setup
- SSL Implementation for web application
- write documentation for installing and uninstalling

## [0.0.4] - 2019-10-04

[Full Changelog](https://github.com/jp1337/easywall/compare/v0.0.3...v0.0.4)

### Added

- added possibility to apply custom IPTables rules
- full implemented webinterface - old PHP sources are history
- rule changes made in the webinterface are only written temporary into web directory
- rules can be applied in the webinterface
- a lot of code improvements
- this is kind the first "stable" version ready for testing
- I will test this on my webserver a lot, so the next versions will be more stable

### Changed

- too many, I can't count them
- there was a long time since the last version

## [0.0.3] - 2019-06-30

[Full Changelog](https://github.com/jp1337/easywall/compare/v0.0.2...v0.0.3)

### Added

- added easywall-Web using flask
- added old php templates to web
- improved install script a lot and added so many features to it
- simplified code using codacy and code climate
- ICMP Support added after testing on a server of mine
- added a daemon script for running easywall-Web
- 404 error page added to web
- for a production use of easywall-Web I added uwsgi instead of the small development server of flask
- logout button added to web
- added a password generator script and added it to install script

### Changed

- improved exception handling in several files
- the `.running` file was not deleted properly
- moved the system `os.system` to a single function where security checks can be implemented in the future

## [0.0.2] - 2019-06-08

[Full Changelog](https://github.com/jp1337/easywall/compare/v0.0.1...v0.0.2)

### Added

- Changed branch master to old python branch
- Renamed old master branch to php-old
- Bumped version
- Changed documentation

### Changed

- Information of the user in install.sh if not running as root or using sudo
- Removed quiet option in install.sh for apt-get and pip3 for better user experience

## [0.0.1] - 2019-04-24

### Added

- Incomplete Rework of Branch php-old
- easywall is split in two parts in the new concept
- easywall Firewall Core Part running as root user finished
- The New easywall will be one part running as root and one part running as easywall user which has access to config files.

[unreleased]: https://github.com/jp1337/easywall/compare/v2.2.0...HEAD
[2.2.0]: https://github.com/jp1337/easywall/compare/v2.1.0...v2.2.0
[2.1.0]: https://github.com/jp1337/easywall/compare/v2.0.0...v2.1.0
[2.0.0]: https://github.com/jp1337/easywall/compare/v0.3.1...v2.0.0
[0.3.1]: https://github.com/jp1337/easywall/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/jp1337/easywall/compare/v0.2.4...v0.3.0
[0.2.4]: https://github.com/jp1337/easywall/compare/v0.2.3...v0.2.4
[0.2.3]: https://github.com/jp1337/easywall/compare/v0.2.2...v0.2.3
[0.2.2]: https://github.com/jp1337/easywall/compare/v0.2.1...v0.2.2
[0.2.1]: https://github.com/jp1337/easywall/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/jp1337/easywall/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/jp1337/easywall/compare/v0.0.4...v0.1.0
[0.0.4]: https://github.com/jp1337/easywall/compare/v0.0.3...v0.0.4
[0.0.3]: https://github.com/jp1337/easywall/compare/v0.0.2...v0.0.3
[0.0.2]: https://github.com/jp1337/easywall/compare/v0.0.1...v0.0.2
[0.0.1]: https://github.com/jp1337/easywall/releases/tag/v0.0.1
