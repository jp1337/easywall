# Field checks

easywall runs on a real server — root01xvp, managed from the `wdk-ansible`
repository. Every gate in CI runs against a namespace with a handful of rules; the
server runs 150 kernel rules, five feeds, a WireGuard tunnel and Docker. The two
together find what neither finds alone.

## What the field found that no gate did

| Release | Found on the server | What it became |
|---|---|---|
| 2.23.0 | every apply with four feeds failed: `recvmsg: no buffer space available` | 2.23.1 — the kernel acks every message and echoes every rule; the receive buffer was never raised. Older than 2.23 (≈107 rules on 2.22); every test had used one port rule |
| 2.23.0 | the failed apply said *nothing was written to the kernel* — the kernel had committed | 2.23.1 — a receive-side overflow is no longer tagged so |
| 2.23.1 | 60 dropped fragments were the host's own WireGuard peer | /options and `filters.md` name tunnels beside DNSSEC |
| 2.23.1 | IPv6 mail clients hung on a silent drop (AAAA record, IPv4-only ports) | roadmap 2.26: closed ports can answer (`reject`) |
| 2.23.2 | feeds stopped 3,472 of 3,903 SSH drops (89 %) before the brute-force meter | feeds pre-empt sshbrute — documented behaviour, nothing to change |
| 2.23.2 | 334 IPv6 default drops on mail ports (993/25/143/587/465/4190) in 11 h | the IPv4-only container gap — 2.24 |
| 2.23.2 | feeds drop some legitimate fetchers on 80/443 (blocklist.de, CINS) | `feeds.md` note and allowlist recipe (2.24); a per-feed port scope is roadmap 2.36 |
| 2.23.2 | IPv4 echo-request always meets policy drop | deliberate and documented (`filters.md`), no switch — roadmap 2.37 candidate: a switch to answer it |
| 2.24 | the published-port warning never read Docker's `iptables-nft` DNAT — an xtables target, not the nft `nat` expression the check read — since 2.20.1 | fixed in 2.24 (Task 3) |

## When

| Moment | Why |
|---|---|
| after every rollout, before the release is called finished | the release's own defects surface in the first hour |
| weekly between releases | feeds change, traffic changes, the operator changes switches |
| whenever the operator reports something odd | the report is the start, the readout is the evidence |

## What to read — read-only, on the host

| Read | Answers |
|---|---|
| `audit.log` since the last check: every `apply_failed`, `*_refused`, `rollback_*`, `boot_*`, and which `options_saved` preceded them | did anything fail, and what changed just before |
| `GET_PACKET_LOG` **as counts only**: by rule, by feed, by destination port, by protocol and family, unique sources | what the firewall refuses, and whether a refusal is the operator's own traffic |
| `GET_FEEDS`: status, entries, changed/checked, packets, warnings | whether a feed is stale, shrinking, failing or overlapping the allowlist |
| `/healthz` and `easywall-core health` | whether it enforces what it says |
| `nft list table inet easywall` counted (rules, sets, feed sets) against `applied-config.json` | whether the kernel holds what easywall believes it applied |
| journal warnings of both services | what the interface does not show |

## Rules

- **Addresses stay on the host.** The packet log holds third-party IP addresses. A
  readout that leaves the host carries counts, ports and rule names — never a
  source address, never the raw ring, never into an issue or a commit.
- **Read, do not tune.** A host sysctl, a switch or a staged rule is not changed to
  make a finding go away; the finding becomes code or documentation.
- **A finding is proved before it is filed.** The 2.23.1 report read "nothing was
  written" and was wrong in the other direction: the kernel source settled it. Ask
  for the one measurement that separates the explanations.
- **Caused by the release: fixed in a patch. Found by it: roadmap or
  `carried-forward.md`**, with the evidence — the rule `CLAUDE.md` sets for reviews.
- **A test follows every field finding**, shaped like the server: 2.23.1's regression
  test uses 40 port rules with sources and the host's four feeds at their real sizes.
