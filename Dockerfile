# ── Stage 1: Build ──────────────────────────────────────────────────────────
# Pinned to the patch, and to the same one as go.mod's toolchain line — a test
# compares the two. `golang:1.26-alpine` floated to whatever the newest 1.26 was,
# which is how this image came to be built by a Go version no workflow tested with.
FROM golang:1.27.1-alpine AS builder

WORKDIR /src

# Cache dependencies separately from source
COPY go.mod go.sum ./
RUN go mod download

# Build both binaries with version injection
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
      -ldflags "-s -w -X github.com/jp1337/easywall/internal/shared.CurrentVersion=${VERSION}" \
      -o /out/easywall-core ./cmd/easywall-core && \
    CGO_ENABLED=0 GOOS=linux go build \
      -ldflags "-s -w -X github.com/jp1337/easywall/internal/shared.CurrentVersion=${VERSION}" \
      -o /out/easywall-web ./cmd/easywall-web

# ── Stage 2: Runtime ─────────────────────────────────────────────────────────
FROM alpine:3.24

# Provenance, set here rather than in one of the workflows, because there are
# three build paths — GoReleaser for a release, publish-edge for :edge, and a
# plain `docker build` for anyone building their own — and only this file is on
# all three. installation/docker.md tells operators to read the revision back
# and compare it against the tag; before this, release images carried no labels
# at all and the instruction could not work.
ARG VERSION=dev
ARG REVISION=unknown
LABEL org.opencontainers.image.title="easywall" \
      org.opencontainers.image.description="Linux firewall management with a web interface" \
      org.opencontainers.image.source="https://github.com/jp1337/easywall" \
      org.opencontainers.image.url="https://easywall-project.org" \
      org.opencontainers.image.documentation="https://easywall-project.org" \
      org.opencontainers.image.licenses="GPL-3.0-or-later" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}"

# nftables for firewall management; supervisor to run both processes; tini for
# signal handling; tzdata so TZ (docker-compose.yml) resolves to an actual
# zone. Alpine carries none of the /usr/share/zoneinfo database by default —
# without this package, Go's time.LoadLocation fails for anything but "UTC"
# and "Local" silently means UTC regardless of what TZ says, which is a TZ
# variable that looks respected and is not.
RUN apk add --no-cache nftables supervisor tini tzdata && \
    addgroup -S easywall && \
    adduser  -S -G easywall easywall

COPY --from=builder /out/easywall-core /usr/sbin/easywall-core
COPY --from=builder /out/easywall-web  /usr/sbin/easywall-web

# Assets served by the web process (relative to WorkingDirectory)
COPY web/     /usr/share/easywall/web/
COPY locales/ /usr/share/easywall/locales/

