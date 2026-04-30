# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- DaisyUI 5.5 component library + HTMX 2.0 are now part of the web UI build. All 15 templates use DaisyUI primitives (cards, buttons, alerts, fieldsets, toggles, tables, badges, tabs, steps); the custom CSS in `web/src/app.css` now contains only layout-specific chrome
- New "Aurora Operator" color palette — analogous cool-tone scheme with deep slate-blue chrome and cyan-400/teal-400 accents in dark mode, white + cyan-600/teal-600 in light mode. Status colors: emerald (success), amber (warning), rose (error), sky (info). Replaces the previous orange/navy complementary pair which created visual tension
- Custom rules now validate **live** as you type — the textarea sends the content to `POST /custom/validate` (HTMX, 600ms debounce) and per-line syntax errors appear inline without a form submit. Falls back to a soft notice when the core daemon is unreachable
- `/options`, `/settings`, and `/system` now **auto-save on change** — toggle a switch or change a numeric input and the form is silently submitted via HTMX, with a small toast notification appearing in the bottom-right corner ("Saved" / "Save failed"). The traditional Save button is still present for graceful degradation when JavaScript is disabled
- Custom rules syntax validation also runs on save — the web UI validates raw nftables rules via `nft --check`; per-line errors are displayed inline in the editor (was already in 2.2 release flow, now used by the live-validation endpoint too)
- Tailwind CSS v4 UI — the web interface now uses a purpose-built "Operator Interface" design with Outfit UI font and JetBrains Mono for IPs/rules; replaces the previous IBM Plex stylesheet
- `make css` target and CI steps compile the Tailwind source in `web/src/app.css` to `web/static/style.css` during build

### Fixed

- Custom rules in `state.Current.Custom` are now actually applied to the nftables kernel after the typed rules flush; previously the slice was stored and validated but never passed to `nft`

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
