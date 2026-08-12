# Packaging

`debian/` is the single description of the installed system: what lands where, who
owns it, which units run it. The container image and `make install` follow it;
where they disagree, this is the one that has been installed and started in CI on
every pull request since 2.5.0.

## The trap that shipped a package with no binaries

`debian/rules` builds into `bin/` and installs from there in
`override_dh_auto_install`. It used to build straight into `debian/easywall/` and
say "binaries already placed by build step".

**`dh_prep` runs between `dh_auto_build` and `dh_auto_install`, and emptying
`debian/<package>` is its entire job.** Everything the build wrote there was
deleted before the package was assembled. The `.deb` held the units, the assets
and the config, and not one executable — for the whole life of the package.

Nothing noticed because CI built the artefact and uploaded it without ever
installing it. On a host that installed it, systemd answered `status=203/EXEC` —
cannot execute — every five seconds, for ever.

The lesson generalises: **an artefact that is only built is not tested.** Every
check in `build.yml`'s `build-deb` job after the build step exists because of this.

## The layout, and why each line of it

`debian/postinst` creates it; `build.yml` asserts every row of this table on a
freshly installed package.

| Path | Owner | Mode | Why |
|---|---|---|---|
| `/usr/sbin/easywall-core`, `…-web` | `root:root` | 0755 | checked by name, because the package shipped without them |
| `/etc/easywall` | `root:easywall` | 0750 | the web user must **traverse** it and must not write in it |
| `/etc/easywall/easywall.toml` | `root:root` | 0600 | the root daemon's config. A network-facing process able to rewrite it defeats the two-process split |
| `/etc/easywall/web.toml` | `easywall:easywall` | 0600 | the wizard and the password page rewrite it |
| `/etc/easywall/ssl` | `easywall:easywall` | 0750 | the web process generates and renews its own certificate |
| `/var/lib/easywall` | `root:easywall` | 0770 | **shared**: the core writes `rules.json`, the web user writes its caches |
| `/var/log/easywall` | `root:easywall` | 0750 | audit log, rotated by logrotate |
| `/run/easywall` | `root:easywall` | 0750 | created by systemd from the unit's `User=`/`Group=` |

Two of these were wrong at once and made a packaged installation useless:

- `/etc/easywall` was `0750 root:root`, so `easywall-web` could not traverse to
  its own config and exited at startup.
- `/var/lib/easywall` belonged to `easywall:easywall`, and the **core** could not
  write it.

That second one is the interesting half. See below.

## Capabilities: what root loses when you bound it

`easywall-core.service` runs as `User=root` with:

```ini
AmbientCapabilities=CAP_NET_ADMIN
CapabilityBoundingSet=CAP_NET_ADMIN
```

For a root service, that bounding set cuts the effective set down to exactly
`CAP_NET_ADMIN` — **no `CAP_CHOWN`, no `CAP_DAC_OVERRIDE`**, both of which root
normally leans on without anyone noticing. Two things broke:

1. The daemon could not `chown` the control socket to the `easywall` group, so it
   stayed `root:root 0660` and `easywall-web` was refused on `connect()`. Every
   page reported the core as unreachable.
2. It could not enter `/var/lib/easywall`, which belonged to the web user, so
   `rules.json` was never created.

The fix is not a wider capability set: the unit runs the daemon in the `easywall`
**group**. The owner of a file may give it to a group it belongs to, no capability
required, and the data directory is group-writable. `Group=easywall` in that unit
is load-bearing, and deleting it looks harmless.

`ProtectSystem=full` is what gives `ReadWritePaths=` anything to do. Without it
everything is writable and the list below it states a restriction that is not in
force.

## Architecture

`debian/control` says `Architecture: any` and `debian/rules` calls a plain
`go build` with no `GOARCH`, so the runner decides what comes out and `dpkg` names
the file from `DEB_HOST_ARCH`. Both `.deb`s are therefore built on a runner of
their own architecture — never cross-compiled — which is what lets CI install and
start each one. See [ci-and-release](ci-and-release.md).

`Build-Depends` names the Go version from `go.mod`'s `toolchain` line and is kept
in step by Renovate plus `TestGoToolchainIsTheSameEverywhere`. It said `>= 1.21`
since the 2.0.0 rewrite while the code already needed 1.25 for
`http.NewCrossOriginProtection`.

Debian trixie ships Go 1.24, so building this package there needs golang from
backports.

## Versions

`debian/rules` reads the package version from `debian/changelog` and passes it to
the linker:

```make
VERSION := $(shell dpkg-parsechangelog --show-field Version)
LDFLAGS := -s -w -X github.com/jp1337/easywall/internal/shared.CurrentVersion=$(VERSION)
```

`-X` writes to a **variable**. `CurrentVersion` was a `const`, so this — and the
identical flags in the `Makefile`, `.goreleaser.yaml`, the `Dockerfile` and two
workflows — succeeded and changed nothing. Every released binary reported the
literal in the source.

`debian/changelog` had also sat at 2.0.0 for five releases, so every package
claimed to be the version of the Go rewrite.

## The maintainer address

`debian/control` and two `debian/changelog` sign-offs carried a personal Gmail
address in a public repository. It is now a GitHub noreply address, and
`TestNoPersonalEmailAddressesAreTracked` walks every tracked file to keep it that
way.
