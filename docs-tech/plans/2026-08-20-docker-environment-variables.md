# Environment variables for the container — Design

**Goal:** A person who reaches for `docker run` can configure *where easywall
runs* without editing a TOML file first, and can find the complete list of what
is settable in one place. What the firewall *does* stays where it is — in the
interface and in the files the interface writes.

**Architecture:** A thin overlay applied at config-load time, between parsing the
TOML and validating it. Twelve variables, all deployment-level. No new
configuration concepts: every variable names exactly one existing TOML key, and
the overlay is the only new code path.

**Tech Stack:** Go, `BurntSushi/toml` (already vendored), no new dependencies.

## Why this is not "document the current state"

easywall reads no environment variable today — `os.Getenv` does not appear in the
non-test Go source at all. The only variable that has any effect is `TZ`, and the
Go runtime consumes that through tzdata, not easywall. A container operator has
to bind-mount two TOML files before the process will start.

## Global constraints

- **A variable may never target a field the interface writes.** If it did, an
  operator would press Save, be told it was saved, and find the old value back
  after the next restart. The alternative — marking such fields read-only in the
  interface — was considered and rejected: it hollows out the Options page,
  which is the product.
- **No secrets.** Environment variables are visible in `docker inspect`, in
  `/proc/<pid>/environ`, and in any support log somebody pastes into an issue.
  `session_key`, `password`, `totp_secret` and `recovery_codes` stay in
  `web.toml`, which is `0600`.
- **An environment value is validated exactly like a file value.** No variable
  may reach a running process without passing the checks a TOML value passes.
- **An environment value never becomes file content.** Setting a variable must
  not, by any path, write that value into the operator's TOML file.

## The variables

### `easywall-core`

| Variable | TOML key |
|---|---|
| `EASYWALL_CORE_SOCKET_PATH` | `socket_path` |
| `EASYWALL_CORE_DATA_DIR` | `data_dir` |
| `EASYWALL_CORE_LOG_DIR` | `log_dir` |

### `easywall-web`

| Variable | TOML key | Type |
|---|---|---|
| `EASYWALL_WEB_BIND_ADDR` | `bind_addr` | string |
| `EASYWALL_WEB_SOCKET_PATH` | `socket_path` | string |
| `EASYWALL_WEB_SSL_DIR` | `ssl_dir` | string |
| `EASYWALL_WEB_DATA_DIR` | `data_dir` | string |
| `EASYWALL_WEB_TLS_CERT` | `tls.cert` | string |
| `EASYWALL_WEB_TLS_KEY` | `tls.key` | string |
| `EASYWALL_WEB_LANGUAGE` | `language` | string |
| `EASYWALL_WEB_UPDATE_CHECK` | `update_check` | bool |
| `EASYWALL_WEB_DEMO_MODE` | `demo_mode` | bool |

`TZ` is documented alongside these, explicitly marked as read by the Go runtime
rather than by easywall — an operator who sets it needs to know it works, and a
maintainer who greps for it needs to know why it is absent from the table.

### Deliberately absent, and why

| Excluded | Reason |
|---|---|
| `firewall.*`, `ipv6.*`, `docker.*`, `routing.*`, `acceptance.*` | written by the interface through `SaveOptions` / `SaveSettings` / `SaveSystem` |
| `telemetry` | written by the interface through `SaveTelemetry` — consent is the operator's to give and withdraw at the machine |
| `username`, `password`, `totp_secret`, `recovery_codes`, `session_key` | secrets, and also written by the interface |

The documentation page states this list with its reasons. "Why can I not set
this?" is otherwise the next question, and an unexplained absence reads as an
oversight.

## Where the overlay runs

Inside `LoadConfig`, after `toml.Unmarshal` and before the caller reaches
`Validate()`.

That position is the design, not an implementation detail. `Validate()` is what
rejects a `tls.cert` without a `tls.key`, requires `bind_addr`, and defaults
`data_dir`. An overlay applied after it would let every environment variable walk
past all of that; an overlay applied before it means `EASYWALL_WEB_TLS_CERT`
alone fails at startup with the same message the TOML file would have produced.

## The write-back trap

`Config.saveLocked` renders through `mergeConfig`, which only rewrites
`managedKeys`: `session_key`, `username`, `password`, `telemetry`, `totp_secret`,
`recovery_codes`. Those are precisely the keys excluded above, so the normal save
path cannot persist an environment value.

But `render` falls back to `encode()` when `mergeConfig` declines — an empty file,
or one that states a key twice — and `encode()` marshals the whole `WebConfig`.
On that path an `EASYWALL_WEB_BIND_ADDR` would be written into `web.toml` by the
next password change and become permanent, with nothing recording where it came
from.

**Fix:** `LoadConfig` retains the parsed struct as it stood *before* the overlay.
`encode()` renders that stored copy, taking only the six managed keys from the
live configuration. The file then continues to say what the file said, plus the
six keys the interface deliberately maintains.

## Types and failure

Ten strings, two booleans. A string variable is taken verbatim; an empty value is
treated as unset, so `-e EASYWALL_WEB_LANGUAGE=` does not blank the language.

A boolean accepts what `strconv.ParseBool` accepts. **An unparseable boolean is a
startup error naming the variable and the value** — not a silent fall-back to the
default. A firewall that quietly ignores `EASYWALL_WEB_UPDATE_CHECK=yes` and
phones GitHub anyway is exactly the surprise this feature exists to remove.

## Guard tests

| Test | Protects |
|---|---|
| `TestNoEnvVarTargetsAFieldTheInterfaceWrites` | the central rule, **derived** rather than restated: it intersects the environment table with the set of fields reachable from the `Save*` methods, and fails on any overlap |
| `TestEveryEnvVarIsDocumented` | the table and `docs/_docs/environment.md` agree in both directions — the same shape as the existing `TestEveryConfigKeyIsDocumented` |
| `TestEnvOverlayNeverReachesTheConfigFile` | drives the `encode()` fallback with a variable set, and fails if the value appears in the rendered file |

The first is the one that matters in a year. Someone will eventually want
`acceptance.duration` as a variable — it looks like deployment, and it is not.
A test that merely lists forbidden keys would be updated in the same commit that
adds the variable. One that derives the forbidden set from the write paths cannot
be satisfied that way.

## Documentation

- `docs/_docs/environment.md` — the full table, the `TZ` note, the exclusion list
  with reasons, and a `docker-compose.yml` that runs as written.

  A flat page, not `configuration/environment.md`: `configuration` is a single
  page today, and `features/` and `installation/` are folders with no sibling
  `.md`. Giving `configuration` a folder alongside its file would invent a third
  structural pattern for one page.
- A nav entry in `docs/_config.yml`, titled **Environment Variables**, directly
  after Configuration. Nothing tests the nav, so a page added without it is
  reachable only by URL — which for a page whose entire purpose is being findable
  would defeat the feature.
- A pointer from the existing Docker installation page, because that is where
  somebody looking for this actually is.
- Both locales. Any string the page introduces into the interface — there should
  be none — would need `locales/*.json` at parity.
- `docs-tech/invariants.md` gains the three tests.

## Out of scope

- `_FILE` indirection for secrets (the Docker-secrets convention). It is the
  natural next step if fully automated first installs are ever wanted; it is a
  separate decision about whether credentials should have a non-interactive path
  at all.
- Any change to what the interface may write.
- Reloading a variable without a restart.
