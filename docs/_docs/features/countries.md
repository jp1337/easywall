---
layout: default
title: Countries
description: Block the addresses registered to a country — each one switched on by itself, checked after your allowlist and after every feed.
---

# Block a country

A country is a list of the networks registered to it. Like a
[feed]({{ '/docs/features/feeds/' | relative_url }}), it is checked **after**
your allowlist, so an address you allow stays reachable. An address that is in a
feed as well is counted and logged by the feed. A country also drops
the forwarded packets the `forward` chain filters, to a container or a routed
network.

**Which ones do your drops come from?** The
[country column]({{ '/docs/features/blocked-traffic/' | relative_url }}#where-it-comes-from)
on Blocked traffic shows it. A row's **Block country** stages its country here.

## Add a country

1. Open **Blocklist** and scroll to **Countries**.
2. Choose the country and press **Add**. It is staged like any rule.
3. Wait until the country's row shows **Entries**. Apply earlier and easywall
   refuses and says so. If the copy never arrives, switch the country off.
4. Open **Apply Rules** and press **Apply now**.
5. Press **Confirm rules** within the window, or the country rolls back.

**Remove** takes it out again; apply and confirm as above. The row's
*Last refresh*, *Entries* and warnings read like a feed's: see
[When a feed fails]({{ '/docs/features/feeds/' | relative_url }}#when-a-feed-fails).

## What it cannot do

- **A registration is not a location.** The list says where a network was
  registered, not where a packet comes from. VPNs, cloud regions and Tor exits
  do not follow it.
- **It only blocks.** *Allow only these countries* is not this feature. Its
  inverse would block the address you manage the host from as soon as you
  travel.
- **A refresh has no acceptance window.** If a regional registry transfers an
  address block into a country you switched on, the next daily refresh blocks
  it without asking. Put the addresses you manage the host from on the
  [allowlist]({{ '/docs/features/allowlist/' | relative_url }}) first; it is
  checked before every feed and country.
- **Blocking the United States blocks Googlebot and Bingbot.** A crawler that
  cannot reach your site drops it from search. Allowlist the ranges the vendors
  publish
  ([Googlebot](https://developers.google.com/static/crawling/ipranges/common-crawlers.json),
  [Bingbot](https://www.bing.com/toolbox/bingbot.json)), or leave it off.

Where the lists come from, how often they change and the limits: the
[reference]({{ '/docs/features/feeds/#countries' | relative_url }}).
