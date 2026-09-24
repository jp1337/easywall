---
layout: default
title: Health Check
description: Wire monitoring to a firewall that measures what it is doing instead of asserting it.
---

# Health Check

Everything easywall reported before 2.17 was a claim about *intent*: a table
exists, a daemon is up, a rule set was applied. For five releases the stateful
half of the input chain matched no packet while every one of those said the
firewall was active.

The health check reports *effect*. This page is how you wire it up.

## The three surfaces

| | Answers with | Reach for it |
|---|---|---|
| `easywall-core health` | three lines and an exit code | a cron or Nagios-style check on the host |
| `GET /healthz` | JSON on the web port | a monitoring system over the network |
| Docker and systemd | nothing to configure | container restarts and unit ordering |

### `easywall-core health`

```
$ easywall-core health
health:     ok
reason:     healthy
selftest:   unprovable for 2.17.0 on 6.12.48-1-lts at 2026-09-09T07:41:02Z
```

| Exit code | State |
|---|---|
| `0` | `ok` |
| `1` | `degraded`, or a reply this binary cannot read |
| `2` | `fail`, including a daemon that is not running |

`2` covers *no daemon* on purpose. Being unable to confirm that a firewall is up
is not the same as it being up.

**It disagrees with `easywall-core status` under panic mode, deliberately.**
`status` exits `0` — somebody chose to unfilter this machine, and a console
asking after intent is right to be quiet. `health` exits `2`, because a
monitoring system is asking a different question.

### `GET /healthz`

The one route on the web process that answers without a session, because an
orchestrator holds none.

`/healthz` reads the kernel, so it waits for an apply that is in progress. A
custom-rules apply can hold that for up to 30 seconds. The endpoint answers
503 during it, and the container's health check reaches its third failure
inside that window. Nothing restarts: an unhealthy container is a label, not
an action.

```bash
curl -sk https://127.0.0.1:12227/healthz
```

```json
{
  "state": "ok",
  "reason": "healthy",
  "selftest": {
    "version": "2.17.0",
    "result": "unprovable",
    "at": "2026-09-09T07:41:02Z"
  }
}
```

**The body is deliberately thin.** No rule detail, no counters, and no kernel
release — this route is unauthenticated, so it says the least that a monitoring
system can act on. The dashboard names the kernel; `/healthz` never does.

A stamp nothing has recorded — a container, where the proof does not run —
renders `"selftest": {}` rather than empty strings. Absence is the answer;
`result` is only ever one of `passed`, `failed` or `unprovable`.

| Code | When |
|---|---|
| `200` | `ok` **and** `degraded` |
| `503` | `fail`, and a core that does not answer |
| `404` | the caller is not on `health_allow` |

`degraded` answers `200` on purpose. A degraded firewall is still filtering, and
every cause of it survives a restart unchanged. Marking a container unhealthy
for one would restart a working firewall and drop every connection through it.
Alert on the `state` field instead.

`404` rather than `403`: not confirming that the endpoint exists costs nothing,
since whoever is allowed gets the real answer anyway.

### Docker and systemd, without configuration

| | |
|---|---|
| The image | `HEALTHCHECK` runs `easywall-web -healthcheck` every 10s, 15s start period, 3 retries — `/healthz` at the address `bind_addr` names |
| `docker-compose.yml` | carries the same check under `healthcheck:`, because podman's default image format has nowhere to keep the image's; a test keeps the two identical |
| `easywall-core.service` | `Type=notify` — `active (running)` means the socket exists |
| | `WatchdogSec=60`, so a wedged daemon is restarted rather than left listening |

**A fresh container reads `unhealthy` until you apply rules for the first time**,
because it genuinely is not filtering yet. That is honest rather than broken, and
it has one consequence: a `depends_on: condition: service_healthy` on easywall
will wait for ever until the first apply. The old check could not see this — it
looked at the core's socket.

## Letting a monitoring host in

`/healthz` starts closed to everything but loopback. It publishes the firewall's
state to whoever can read it. The gate reads the TCP peer, never a forwarding
header, so nothing behind a proxy can claim to be loopback.

> **Moving `bind_addr` needs nothing here.** The container's check asks
> `/healthz` at the bound address and from it, and the endpoint admits a peer
> that is its own local address. Widen the list for a monitoring host, not for
> the check. `health_allow = []` still closes the endpoint to everyone,
> the container's check included.
>
> **This is not a loopback exception.** Any process on this host that
> connects to the bound address is admitted the same way, a same-host reverse
> proxy relaying remote traffic included. Unless the list is `[]`, it is not
> the complete set of admitted peers.

```toml
# /etc/easywall/web.toml
health_allow = ["127.0.0.1/8", "::1/128", "10.20.0.0/24"]
```

`EASYWALL_WEB_HEALTH_ALLOW` sets the same list, comma-separated.

| You write | You get |
|---|---|
| the key, with entries | those addresses and networks, and nothing else |
| the key, empty list | the endpoint off entirely |
| no key at all | the loopback default above |

**The list is matched against the TCP peer and never against `X-Forwarded-For`.**
A proxy in front of easywall cannot pass a monitoring host through it — list the
proxy's own address. See [Security]({{ '/docs/security/' | relative_url }}).

## The three states, and the reason each carries

{% include themed-figure.html base="/assets/diagrams/health-states" ext="svg"
   alt="Decision flow: panic mode gives fail with reason panic; no live rules gives fail not_enforcing; ports counting while established counts nothing gives degraded stateful_dead; an expression-check finding gives degraded build_findings; a failed self-test gives degraded selftest_failed; everything else is ok healthy." %}

