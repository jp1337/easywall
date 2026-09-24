---
layout: default
title: Docker
description: One multi-arch image, three registries, four tags — and why it needs host networking.
---

# Docker

easywall filters the **host's** firewall, so it runs on the machine you want
protected — which is usually not the machine you are browsing from. Everything
below assumes that: commands on the server, browser on your own machine.

## First run

On the server, over SSH:

```bash
git clone https://github.com/jp1337/easywall.git
cd easywall
docker compose up -d
```

Then, from your own machine, open `https://<server>:12227` — the address you
already reach that host on. **`localhost` only works when easywall is on the
machine in front of you**, and it is the one instruction this page used to give.

Four things to expect on that first page:

| | |
|---|---|
| A setup token field | `docker compose logs easywall \| grep 'setup token'` — see [First Run]({{ '/docs/installation/first-run/' | relative_url }}#the-setup-token) |
| A certificate warning | easywall generates its own on first start. Accept it, or [supply your own](#your-own-certificate) |
| Nothing filtered yet | a fresh container carries no rules, so nothing easywall did is between you and port 12227 |
| The container reads `unhealthy` | correct, and it clears at your first apply — see [Health check](#the-health-check) |

If the page does not load at all, easywall is not what is blocking it. Check the
host's existing firewall and any provider-level security group for port 12227.

Now complete the [setup]({{ '/docs/installation/first-run/' | relative_url }}) — it
covers the SSH port, which is the one answer that can shut you out. Before you do,
see [Environment Variables]({{ '/docs/environment/' | relative_url }}) for what
`docker-compose.yml` can set without editing `./easywall-config` at all — and what it
deliberately cannot.

> **`./easywall-config` is yours, and changes owner on first start.** Docker
> creates it empty. The entrypoint fills it from the defaults inside the image,
> in the shape the Debian package installs. `web.toml` and `ssl/` go to the
> container's `easywall` user (uid 100, gid 101); `easywall.toml` goes to root.
> It is gitignored: the account lives there. **Editing those files on the host
> needs `sudo`.** In a bind-mounted `/var/lib/easywall` the entrypoint does the
> same: the directory is root's, and the web process keeps its passkeys and
> TOTP replay store in `web/` inside it.

> **Upgrading a git checkout from 2.21 or earlier?** compose mounted `./config`,
> the tracked defaults. Move your files once, before pulling:
>
> ```bash
> docker compose down
> sudo mv config easywall-config && sudo rm -f easywall-config/embed.go
> git checkout -- config && git pull
> docker compose up -d
> ```
>
> Your existing `easywall.toml` is moved as-is: whatever it already says for
> `docker.enabled` is kept, not replaced by the new default.
> Skip it and the container starts a fresh first run — which the setup token
> keeps anyone else from claiming.

## What must persist

| Path in the container | Holds | Without it |
|---|---|---|
| `/etc/easywall` | `easywall.toml`, `web.toml`, the certificate | the setup starts over |
| `/var/lib/easywall` | `rules.json`, the apply state, passkeys | every rule is gone |
| `/var/log/easywall` | `audit.log`, `packets.log` (with `persist = true`) | the audit trail and the refused-packet history start empty at every update |

```bash
docker run -d --name easywall --network host --cap-add NET_ADMIN \
  -v easywall_config:/etc/easywall \
  -v easywall_data:/var/lib/easywall \
  -v easywall_logs:/var/log/easywall \
  ghcr.io/jp1337/easywall:latest
```

Without the mount, Docker's `VOLUME` directive gives every recreated container a
new, empty anonymous volume, and the history in it is gone after the next update.

## The health check

The image carries a `HEALTHCHECK` that fetches `/healthz` every ten seconds, and
`docker-compose.yml` carries the same check under `healthcheck:`. Since 2.19
there are two definitions rather than one, because podman's default image
format has no place to put the first — see [Podman](#podman) below. A test
keeps them identical.

```bash
docker compose ps                     # healthy / unhealthy
docker exec easywall easywall-core health
```

It reports **unhealthy while the firewall is not filtering** — a fresh container,
a deleted table, a dead web process, or a core that stopped answering. That is
new: for a year the check looked at the core's socket, so a container whose
interface had died stayed *Up*.

Two consequences worth knowing before you meet them:

- **`depends_on: condition: service_healthy` on easywall waits for ever** on a
  container that has never applied rules. Apply once, or drop the condition.
- **A `degraded` firewall stays healthy.** It is still filtering, and a restart
  fixes no cause of it. [Health Check]({{ '/docs/features/health/' | relative_url }})
  has the states, the exit codes, and how to let a monitoring host read
  `/healthz`.

## Podman

**`podman compose up -d` needs nothing extra.** `docker-compose.yml` declares its
own `healthcheck:` block since 2.19, so the container gets one whether or not the
image does.

**A plain `podman build` + `podman run` still needs `--format docker`:**

```bash
podman build --format docker -t easywall .
podman run -d --network host --cap-add NET_ADMIN easywall
```

**Without it, the image carries no health check and the build still succeeds.**
OCI is podman's default image format and has no healthcheck field, so
`podman build` exits `0` and leaves `HealthCheck: null`. A plain `podman run`
has no compose file to fall back on here. Check what you got:

```bash
podman image inspect --format '{% raw %}{{ .HealthCheck }}{% endraw %}' easywall
```

`supervisorctl` works inside the container, over a root-only socket, if you need
to stop or start one process without restarting both:

```bash
docker exec easywall supervisorctl status
```

## Where to pull from

The same `linux/amd64` + `linux/arm64` image, pushed to all three by CI. Public, no
authentication.

| Registry | Image |
|---|---|
| GitHub Container Registry | `ghcr.io/jp1337/easywall` |
| Docker Hub | `docker.io/kermit1337/easywall` |
| Quay.io | `quay.io/jp1337/easywall` |

> **Quay is behind.** The publishing token is being rejected, so that mirror is
> skipped until it is replaced, and it does not have the recent releases. Use
> GHCR or Docker Hub.

## Which tag

| Tag | Moves | For |
|---|---|---|
| `:latest` | on **stable** releases only — a release candidate does not move it | **production** |
| `:vX.Y.Z` | never | pinning, e.g. `v{{ site.version }}` |
| `:edge` | after every green build on `main` | tracking development, [demo mode]({{ '/docs/installation/demo/' | relative_url }}) |
| `:sha-<commit>` | never | rollback and debugging |

```yaml
# docker-compose.override.yml — pin a version
services:
  easywall:
    image: ghcr.io/jp1337/easywall:v{{ site.version }}
```

## Why host networking

nftables acts on the host's network namespace. In its own namespace the container
would filter only its own traffic, which is not what you asked for. `NET_ADMIN` is
what lets it issue the netlink calls that reach host tables.

```yaml
network_mode: host
cap_add:
  - NET_ADMIN
security_opt:
  - no-new-privileges:true
```

`NET_ADMIN` and nothing beyond it. The shipped compose file also asked for
`SYS_MODULE` — the capability to load kernel modules. From a container that
already shares the host's network, that capability is host root under another
name. This page never listed it. It is gone; applying a full rule set was verified without it. If
`nf_tables` is not loaded, load it on the host with `modprobe nf_tables` — a host
already running nftables has it.

This is also why easywall in a container still coexists with Docker's own rules —
it owns [`table inet easywall`]({{ '/docs/features/docker/' | relative_url }}) and nothing else.

## Needs

- Docker Engine 24+, Compose v2
- A Linux host with nftables

## Your own certificate

```yaml
volumes:
  - /etc/letsencrypt:/etc/letsencrypt:ro
  - ./easywall-config:/etc/easywall
```

```toml
# ./easywall-config/web.toml
[tls]
cert = "/etc/letsencrypt/live/example.com/fullchain.pem"
key  = "/etc/letsencrypt/live/example.com/privkey.pem"
```

**`tls.acme` is not supported in this image.** The web process drops to an
unprivileged user with no path to reacquire `CAP_NET_BIND_SERVICE`.
`no-new-privileges:true` is deliberate hardening this project will not trade
away for one feature. Run your own ACME client (certbot, or Let's Encrypt's
own container) against the host and mount its output the way shown above
instead.

## Updating

```bash
docker compose pull && docker compose up -d
```

[Watchtower](https://containrrr.dev/watchtower/) automates it. Nightly or weekly on
`:latest` for production, or against `:edge` if you want every green build.

## Checking what you pulled

Every image carries the source commit in an OCI label:

```bash
docker pull ghcr.io/jp1337/easywall:latest
docker image inspect \
  --format '{% raw %}{{ index .Config.Labels "org.opencontainers.image.revision" }}{% endraw %}' \
  ghcr.io/jp1337/easywall:latest
```

> **This used to read `.Manifest.Annotations`, where the value has never been.**
> A label goes into the image *config*; manifest annotations are a different
> field that only carries base-image and creation keys. Checked by building an
> image with `--label org.opencontainers.image.revision=abc123` and reading the
> pushed OCI layout: `manifest annotations` held `image.base.name` and
> `image.created`, and `abc123` was in `config.Labels`. So the command printed
> nothing and looked like an image with no provenance. Release images genuinely
> had none either — the labels are set in the `Dockerfile` now, which is the one
> file all three build paths share.

Compare it against the commit the release tag points at. The workflows that publish
these images are
[`release.yml`](https://github.com/jp1337/easywall/blob/main/.github/workflows/release.yml)
and
[`publish-edge.yml`](https://github.com/jp1337/easywall/blob/main/.github/workflows/publish-edge.yml).
