---
layout: default
title: Feeds
description: Block known attackers with lists other people publish — checked after your allowlist, each one off until you switch it on.
---

# Block known attackers with feeds

A feed is a list of attacking addresses that somebody else publishes and
easywall fetches. Feeds are checked **after** your allowlist, so an address you
allow stays reachable whatever a feed says.

{% include themed-figure.html base="/assets/diagrams/feed-order" ext="svg"
   alt="Four boxes in a row: your blocklist drops, then the allowlist accepts, then the feeds drop, then the port rules." %}

## Start with Spamhaus DROP and DShield

1. Open **Blocklist** and scroll to **Feeds**.
2. Switch on *Spamhaus DROP* and *DShield top block list*.
3. Press **Save**. The switch is staged like any rule.
4. Open **Apply Rules** and press **Apply now**. If the verdict names a feed
   that holds your own address, add that address to the
   [allowlist]({{ '/docs/features/allowlist/' | relative_url }}) first.
5. Press **Confirm rules** within the window, or the switch rolls back.

Later refreshes load a new copy **without** an acceptance window. Allowlist the
address you manage this host from, so no copy can lock you out.

Nothing is switched on for you. To see whom a feed refuses, switch on
*Log feeds* on the [options page]({{ '/docs/features/filters/' | relative_url }});
its hits appear on [Blocked]({{ '/docs/features/blocked-traffic/' | relative_url }})
with the feed's name.

## Choosing more

Every row in the Feeds card carries one of three marks. They follow firebog's.

| Mark | Means | Switching it on |
|---|---|---|
| ✓ | least likely to lock out a legitimate client | as above |
| • | useful, with a stated cost — the row names it | as above |
| ✗ | a deliberate choice, not a default | tick the box under the switch, which names whom it locks out |

A false positive here is a legitimate client that can no longer connect: a
visitor, a webhook sender, a mail server, your own monitoring. Outbound traffic
is never affected.

## An own feed

Up to three lists of your own, by address. Any plain-text list works: one
address or network per line, `#` and `;` start a comment.

1. On **Blocklist**, fill in one slot under **Own feeds**: a name, the address,
   and a user and password if the list needs them.
2. Press **Save**.
3. Switch the feed on in **Feeds**, save, apply and confirm.

**Remove** empties a slot only once its feed is switched off and applied.

| The address may be | Refused |
|---|---|
| `https://` to any host | a user or password inside the URL — use the two fields |
| `http://` to a literal loopback address, such as `127.0.0.1` | `http://localhost`, and `http://` to any other host |

An own feed is fetched once every 24 hours. It is not part of an
[export]({{ '/docs/features/export-import/' | relative_url }}); where its
password is stored is on the [security page]({{ '/docs/security/' | relative_url }}).

### Example: CrowdSec's Raw IP List

CrowdSec serves the blocklists you subscribe to in its Console from one URL,
with no CrowdSec engine installed.

1. In the CrowdSec Console, create a **Raw IP List** integration and subscribe
   it to the blocklists you want.
2. Copy the endpoint and the Basic credentials. The credentials are shown
   **once**.
3. Enter them as an own feed.