# Default configs — can be overridden with a bind mount, which is why a pristine
# copy stays here as well: a bind mount of an empty directory brings no
# configuration with it, and the entrypoint installs these into it.
COPY config/*.toml /etc/easywall/
COPY config/*.toml /usr/share/easywall/config/

# Runtime directories
# Ownership mirrors the Debian layout, and for the same reason: /etc/easywall
# holds easywall.toml, which the root core reads. Handing the whole directory to
# the unprivileged web user — as `chown -R easywall:easywall /etc/easywall` did
# — let a network-facing process rewrite the configuration root loads.
RUN mkdir -p /run/easywall /var/lib/easywall /var/log/easywall /etc/easywall/ssl && \
    chown root:easywall /run/easywall     && chmod 750 /run/easywall && \
    chown root:easywall /etc/easywall     && chmod 750 /etc/easywall && \
    chown root:root     /etc/easywall/easywall.toml && chmod 600 /etc/easywall/easywall.toml && \
    chown easywall:easywall /etc/easywall/web.toml  && chmod 600 /etc/easywall/web.toml && \
    chown easywall:easywall /etc/easywall/ssl       && chmod 750 /etc/easywall/ssl && \
    chown root:easywall /var/lib/easywall && chmod 770 /var/lib/easywall && \
    chown root:easywall /var/log/easywall && chmod 750 /var/log/easywall

# Supervisor config
COPY docker/supervisord.conf /etc/supervisord.conf

# The ownership above is set at build time and a bind mount replaces all of it.
# The entrypoint restores it at start, which is what makes `docker compose up -d`
# with the shipped ./config mount work at all — read its header.
COPY docker/entrypoint.sh /usr/local/bin/easywall-entrypoint
RUN chmod 0755 /usr/local/bin/easywall-entrypoint

EXPOSE 12227

# One request, and it covers both halves of the recorded incident.
#
# docker/entrypoint.sh's header describes a container that stayed "Up" with a
# live core and a dead web process: easywall-web could not write its own config,
# exited 1, supervisord restarted it for ever, and the check of the day looked
# only at the core's socket. A check on the core cannot see that half, and until
# this line a plain `docker run` had no check at all — only docker-compose.yml
# did, so anybody not using compose got the container's status from nothing but
# whether PID 1 was alive.
#
# Do not "restore" a second command asking easywall-core. It was here, and it
# was wrong twice over. /healthz already sees everything it saw:
#
#   - the endpoint is *served by the web process*, so an answer at all proves
#     that process is alive. That is the incident above, and the only half a
#     core-side check could never see.
#   - the handler *asks the core over the socket*, so a dead core is
#     fail/core_unreachable → 503, and a kernel that is not carrying the rules
#     is fail/not_enforcing → 503.
#
# One wget therefore sees a dead web process, a dead core, and a firewall that
# has stopped filtering. The second command added no coverage — and it broke the
# one state that must not restart anything. handler_health.go answers 200 for
# `degraded` deliberately, because every cause of `degraded` is immune to a
# restart: a failed proof is the same binary, a build finding is the same binary,
# a dead stateful counter is the same rules. Restarting buys nothing and costs a
# window in which the machine is not filtering at all. `easywall-core health`
# exits 1 on `degraded`, so a composite `health && wget` marked the container
# unhealthy anyway and made the handler's argument unreachable.
#
# start-period is 15s and not docker-compose.yml's old 5s: the entrypoint does
# its ownership work over a bind-mounted /etc/easywall before supervisord starts
# anything, and a first-run container generates its certificate after that.
#
# Podman needs `--format docker`, and so does `podman compose build`. Its
# default image format is OCI, and the OCI spec has no healthcheck field, so a
# podman build prints one warning — "not supported for OCI image format and will
# be ignored" — and produces an image with no check in it, from a build that
# exits 0. Building this file with podman and no flag gives you back exactly the
# state this line exists to remove, which is why it is written here next to the
# check rather than only in the installation guide.
#
# `podman compose build` is the sharper edge of the two, because
# docker-compose.yml has a `build:` section and `podman compose up -d` is the
# documented path: an operator who follows the documentation on a podman host
# builds an OCI image locally and gets no health check and no error. That is
# named again in docker-compose.yml and is carried in carried-forward.md,
# because a comment is not a fix — the real answers are making the documented
# path pull a published image or accepting a compose-level check, and both are
# decisions past this release.
#
# The port is the other thing this line cannot adapt to. 12227 is written here
# while bind_addr in web.toml is the operator's to change, and a container whose
# interface moved reads `unhealthy` for ever — the check would be asking a port
# nothing listens on. That is the one case where the compose `healthcheck:` this
# repository otherwise refuses is the right answer, and docker-compose.yml says
# so where the block used to be. It is not read from the config here because
# this line is baked at build time and the port is not known until the container
# starts.
#
# The self-test needs CAP_SYS_ADMIN to build the namespace it proves rules in,
# and this image asks for NET_ADMIN and nothing else — so health reports the
# self-test as never recorded. That is not `degraded` and must not make the
# container unhealthy: being unable to prove something is not the same as it
# being broken. computeHealth's last branch is where that is decided.
HEALTHCHECK --interval=10s --timeout=5s --start-period=15s --retries=3 \
  CMD wget -q --no-check-certificate -O /dev/null https://127.0.0.1:12227/healthz

VOLUME ["/etc/easywall", "/var/lib/easywall", "/var/log/easywall"]

ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/easywall-entrypoint"]
CMD ["/usr/bin/supervisord", "-c", "/etc/supervisord.conf", "-n"]
