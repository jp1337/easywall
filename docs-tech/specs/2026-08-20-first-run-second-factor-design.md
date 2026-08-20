# A second factor from the first minute

**Date:** 2026-08-20 · **Status:** design approved, not yet planned or executed

Not published. This directory sits outside `docs/`, which is the entire Jekyll
source — see `TestTheTechnicalDocsAreNotPublished`.

## The sentence

The first-run wizard can switch the second factor on, and refuses to make that a
way of not getting an account.

## What is in the change

| | |
|---|---|
| **The feature** | An optional second factor inside the wizard, enrolled before the account is written |
| **A correction** | `installation/first-run.md` currently states the wizard deliberately does not ask. That becomes untrue and is rewritten |
| **A roadmap change** | Passkeys as a **second factor** get their own entry, with the hostname dependency written out. See *Roadmap change* |

## Decisions

| Question | Decision | Why, in one line |
|---|---|---|
| Where it sits | **Inside the wizard, before anything is written** | `SaveFirstRun` writes once on purpose, and that property is worth keeping |
| What holds the half-finished setup | **An in-memory table, addressed by a random handle in a short cookie** | There is no session yet, and `gorilla/sessions` signs without encrypting |
| The password across two requests | **Hashed in step 1, only the hash is carried** | A plaintext password has no business in a hidden field or a cookie |
| A code that will not verify | **"Continue without a second factor" on the same page** | A dead RTC must not be able to prevent an account existing at all |
| Confirming before storing | **Unchanged from 2.8** | Storing an unconfirmed secret is a lockout generator |
| `SaveFirstRun`'s signature | **A struct, not five positional parameters** | Two adjacent strings and two optional trailing arguments is a swap waiting to happen |

## Why the escape hatch is the important part

The wizard is the one screen that runs on a machine which is, by definition,
freshly exposed and not yet protected. Everything else in this design is
ordinary; the branch that matters is the one where the operator's code never
verifies.

TOTP fails for exactly one common reason on this hardware: the server's clock.
easywall runs on single-board computers with no RTC, which come up at the epoch
until NTP lands. If the only way past step 2 were a correct code, a flat battery
would mean **no account at all** on a machine already reachable from the network.

So step 2 carries "Continue without a second factor", and it takes precisely
today's one-write path. The server time is printed on that page for the same
reason it is printed on the enrolment card: it is the only place somebody without
shell access learns that the clock, not the code, is wrong.

## Architecture

### New state

```
internal/web/firstrunpending.go   the table, the handle cookie, the sweep
```

A table on the pattern of `pendingSecrets` (`handler_2fa.go`), with one
difference: enrolment keys by session id, and here there is no session. The key
is a random handle carried in `easywall_setup` — own name, `Path=/firstrun`,
`HttpOnly`, `Secure`, built with `store.MaxAge`, never `Options.MaxAge`.

```go
type pendingFirstRun struct {
    Username     string
    PasswordHash string   // argon2id, computed in step 1
    Telemetry    bool
    Secret       string   // base32, unconfirmed
    Issued       time.Time
}

// Same value as pendingSecretLifetime, and a separate constant on purpose:
// they answer different questions and may diverge.
const firstRunPendingLifetime = 10 * time.Minute
```

Nothing here reaches disk. A restart mid-wizard means "start again", and that
costs nothing at all, because the account does not exist yet either — the first
run is still the first run.

### What one write now carries

`SaveFirstRun` takes a struct rather than growing to five positional parameters,
two of which would be optional and two of which would be adjacent strings:

```go
// FirstRunAccount is everything the wizard decides, in one value, so the write
// that persists it cannot be called with the username and the hash swapped.
// Lives in internal/web beside Config — it is not protocol and the core never
// sees it.
type FirstRunAccount struct {
    Username     string
    PasswordHash string
    Telemetry    bool

    // Empty and nil when no factor was set up. They are written together with
    // the account or not at all; there is no path that stores one without the
    // other.
    TOTPSecret     string
    RecoveryHashes []string
}

func (c *Config) SaveFirstRun(a FirstRunAccount) error
```

The existing guard inside it — refusing when a password is already set, under the
same lock as the write — is unchanged, and so is the reason for it.

### Changed

| File | Change |
|---|---|
| `internal/web/config.go` | `SaveFirstRun` takes a `FirstRunAccount` struct; still one write, still under the same lock, still refusing when a password already exists |
| `internal/web/handler_firstrun.go` | The checkbox branch; two new handlers |
| `internal/web/server.go` | `POST /firstrun/confirm`, `POST /firstrun/skip` — **inside the same `if cfg.IsFirstRun()` block** as the existing pair |
| `web/templates/firstrun.html` | Two further states, rendered as the POST's own response |
| `locales/{en,de}.json` | Both, in the same change |
| `docs/_docs/installation/first-run.md` | Rewritten where it claims the wizard does not ask |
| `docs/_docs/features/two-factor.md` | A line saying it can be switched on during setup |
| `docs/_docs/roadmap.md` | The passkeys entry |

**The new routes live and die with the wizard.** They are registered only while
`cfg.IsFirstRun()` holds, exactly as `/firstrun` already is, so they stop
existing the moment an account does. That is also why they do **not** join
`credentialWritingRoutes` in the demo guard: the demo ships with a password set,
so those routes are never registered there — the same structural argument the
2.8 review already made for `/firstrun` itself. Say so in a comment beside the
list, or the next reader will think the coverage lapsed.

