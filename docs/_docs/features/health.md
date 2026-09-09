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

```bash
curl -sk https://127.0.0.1:12227/healthz
```

```json
{
  "state": "ok",
  "reason": "healthy",
  "selftest": {
    "version": "2.17.0",
    "kernel": "6.12.48-1-lts",
    "result": "unprovable",
    "at": "2026-09-09T07:41:02Z"
  }
}
```

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
| The image | `HEALTHCHECK` fetches `/healthz` every 10s, 15s start period, 3 retries |
| `docker-compose.yml` | declares no block of its own and inherits the image's |
| `easywall-core.service` | `Type=notify` — `active (running)` means the socket exists |
| | `WatchdogSec=60`, so a wedged daemon is restarted rather than left listening |

**A fresh container reads `unhealthy` until you apply rules for the first time**,
because it genuinely is not filtering yet. That is honest rather than broken, and
it has one consequence: a `depends_on: condition: service_healthy` on easywall
will wait for ever until the first apply. The old check could not see this — it
looked at the core's socket.

## Letting a monitoring host in

`/healthz` starts closed to everything but loopback. It publishes the firewall's
state to whoever can read it. An orchestrator's own probe comes from the
container, so loopback is where it is needed and the network is not.

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

Two places you will meet it:

- **In a container.** Expected. Nothing to do.
- **Running `easywall-core selftest` by hand on a live host.** Also expected: one
  claim needs an inbound connection accepted host-side as its control, and the
  table already in place refuses it. That is why the shipped proof runs from a
  `oneshot` unit ordered *before* the daemon.

## The self-test

Four claims, each measured with a real packet over a veth pair rather than read
off the rules:

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
