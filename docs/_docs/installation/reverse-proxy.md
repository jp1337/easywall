---
layout: default
title: Behind a Reverse Proxy
description: Put easywall behind nginx and have it still know which client is talking to it.
---

# Behind a Reverse Proxy

easywall terminates TLS itself, so a proxy in front of it is a choice rather
than a requirement. Make that choice and one thing breaks quietly: every request
now arrives from the proxy, so the audit log records the proxy's address, the
apply screen cannot tell whether *you* are about to be locked out, and the login
limiter's five attempts per ten minutes are shared by everybody behind it.

Listing the proxy in `trusted_proxies` fixes all three. Getting the list wrong
hands address spoofing to anything that can reach the port.

## 1 · Point nginx at easywall

```nginx
server {
    listen 443 ssl;
    server_name firewall.example.com;

    ssl_certificate     /etc/letsencrypt/live/firewall.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/firewall.example.com/privkey.pem;

    location / {
        # easywall serves HTTPS with its own certificate, generated on first
        # start. It is not in your chain and nginx has no reason to verify it.
        proxy_pass https://127.0.0.1:12227;
        proxy_ssl_verify off;

        proxy_set_header Host              $host;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

## 2 · Tell easywall which peer to believe

```toml
trusted_proxies = ["127.0.0.1"]
```

or `EASYWALL_WEB_TRUSTED_PROXIES=127.0.0.1`.

The value is the address **the proxy's connection arrives from**, as easywall
sees it — not the client's, and not the network the proxy is on.

## 3 · Check that it took

Sign in and open the [audit log]({{ '/docs/features/audit-log/' | relative_url }}).
The entry for that sign-in carries your own address, not `127.0.0.1`. If it says
`127.0.0.1`, the header is not being believed: the peer is not on the list, or
nginx is not sending `X-Forwarded-For`.

## In Docker

`trusted_proxies` names the **proxy container's own address**, and the default
bridge is not the one a compose project uses. Ask Docker rather than guessing:

```bash
docker network inspect <network> --format '{% raw %}{{range .Containers}}{{.Name}} {{.IPv4Address}}{{"\n"}}{{end}}{% endraw %}'
```

The default bridge is `172.17.0.0/16`; a compose project gets a network of its
own, typically `172.18.0.0/16` upward. Take the proxy's address out of that
output — a single address, with no mask.

> **A container's address is not stable across a recreate.** Give the proxy a
> fixed address on a user-defined network, or the list stops matching the next
> time the stack comes up and easywall quietly goes back to believing nobody.

## What it costs

Being on this list is total trust in that peer. Two mistakes hand address
spoofing to anyone who can reach the port:

- listing an address that is **not** actually a proxy in front of easywall;
- listing a **network** rather than the proxies themselves — every host in
  `10.0.0.0/8` can then choose the address easywall records, decides lockouts
  on, and rate limits.

List the proxies. Not the subnet they live in, not `0.0.0.0/0`, and never an
address you do not control. This is why the setting is a *list* and not a
boolean: "trust the header" with no way to say whose is
GHSA-3fxj-6jh8-hvhx, GHSA-rjr7-jggh-pgcp and GHSA-9g5q-2w5x-hmxf, and no
configuration of easywall can express it.

`X-Real-IP`, `True-Client-IP` and `Forwarded` are never believed, from any peer.

## When it does not work

| Symptom | Cause |
|---|---|
| The audit log records the proxy | The peer is not on `trusted_proxies`, or nginx sends no `X-Forwarded-For` |
| easywall refuses to start | An entry that is neither an address nor a CIDR network. The message names it |
| The lockout warning on Apply is wrong | Same cause: the verdict is computed for the address easywall resolved |
| It worked, then stopped | A container was recreated and took a new address. Fix the address, not the list |

**Next:** [Configuration]({{ '/docs/configuration/' | relative_url }}) ·
[Security]({{ '/docs/security/' | relative_url }})
