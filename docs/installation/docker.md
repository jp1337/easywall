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

## Where to pull from

The same `linux/amd64` + `linux/arm64` image, pushed to all three by CI. Public, no
authentication.

| Registry | Image |
|---|---|
| GitHub Container Registry | `ghcr.io/jp1337/easywall` |
| Docker Hub | `docker.io/kermit1337/easywall` |
| Quay.io | `quay.io/jp1337/easywall` |

> **Quay is behind as of v2.4.0.** The publishing token is being rejected, so that
> mirror is skipped until it is replaced. Use GHCR or Docker Hub.

## Which tag

| Tag | Moves | For |
|---|---|---|
| `:latest` | on tagged releases only | **production** |
| `:vX.Y.Z` | never | pinning, e.g. `v2.4.0` |
| `:edge` | after every green build on `main` | tracking development, [demo hosts]({{ '/installation/demo/' | relative_url }}) |
| `:sha-<commit>` | never | rollback and debugging |

```yaml
# docker-compose.override.yml — pin a version
services:
  easywall:
    image: ghcr.io/jp1337/easywall:v2.4.0
```

## Why host networking

nftables acts on the host's network namespace. In its own namespace the container
would filter only its own traffic, which is not what you asked for. `NET_ADMIN` is
what lets it issue the netlink calls that reach host tables.

```yaml
network_mode: host
cap_add:
  - NET_ADMIN
```

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
`:latest` for production; the public demo runs it against `:edge`.

## Checking what you pulled

Every image carries the source commit in an OCI label:

```bash
docker buildx imagetools inspect \
  --format '{% raw %}{{ index .Manifest.Annotations "org.opencontainers.image.revision" }}{% endraw %}' \
  ghcr.io/jp1337/easywall:latest
```

Compare it against the commit the release tag points at. The workflows that publish
these images are
[`release.yml`](https://github.com/jp1337/easywall/blob/main/.github/workflows/release.yml)
and
[`publish-edge.yml`](https://github.com/jp1337/easywall/blob/main/.github/workflows/publish-edge.yml).