| State | Reason | What it means |
|---|---|---|
| `ok` | `healthy` | rules are live and the stateful half is matching packets |
| `degraded` | `stateful_dead` | ports counted packets, the established rule counted none |
| `degraded` | `build_findings` | a rule reached the kernel that the expression check could not believe |
| `degraded` | `selftest_failed` | the self-test disproved a claim about the rule builder |
| `fail` | `not_enforcing` | the kernel is not carrying easywall's rules |
| `fail` | `panic` | somebody ran `easywall-core panic`, and it survives a reboot |
| `fail` | `core_unreachable` | the web process asked and got no answer |

`stateful_dead` is the defect this release exists to catch. It needs both halves:
`established == 0` on a host with no traffic is honest, so the alarm also wants
the port counters to have moved.

`core_unreachable` comes from the web process and never from the core. A crashed
core says nothing about the kernel, which may still be filtering perfectly.

## `unprovable` is normal, and is not a fault

The self-test reports `unprovable` on most installations, and the health state
stays `ok`.

Proving anything needs a network namespace of easywall's own, which needs
`CAP_SYS_ADMIN`. The daemon holds `CAP_NET_ADMIN` and nothing else, so it cannot
build one — and neither can an ordinary container. Being unable to prove
something is not the same as it being broken.

Where you will meet it: **running `easywall-core selftest` by hand on a live
host.** Expected — one claim needs an inbound connection accepted host-side as
its control, and the table already in place refuses it. That is why the shipped
proof runs from a `oneshot` unit ordered *before* the daemon.

In a container you meet something else. The image asks for `CAP_NET_ADMIN` and
nothing else, so the proof cannot run there and never will. The selftest line
reads *unavailable here* rather than *never recorded* — the first names a
capability, the second names a chore somebody ought to clear. `/healthz` renders
`"selftest": {}`, the dashboard omits the fact, and the state stays `ok`.

*Never recorded* still appears where it is true: a host that **could** run the
proof and has not. There, `easywall-core selftest` clears it.

## Proving a port that answers nothing

If the question is only *has this rule ever matched*, the ports page's
**Last used** column already answers it from these same counters. What follows
is for the question it cannot: the `forward` chain's drop is not a rule of
yours and has no row there.

`unprovable` is easywall declining to measure its own claim. The same problem
arrives from the other side. A UDP service that never replies looks the same
whether the firewall dropped the packet or the service ignored it. No probe that
waits for an answer can separate the two.

The rules count packets. Ask them instead of asking the service.

```bash
sudo nft list table inet easywall > /tmp/before
#   from the outside host, send what needs no reply
#   dig @198.51.100.7 example.com   ·   nc -u host 514   ·   nmap -sU -p 53 host
sudo nft list table inet easywall > /tmp/after
diff /tmp/before /tmp/after
```

Do not apply rules in between. An apply rewrites the table, and that zeroes
every counter in it.

| The counter that moved | Reading |
|---|---|
| an `accept` on that port | the packet arrived and easywall let it through — what the service did with it is not easywall's business |
| the `forward` chain's `drop` | it arrived and easywall dropped it. A published container port with no **forwarded** [rule]({{ '/docs/features/ports/' | relative_url }}) is the usual cause |
| nothing moved | nothing arrived. Read the path in front of easywall — a provider firewall, a NAT, the wrong address |

This is how the port published on a bridge gateway in
[Docker coexistence]({{ '/docs/features/docker/' | relative_url }}) was found:
every container's DNS was gone while fifteen external probes stayed green.

## The self-test

Four claims, each measured with a real packet over a veth pair rather than read
off the rules.

**What it creates on your host, at every boot after an upgrade.** A veth pair
named `ewst-r` and `ewst-p`, addressed `10.77.9.1` and `10.77.9.2` in
`10.77.9.0/24`, with `ewst-p` moved into a throwaway network namespace. The
pair is removed when the proof finishes. Two things worth knowing before it
runs:

- **The range is fixed.** If your own LAN is `10.77.9.0/24`, the proof adds a
  route that collides with it for the seconds it runs.
- **The names are fixed, and setup clears them by name.** Any host interface
  literally called `ewst-r` is deleted before the pair is built — whoever made
  it — and deleting one end of a veth takes its peer with it.

The claims:

1. a reply on an established connection passes
2. an open port accepts a connection
3. a closed port does not
4. a blacklisted address does not reach an open port

A disproved claim records `failed`, which reads `degraded` — never a refusal to
filter. No layer of this ever declines to put the rules in the kernel.

**It runs once per version and kernel.** `easywall-selftest.service` runs
`selftest --if-stale` before the daemon starts, and a stamp that already matches
both is not re-proven. The defect class it catches is a build defect: identical
on every start of the same binary, so proving it once is enough.

```bash
systemctl start easywall-selftest    # honour the stamp
easywall-core selftest               # force it, and rewrite the stamp
```

The bare subcommand always runs. Reach for it after changing a rule builder —
the reason to run it by hand is to distrust what is recorded. It prints a
`detail` line naming the claim that failed, which `/healthz` deliberately never
carries.

`unprovable` exits `0` and a disproved claim exits `1`. The unit lists
`SuccessExitStatus=0 1 2`, so neither can fail a boot.

Both outcomes reach
[the audit log]({{ '/docs/features/audit-log/' | relative_url }}) as
`selftest_passed` or `selftest_failed`, and the
[dashboard]({{ '/docs/features/dashboard/' | relative_url }}) carries the state
with its reason in words.

---

**Next:** [Recovery & Panic Mode]({{ '/docs/features/recovery/' | relative_url }}) ·
[Dashboard]({{ '/docs/features/dashboard/' | relative_url }}) ·
[Configuration]({{ '/docs/configuration/' | relative_url }})
