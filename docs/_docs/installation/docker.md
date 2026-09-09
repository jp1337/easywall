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

Three things to expect on that first page:

| | |
|---|---|
| A certificate warning | easywall generates its own on first start. Accept it, or [supply your own](#your-own-certificate) |
| Nothing filtered yet | a fresh container carries no rules, so nothing easywall did is between you and port 12227 |
| The container reads `unhealthy` | correct, and it clears at your first apply — see [Health check](#the-health-check) |

If the page does not load at all, easywall is not what is blocking it. Check the
host's existing firewall and any provider-level security group for port 12227.

Now complete the [setup]({{ '/docs/installation/first-run/' | relative_url }}) — it
covers the SSH port, which is the one answer that can shut you out. Before you do,
see [Environment Variables]({{ '/docs/environment/' | relative_url }}) for what
`docker-compose.yml` can set without editing `./config` at all — and what it
deliberately cannot.

> **`./config` changes owner on first start, and needs to.** The mount replaces the
> ownership the image sets, so the files arrive belonging to whoever cloned the
> repository — and `easywall-web` must write `web.toml` and create its certificate in
> `config/ssl/`. It could do neither, and the container reported healthy anyway
> because the healthcheck only looked at the core's socket. The entrypoint now puts
> the directory into the shape the Debian package installs. **Editing those files on
> the host afterwards needs `sudo`.**

## The health check

The image carries a `HEALTHCHECK` that fetches `/healthz` every ten seconds.
`docker-compose.yml` declares no block of its own and inherits it, so there is
one definition and nothing to keep in step.

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

Two flags and nothing else, but the first one is not optional:

```bash
podman build --format docker -t easywall .
```

**Without `--format docker` the image carries no health check and the build still
succeeds.** OCI is podman's default image format and has no healthcheck field, so
`podman build` exits `0` and leaves `HealthCheck: null`. `podman compose build`
takes the same flag. Check what you got:

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
  - ./config:/etc/easywall
```

```toml
# config/web.toml
[tls]
cert = "/etc/letsencrypt/live/example.com/fullchain.pem"
key  = "/etc/letsencrypt/live/example.com/privkey.pem"
```

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
