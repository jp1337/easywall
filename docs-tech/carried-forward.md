# Carried forward from 2.7

What 2.7 found and deliberately did not fix, with enough context to act on. Every
entry was reviewed, triaged and ruled on rather than forgotten; the reasoning for
deferring is part of the entry.

Not published — this directory sits outside `docs/`, which is the entire Jekyll
source. See `TestTheTechnicalDocsAreNotPublished`.

## Operator-visible

| | |
|---|---|
| **`boot_enforce_failed` reads "at startup"** | 2.7 also writes it when a mid-apply panic teardown fails. The colour is right, the label is not, and `actionLabel` is what the audit filter searches — so somebody hunting a 15:00 teardown failure must search for "startup". Fixing it means rewording both locales and the *Reads as* column in `features/audit-log.md` |
| **`rollback_skipped`'s label** | Same shape, same fix: it now also covers a rollback that *was* written and then torn down. The detail says so; the label says "Rollback skipped" |
| **A forgotten panic mode is invisible to monitoring** | `easywall-core status` exits 0 under panic, deliberately — it is a state somebody chose. So a panic nobody remembers never pages anyone. Documented in the man page. A product decision, not a defect |
| **Every page pays a `GET_STATUS`** | **Shipped in 2.14** as `Server.statusForRender`, a ~2 s TTL cache read only by `render`. Not a passenger: the topbar countdown put a clock on every page, and a stall that used to cost the panic banner would have cost the countdown as well. Handlers that act on the status still ask the core directly |

## Invisible failures

| | |
|---|---|
| **Four audit-silent paths in `apply`** | The second `GetState` (the re-read after promote), `BackupCurrent`, `PromoteStaged` and `acceptance.Start` all return without an audit entry. 2.7 fixed the first `GetState` and left its four neighbours; do them in one pass, because four invisible paths beside two visible ones is the actual shape |
| **`acceptance.Start`'s error path** | Returns with the new rules live, no window, no rollback and no entry. Unreachable today and the comment says so; it should roll back rather than return |

## Guards that do not see enough

| | |
|---|---|
| **The kernel-write guard is scoped to two files** | `daemon_source_order_test.go` enumerates `firewall.go` and `restore.go`, so a fourth writer of the table added in a third file of the package is invisible — which is the guard's stated purpose. `filepath.Glob` over the package closes it. Nothing exploits it today: `f.nft.Apply(` occurs exactly three times repo-wide |
| **…and cannot see reachability** | The same guard passes when the panic check is kept textually but wrapped in `if false`. Not closable without `go/parser` and constant folding; worth a comment saying so |
| **…nor call order beyond "after the write"** | Moving `apply`'s check to after `f.rollback` does not fire it, though a comment says the order matters |
| **`bootBridges` has no test** | All four reconciler tests write the field directly, so deleting `setBootBridges` from `RestoreCurrent` leaves the suite green — and that call was the whole subject of the commit that added it |
| **The nft mutex is pinned only under `integration`** | `make test` cannot notice `mu sync.Mutex` being deleted. Self-documented in `nftables_mutex_test.go`, and CI's `test-integration` job does run it |

## Narrow races

| | |
|---|---|
| **Marker check to netlink write is not atomic** | Closed in 2.7 by re-reading the marker after every write. What remains is a microsecond window at the third site, which stats the marker three times — twice in the gate, once inside the helper via `PanicEngaged`. A marker becoming unreadable between the second and third read reintroduces the inversion. Pass the known state into the helper |
| **`Panic` and `Resume` share no lock** | `Resume`'s `ClearPanic` interleaving between `EngagePanic` and `nft.Reset` leaves no marker and an empty table. Visible — dashboard red, `status` exits 2 — and three lines of `panicMu` fix it |

## Wording and hygiene

| | |
|---|---|
| **`CHANGELOG.md`'s count claim** | Says the assertion in `daemon_dispatch_test.go` "now says seventeen"; it says at least fifteen. The loosening is correct — the bidirectional source checks are the real guard — the sentence is not |
| **`locales/de.json`'s `Fortsetzen`** | A hapax. Every other panic string uses `Notfallmodus`, and the same command is described as *beenden* elsewhere in the file |
| **The Docker reconcile reuses `RestoreReasonBoot`** | So a restore minutes after boot logs the detail "daemon start". The reason reaches the detail, not the action, so a third constant costs nothing in locales |
| **`CmdPanic`'s 35 s deadline** | On expiry `daemonAbsent` is false and the CLI reports the daemon is not answering — but the marker reached the disk in the first millisecond and the teardown lands moments later. A timeout should check the marker and say so |
| **The reconciler polls under panic mode** | For ninety seconds, and on finding a bridge logs "putting the rules back" immediately before `RestoreCurrent` logs the refusal. One check at the top removes a misleading pair |
| **`.opencode/opencode.json` is in the history** | Added and removed on the 2.7 branch by a blanket `git add`, so it survives unless the branch is squashed. `.gitignore` records the accident |

## Not carried — decided

Two things were ruled on rather than deferred, and should not be reopened without
a reason:

- **`easywall-core status` exits 2 when no daemon is running, whatever the marker
  says.** A machine with no daemon is not in the state it should be, even when
  deliberately unfiltered: nothing will restore the rules when panic mode ends.
  This was documented wrongly twice before it was documented right.
- **The panic banner has no button.** Ending panic mode from the web interface
  would let the network-facing process re-arm a firewall a human disarmed at the
  console, reachable by a stolen session. Whoever ran `panic` is at that console.
