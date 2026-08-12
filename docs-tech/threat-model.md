# Threat model, in detail

The operator-facing version is [`docs/security.md`](../docs/security.md). This page
holds the reasoning and the internals behind it — the parts that matter when
changing the code, not when running it.

The design follows from one decision: **the process reachable from the network
holds no privilege worth stealing.** A flaw in form parsing or template rendering
cannot reach the kernel, because the process holding that flaw has no kernel
access to misuse.

## Sessions

| | |
|---|---|
| Cookie | `easywall`, signed with `session_key`, `HttpOnly` · `Secure` · `SameSite=Lax` |
| Lifetime | `SessionLifetime` = 600 s, in `internal/web/auth.go` |
| Constructed by | `newSessionStore` — the **only** place a store is built |
| Password change | every other session is refused at once |
| Logout | that session's identifier is recorded as revoked |

`newSessionStore` exists because getting this right is not obvious.
`sessions.NewCookieStore` sets the codec's maximum age from its own default —
thirty days — and **assigning a fresh `Options` struct afterwards does not change
it.** Every call site did exactly that, so the browser dropped the cookie after ten
minutes while the server kept accepting it for thirty days.

The consequence was worse than the arithmetic. A logged-out session is remembered
as revoked for one lifetime and then forgotten, on the stated ground that the
cookie has expired by then. It had not: replaying the same cookie eleven minutes
after signing out signed you straight back in to a firewall's administration
interface, with nothing to see. Measured before the fix: `/dashboard` answered 200
with a cookie 29 days old, and 200 again after the logout record had been swept.

`store.MaxAge()` sets both halves. Write that, never `Options.MaxAge`.

A session also carries `SessionCredentialKey`: a non-reversible fingerprint of the
password hash it was issued under. There is nothing to revoke server-side in a
signed cookie, so this is how "changing the password ends every other session"
works — a session whose fingerprint no longer matches is refused.

The revocation record is in memory. A restart inside the same ten minutes forgets
it, which is stated on the operator page rather than hidden.

## Passwords

Argon2id, 64 MiB, 3 iterations, parallelism 4, a 16-byte salt per password.
Minimum length 12, in one constant (`minPasswordLen`) because the interface also
states the rule in words.

There is no default password and no recovery path — no mail, no outside service.
Editing `web.toml` on the host is the way back. A hash easywall cannot parse is
refused with a log line rather than failing the login page silently, and the parser
never puts anything it read into that line: ten constant `errHash*` values exist so
that no attacker-influenced text reaches the journal, and `maxHashPart` bounds what
is decoded.

## Why `X-Forwarded-For` is ignored

`buildRouter` deliberately does **not** use `middleware.RealIP`:

```go
// easywall-web terminates TLS itself and isn't assumed to sit behind a trusted
// reverse proxy, so X-Forwarded-For/X-Real-IP/True-Client-IP are
// attacker-controlled. Trusting them would let a client spoof its IP and bypass
// the per-IP login rate limiter.
// See GHSA-3fxj-6jh8-hvhx, GHSA-rjr7-jggh-pgcp, GHSA-9g5q-2w5x-hmxf.
```

`r.RemoteAddr` — the actual TCP peer — stays authoritative. This is the right
default for a service that expects to be reached directly.

It has a cost that the operator documentation now states: **behind a reverse
proxy, every request appears to come from the proxy**, so the login limiter's five
attempts per ten minutes are shared by everyone. One attacker exhausts the budget
and nobody can sign in until it refills.

Making that configurable is on the roadmap and must be opt-in and explicit — a
list of trusted proxy addresses, not a boolean. A boolean that says "trust the
header" is the vulnerability those three advisories describe.

## Rate limiting

`LoginRateLimit`: a token bucket per source address, 5 tokens refilling one every
2 minutes. Buckets unused for 15 minutes are swept by a goroutine started once per
process.

Only the login path is limited. The rest of the interface is behind
authentication, and the protection modules in the firewall itself are what limit
unauthenticated traffic — those are counted per source address in the kernel, in a
set whose entries expire when a source goes quiet. Four of them held a single
counter for the whole machine until 2.5.0, which meant the module meant to prevent
a lockout produced one from a single attacker at negligible cost.

## The one place a string reaches a command

Everything typed reaches the kernel as a Go struct over netlink. Custom rules are
the exception: the netlink library takes typed expressions, not text, so a raw
nftables statement is applied by writing `add rule inet easywall input <rule>` into
`nft -f -`.

`nft` reads a newline and a semicolon as the end of one command and the start of
the next, so a rule carrying either is a second command run by the root daemon —
demonstrated against a real kernel, where an imported rule containing a newline
wrote into a neighbouring table.

Both characters are refused **on the shape of the input**, before anything is
stored, on save, on import, and again inside the core. Deliberately not by parsing:
the check must not depend on nft's grammar, on a subprocess being available, or on
the syntax-check wrapper happening to be balanced. There is also a ceiling of 256
statements per check.

## Content Security Policy

```
default-src 'self'; script-src 'self' 'nonce-<per-request>'; style-src 'self';
font-src 'self'; img-src 'self' data:; connect-src 'self'
```

No external origin at all: an administrative interface should not report a visit
to anyone, and easywall often runs on hosts with no outbound route, where a
third-party font request simply fails and leaves the typography broken.

`style-src` has no `'unsafe-inline'`, which constrains contributions: assigning
`element.style.*` from JavaScript, or letting a library inject a `<style>` block,
is blocked. Scripts toggle a class.

htmx was configured through a listener for an `htmx:config` event, which htmx does
not emit. The listener never ran, so `allowEval` stayed `true` and the script nonce
was never applied. It is configured through the `meta[name=htmx-config]` tag now —
found by tightening `style-src`, which surfaced the inline `<style>` block htmx had
been injecting unnoticed.

## What is not defended

- A compromised root account. Root owns the core.
- A kernel nftables vulnerability. That is below easywall.
- A legitimate administrator writing a bad rule. The audit log records it; nothing
  prevents it.
- Anyone holding `session_key`. It signs the cookies, so it is a login. easywall
  generates one on first start if the key is missing, too short, or still the
  placeholder — that placeholder is published in this repository.
