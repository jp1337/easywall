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
| `/lib/systemd/system/easywall-{core,web,selftest}.service` | `root:root` | 0644 | checked by installed path. A unit file nothing installs does not exist on the machine |

## Neither config is shipped under its own name

Both TOMLs arrive as `easywall.toml.template` and `web.toml.template`, and
`postinst` copies each into place only when the real file is absent. That is
what keeps them out of dpkg's conffile list — and it has to, because **easywall
rewrites both while it runs**: the settings pages write `easywall.toml` through
the core, the wizard and the password page write `web.toml`.

`web.toml` always did this, for its secrets. `easywall.toml` did not, and dpkg
therefore tracked a file a script edits. Measured in a `debian:trixie` container,
upgrading 2.5.1 → 2.5.2 with one setting saved beforehand and a plain
`apt-get install -y`:

```
Configuration file '/etc/easywall/easywall.toml'
 ==> Modified (by you or by a script) since installation.
 end of file on stdin at conffile prompt
dpkg-query: install ok unpacked 2.5.2
```

`unpacked`, not `installed`: dpkg stopped at the prompt, so **postinst never ran**
— the new binaries were on disk and the services were never restarted. Any
upgrade path that does not pass `--force-confold` takes it, and a package may not
assume the administrator's does. The prompt only appears when the shipped default
also changed, which is exactly what a release that fixes a key looks like;
`config/easywall.toml` carried the obsolete `ipv6.enabled` for a release.

Removing a conffile is its own hazard, so the direction that matters was measured
too: upgrading *from* a package that still carried it leaves
`/etc/easywall/easywall.toml` untouched at `root:root 0600` with the operator's
settings intact, prints nothing, and ends `install ok installed`. A fresh install
produces the layout in the table above.

`TestNeitherConfigIsShippedAsAConffile` reads `debian/rules` and `debian/postinst`
and fails if either file goes back to being installed directly.

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

## The web unit's one capability

`easywall-web.service` runs `User=easywall`, not root, and its own port
(12227) is above 1024 and needs nothing extra. When `tls.acme` is on, though,
it also binds port 80 for the ACME HTTP-01 challenge listener — see
`internal/web/acme.go` — and that needs exactly one capability:

```ini
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
```

Granted unconditionally, the same reasoning as `easywall-core.service`'s
bounding set above: a unit that has to be hand-edited to turn a setting on
fails with "permission denied" for whoever forgets, with nothing in that
message pointing at a systemd file. `CapabilityBoundingSet` repeats the same
single value so the grant is also a ceiling — nothing this process does can
add a second capability to itself later.

It composes with `NoNewPrivileges=yes` a few lines above it in the same unit:
ambient capabilities are granted by systemd at exec, not acquired by the
process through a setuid binary or a file capability, which is exactly what
`NoNewPrivileges` forbids.

`debian/rules` installs `systemd/easywall-web.service` verbatim — `grep -rn
'easywall-web.service' debian/` finds no template, only the `install` line —
so the two capability lines above are the only place this needs to land.

## The third unit, and the only `CAP_SYS_ADMIN` in the repository

`easywall-selftest.service` is new in 2.17. It exists for one reason: the
self-test measures the table easywall builds against a real packet, which needs a
network namespace of its own, and `CLONE_NEWNET` requires `CAP_SYS_ADMIN`.

`easywall-core.service` bounds the daemon to `CAP_NET_ADMIN` and says nothing
else is needed. That is true of nftables, and widening it would hand the extra
capability to the long-lived root process that holds the control socket and
answers a network-facing one — to buy a check that runs once per upgrade. So the
proof got a unit instead.

| | |
|---|---|
| `Type=oneshot` | alive for milliseconds, not for the uptime of the machine |
| `Before=easywall-core.service` | ordering only. `Requires=` would let a failed proof stop the firewall |
| `SuccessExitStatus=0 1 2` | a disproved claim exits 1 and cannot fail a boot |
| `Group=easywall` | it writes `/var/lib/easywall/selftest.json`, and a bounded root has no `CAP_DAC_OVERRIDE` — the same fault that left `rules.json` uncreated |
| no socket, no listener | nothing a stranger can reach, and it exits before the daemon starts |

**Without this unit the proof would never run where it matters.** Under
`easywall-core.service`'s bounding set the daemon cannot `unshare`, so it would
record `unprovable` on every Debian install — the primary target.

`postinst` enables it with the other two and deliberately does not start it.
Starting it there would hold an often-unattended upgrade for the length of a
proof, and `selftest --if-stale` would then skip it at the next boot.
`TestCapSysAdminIsGrantedByExactlyOneUnit` is what says no to the one-line diff
that widens the daemon instead.

**The install-verify job did not catch a unit removed from `debian/`** until this
release widened it. It now asserts all three by installed path, and that
`easywall-selftest.service` is *enabled* — `is-active` reports the wrong thing
about a `oneshot`, and an installed-but-disabled unit is a proof that never runs.

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
