---
layout: default
title: Docker
description: One multi-arch image, three registries, four tags — and why it needs host networking.
---

# Docker

```bash
git clone https://github.com/jp1337/easywall.git
cd easywall
docker compose up -d
```

Open `https://localhost:12227` and complete the setup wizard.

> **`./config` changes owner on first start, and needs to.** The compose file
> mounts it at `/etc/easywall`, which replaces the ownership the image sets — so
> the files arrive belonging to whoever cloned the repository, in a directory the
> container's `easywall` user cannot write. easywall-web has to write `web.toml`
> (it generates the session key into it, and the wizard and the password page
> rewrite it) and to create its certificate in `config/ssl/`. It could do neither:
> the process exited before binding, supervisor restarted it for ever, and the
> container reported healthy because the healthcheck only looked at the core's
> socket. The entrypoint now puts that directory into the same shape the Debian
> package installs — `web.toml` and `ssl/` to `easywall`, `easywall.toml` to
> `root`, both `0600`. Editing them on the host afterwards needs `sudo`.

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
| `:edge` | after every green build on `main` | tracking development, [demo mode]({{ '/installation/demo/' | relative_url }}) |
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
`SYS_MODULE` — the capability to load kernel modules, which from a container that
already shares the host's network is host root under another name, and which this
page never listed. It is gone; applying a full rule set was verified without it. If
`nf_tables` is not loaded, load it on the host with `modprobe nf_tables` — a host
already running nftables has it.

This is also why easywall in a container still coexists with Docker's own rules —
it owns [`table inet easywall`]({{ '/features/docker/' | relative_url }}) and nothing else.

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