## Data flow

```
POST /firstrun                     [unchanged when the box is not ticked]
  |
  +- box unticked --> SaveFirstRun(account)          one write, then /login
  |
  +- box ticked ----> hash the password
                      generate a secret
                      put both in the table under a fresh handle
                      set easywall_setup, render step 2 as this POST's response
                      NOTHING IS WRITTEN

POST /firstrun/confirm
  | handle missing or expired -> /firstrun, start again
  |
  +- code hits at +/-1 ---> SaveFirstRun(account incl. secret + 8 hashes)
  |                         ONE write. Then the eight codes, once.   => totp_enabled
  |
  +- code hits at +/-10 --> "the code is right, but this server's clock is
  |                          about N minutes behind" — nothing stored, step 2 again
  |
  +- no hit --------------> "Wrong code." Nothing stored, step 2 again.

POST /firstrun/skip
  | handle missing or expired -> /firstrun
  +- SaveFirstRun(account without a factor)          today's path exactly
```

Step 2 and step 3 are rendered as the response to their POST, never as a `GET`
with a URL of their own: a reload must not mint a second secret, and the eight
codes must not have an address.

## Error cases

| Case | Behaviour | Why |
|---|---|---|
| `web.toml` unwritable at confirm | Nothing created, message shown, **the table entry survives** | Otherwise the operator re-types a password and re-pairs a phone because a disk was briefly full |
| The clock is wrong | The ±10 diagnosis with sign and magnitude, plus the escape hatch on the same page | The fault is on the server; the message must not point at the human |
| The wizard is abandoned at step 2 | Nothing exists; the wizard is still open | The account was never written — this is the property that makes the design safe |
| Two wizards submitted at once | The second is refused by `SaveFirstRun`'s own check | Unchanged: that check lives under the write lock and already exists |
| The handle cookie is forged | It verifies or it does not; a bad one starts again | Signed with the session key, like every other cookie here |

## Proof

| Area | Assertion |
|---|---|
| The escape hatch | `POST /firstrun/skip` creates the account with **no** factor, and the result is byte-identical to today's one-write path |
| One write | A confirmed setup calls `SaveFirstRun` exactly once, carrying account **and** factor — asserted by counting writes, not by reading the code |
| Nothing early | After step 1 with the box ticked, `IsFirstRun()` is still true and `web.toml` is unchanged |
| The clock branch | ±2..±10 stores nothing and names the magnitude and direction |
| Rollback | A failed write leaves the table entry intact, and the retry succeeds |
| Handle | An absent, expired or forged handle lands on `/firstrun` and creates nothing |
| Routes | Once an account exists, `POST /firstrun/confirm` and `/firstrun/skip` are not registered at all |
| Locales | Both files carry every new key; the German is not the English |

**The escape-hatch test is the one that must not be allowed to rot.** It is the
difference between "an optional second factor" and "a wizard a flat battery can
brick". Its failure message should say that.

## Interface

`firstrun.html` gains one checkbox in the existing form — *"Set up a second
factor now"*, unticked, with one line under it saying an authenticator app is
needed and that it can be done later under Password.

Step 2 reuses the enrolment card's shape: the QR on its white plate (dark on
white in both themes — an inverted code is rejected by many scanners), the key in
groups of four, the server time, one code field, and below it the escape hatch as
a plain secondary button, not a small link. Step 3 is the eight codes in the same
mono grid with the same copy button and the same "these will not be shown again".

Everything reviewed at 1600/900/390 in both themes and both languages, and the
wizard screenshots retaken — `firstrun-{light,dark}` change, and the two new
steps get `firstrun-2fa-{light,dark}` and `firstrun-codes-{light,dark}`.

## Documentation

| | |
|---|---|
| `installation/first-run.md` | Rewritten: the wizard offers it, what you need in hand, and that skipping is a first-class answer rather than a failure |
| `features/two-factor.md` | One line that it can be switched on during setup, linking back |
| `docs-tech/invariants.md` | The escape-hatch guard, with the reason: a dead RTC must not be able to prevent an account existing |

## Roadmap change

Passkeys already sit in 3.0, bundled into "Reachable from outside" beside the API
and ACME work, with one clause explaining that WebAuthn rejects a bare IP address
as an RP ID. That undersells them and hides the dependency.

They get their own entry, as a **second factor** — an alternative to TOTP, never a
replacement for the password — and it stays after the hostname work, because the
constraint is real: WebAuthn needs a registrable domain, and most installations
are reached at `https://192.168.1.10:12227`. An entry that did not say so would
promise something that cannot work where easywall usually runs.

## Deliberately not in this change

| | |
|---|---|
| Recovery codes without TOTP | The codes exist to survive a lost phone. Alone they are just a second password |
| Enforcing a factor | The wizard offers; it does not insist. An operator who cannot use one must still get an account |
| Carrying the setup across a restart | Persisting an unconfirmed secret is exactly what the in-memory table exists to avoid |

## Open for the implementation plan

- `firstRunPendingLifetime` (10 minutes) is a starting value, asserted by a test.
- Whether the checkbox should also appear when JavaScript is off — it should; the
  whole flow is plain forms, and this is a note to verify rather than a decision.