| | |
|---|---|
| Address | `https://admin.api.crowdsec.net/v1/integrations/<id>/content` |
| User, password | the integration's Basic credentials |
| Free plan | one pull per 24 hours — easywall's interval for an own feed — and at most three lists |
| Terms | the [CrowdSec EULA](https://booking.crowdsec.net/crowdsec-eula) applies to the data |

CrowdSec's default *Pull limit* is 10 000 IPs per pull, and easywall fetches
one page, so three subscribed lists can arrive partial. The limit is
CrowdSec's, set on its side: see its [integration documentation](https://docs.crowdsec.net/u/integrations/intro).

Already running a CrowdSec engine? Its `cs-blocklist-mirror` serves your own
decisions locally: use `http://127.0.0.1:41412/security/blocklist`.

## When a feed fails

The previous copy stays active through every failure. A refresh never opens an
acceptance window; switching a feed on or off does.

| Last refresh | Means |
|---|---|
| Never fetched | no copy yet; switched on, it blocks nothing |
| Updated | the last refresh brought a new version |
| Unchanged | the server had nothing new |
| Failed — the previous copy stays active | the *Error* line says why — *a web page, not a list* when the server sent HTML; the next try follows on its own |
| Failed — no copy yet | as above, and the feed blocks nothing until a copy arrives |

| Warning on the row | What to do |
|---|---|
| *In the kernel with an empty set* | nothing — it blocks once the first copy arrives |
| *Unchanged for 30 days* | check the list's source page; a list that stops changing may be abandoned |
| *Contains allowlist entries (N)* | nothing — those addresses stay reachable, which is the point of the order |
| *Failed N times in a row* | read the *Error* line above it |
| *Refused: shrank from N to M* | the core refuses a list under 70 % of the copy it holds. To take the smaller list, switch the feed off and apply, then on and apply |
| *N lines were not an address* | the first five are shown — often a notice from the server among the addresses, such as a rate-limit sentence |

The core also refuses, whole, a list of more than 100 000 entries, or one with a
network broader than /8 (IPv4) or /16 (IPv6). Private and reserved ranges are
dropped from every feed without a warning, and *Entries* counts what is left.

*Another change is being written*, with no window open, means a feed refresh
held the apply slot for a moment. That is under a second, even at the largest list the core accepts.
Press **Apply now** again; `easywall-core resume` waits for it on its own.

## Reference: the catalogue

### Measured 2026-09-24

The figures are a snapshot. A release that changes a feed's verdict measures
it again.

| Feed | What it blocks | Entries | Poll |
|---|---|---|---|
| [Spamhaus DROP](https://www.spamhaus.org/blocklists/do-not-route-or-peer/) | Netblocks that were hijacked or handed to criminals | 1710 + 91 (IPv4 + IPv6) | 12 h |
| [DShield top block list](https://isc.sans.edu/api/) | The 20 /24 networks with the most scanning sources over the last three days | 20 × /24 | 1 h |
| [blocklist.de (all)](https://www.blocklist.de/en/export.html) | Addresses fail2ban reported for attacks in the last 48 hours | 32 002 | 1 h |
| [CINS Army](https://cinsscore.com/) | Addresses the CINS sensors saw attacking; always the latest 15 000 | 15 000 | 1 h |
| [Emerging Threats compromised](https://rules.emergingthreats.net/OPEN_download_instructions.html) | Hosts known to be compromised | 686 | 1 h |
| [IPsum, on ≥ 3 lists](https://github.com/stamparm/ipsum) | Addresses on at least three of more than 30 public lists | 19 549 | 24 h |
| [Hagezi threat intelligence IPs](https://github.com/hagezi/dns-blocklists) | Addresses from threat-intelligence sources, collected for DNS blocking | 79 175 | 24 h |
| [Tor exit nodes](https://metrics.torproject.org/collector.html) | Every Tor exit relay | 1370 | 1 h |

What each costs, and where its terms are:

| Feed | False positives | Verdict | Terms |
|---|---|---|---|
| Spamhaus DROP | practically none | ✓ | [terms](https://www.spamhaus.org/drop/terms/) |
| DShield top block list | practically none | ✓ | [terms](https://isc.sans.edu/api/) |
| blocklist.de (all) | some — dynamic and recycled addresses, some cloud hosts and Tor exits | ✓ | [terms](https://www.blocklist.de/en/export.html) |
| CINS Army | some — about one in eleven is a cloud address | • | [terms](https://cinsscore.com/) |
| Emerging Threats compromised | notable — a quarter are AWS and Google Cloud addresses; not for hosts that receive webhooks or monitoring from a cloud | • | [terms](https://rules.emergingthreats.net/open/suricata-7.0.3/LICENSE) |
| IPsum, on ≥ 3 lists | yes — an aggregate, some sources aggressive, with Tor exits | ✗ | [terms](https://github.com/stamparm/ipsum) |
| Hagezi threat intelligence IPs | yes — cloud hosts and Tor exits | ✗ | [terms](https://github.com/hagezi/dns-blocklists) |
| Tor exit nodes | by design — every Tor user, legitimate or not | ✗ | [terms](https://metrics.torproject.org/collector.html) |
