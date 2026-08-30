# The reviewer's first thirty

`locales/en.json` holds 461 keys. A reviewer checking a new language against it
in order reaches the sentences that matter most at roughly key 400, tired. This
page collects them so they are read first.

**The rule these sentences have to survive translation under:**

> A translator may rephrase freely. A translator may not change what the
> sentence *claims*.

An acceptance window that "keeps" a change in one language and "applies" it in
another describes a different product depending on which language you read it
in — not a stylistic difference, a different firewall. The same test applies to
every row below: does the translated sentence assert the same fact the English
one does, in the same direction?

**What made a key belong here:** not that the text is important, but that
getting it backwards would make an operator's decision about their firewall
*worse* — reachable instead of blocked, kept instead of undone, ended instead
of still engaged. Navigation labels, button text and generic error strings are
not here even where they are frequent, because a wrong translation of "Cancel"
produces confusion, not a lockout or an open port.

Six things are dangerous to get backwards in this codebase: which list is
consulted first, what the acceptance window promises, what panic mode does and
does not end, how the second factor and its recovery codes actually work, what
blacklisting and whitelisting actually do to traffic, and whether demo mode
touches a real firewall. Each is its own section below.

Each row names an id from `locales/en.json` and gives its current English
text, so a reviewer can check the translation without opening a second file.
The guard test (`internal/web/i18n_review_test.go`) fails if any id below is
renamed or removed from `en.json` without this page being updated to match.

## Rule evaluation order

Which list a packet meets first decides whether it is dropped or accepted —
independent of what any port rule says.

| id | English text |
|---|---|
| `blacklist_subtitle` | Sources that are dropped before any other rule is evaluated. |
| `blacklist_what_body` | The blacklist is evaluated first. An address listed here is dropped even if a port rule would have accepted it. |
| `blacklist_order_body` | This list is evaluated *before* the {}. An address that appears in both is dropped — a narrow allow inside a wide block does not work. |
| `whitelist_order_note` | The {} is evaluated first: an address in both lists is dropped, not allowed. |

## What the acceptance window promises

easywall's central safety property: an applied change reverts itself after a
timeout unless it is confirmed from a connection that still works. Saying it
is "saved" or "kept" instead of "will be undone unless you confirm" describes
a different product, and a reader who believes the wrong one can lock
themselves out with no warning.

| id | English text |
|---|---|
| `apply_rollback` | Rules will be rolled back automatically if not confirmed. |
| `system_acceptance_desc` | After an apply, easywall waits for you to confirm. No confirmation, and the previous rules come back. |
| `system_acceptance_enabled_desc` | Auto-rollback rules if not confirmed within the timeout |
| `system_warn_body` | Without the acceptance window an apply is final. A rule that closes your own SSH port leaves you with no way back in short of console access. |
| `firstrun_choices_desc` | Staged, not applied. Nothing reaches the firewall until you review and apply — and an apply undoes itself unless you confirm it. |
| `dashboard_staging_body` | Editing a rule set stages it. The running firewall changes only when you apply, and only stays changed if you confirm inside the acceptance window. |
| `apply_step3_desc` | Confirming keeps the new rules. Doing nothing restores the previous set when the window closes, which is what saves you if you just locked yourself out. |
| `apply_lead_pending` | The new rules are live but unconfirmed. Open a second connection and check that you can still reach this host — then confirm. |
| `apply_lead_rolled_back` | The window closed without a confirmation, so the previous rules were restored. Nothing you staged was lost — review it and apply again. |
| `staged_note` | Changes are staged until you {}. |
| `options_saved_note` | Options save immediately. Rule changes stay staged until you {}. |
| `settings_saved_note` | Saved immediately. Rule changes stay staged until you {}. |
| `accept_too_late` | Too late — the confirmation window had already closed, and the previous rules are back. Your edits are still staged; apply them again when you are ready. |

## Panic mode and recovery

What panic mode ends (filtering), what it deliberately does not end
(itself, across a restart), and that ending it is a console command with no
equivalent button in the interface.

| id | English text |
|---|---|
| `panic_banner_title` | Panic mode: this machine is not filtered |
| `panic_banner_body` | The firewall was taken down from the console and stays down across a restart. Run this on the machine to put your rules back: |
| `apply_panic_engaged` | Nothing was applied: panic mode is engaged. Run easywall-core resume on the machine to end it. |
| `audit_panic_engaged` | Panic mode engaged |
| `audit_panic_resumed` | Panic mode ended |
| `audit_apply_refused_panic` | Apply refused — panic mode is engaged |
| `audit_rollback_skipped` | Rollback skipped — panic mode is engaged |
| `audit_resume_restore_skipped` | Resume could not restore the rules |

## The second factor and recovery codes

That recovery codes are shown once, that a stale disk write can leave a code
usable a second time, that there is no reset by mail, and that the way back
after losing both a password and a second factor is editing a file on the
host — not a support request.

| id | English text |
|---|---|
| `firstrun_account_desc` | The only one. There is no recovery by mail — a lost password needs shell access to this server. |
| `password_lost_body` | There is no reset link — this interface sends no mail and reaches no outside service. Recovery means editing the web config on the host itself: the `password` line, and `totp_secret` and `recovery_codes` if a second factor is enrolled. {} |
| `recovery_left` | Recovery code used. {{.N}} left — make new ones under Password. |
| `recovery_not_consumed` | Recovery code accepted, but it could not be marked as used and will work again. Check the disk and make new codes under Password. |
| `totp_enabled` | Second factor enabled. Keep the eight recovery codes somewhere safe — they will not be shown again. |
| `totp_codes_once` | These will not be shown again. |
| `totp_disabled` | Second factor turned off. The password alone signs you in now. |
| `totp_recovery_renewed` | New recovery codes issued. The previous eight no longer work. |
| `totp_not_saved` | The code was right, but the second factor could not be saved. Check the disk and try again — this setup stays open. |

## Blacklist and whitelist semantics

What a block actually does to traffic, and that the whitelist skips the port
rules — not the protection modules, which still run first. `internal/core/
nftables.go:848–875` and `:1131–1144` (`addBogonFilter`, `addSSHBruteForce`)
build the chain in that order: protection modules, then the Docker bridge,
then the blacklist, then the whitelist, then the ports. The bogon filter is
the single exception — it exempts the whitelist and the Docker bridge because
its own premise ("nothing legitimately has this source address") is what an
operator contradicts by whitelisting a private network; nothing else in the
chain makes an exception for the whitelist. `whitelist_section_desc` and
`whitelist_narrow_body` said the opposite — "exempt from the protection
modules" and "bypasses every protection module" — until this review caught it;
see `internal/core/nftables_bogon_test.go:53` for the test that pins the one
real exception.

| id | English text |
|---|---|
| `blacklist_section_desc` | Nothing from these sources reaches an open port. |
| `whitelist_subtitle` | Trusted sources that reach every port, including ports you never opened. |
| `whitelist_section_desc` | Accepted before the port rules are consulted. The protection modules still run first — only the bogon filter makes an exception for this list. |
| `whitelist_wayback_note` | A whitelisted source skips the port rules entirely, so it reaches services that are not listed under {} at all. |
| `whitelist_narrow_body` | An entry here reaches every port, open or not. Prefer a single address over a range, and a range over a whole network. |
| `tile_blacklist_note` | dropped before any rule |
| `tile_whitelist_note` | bypass every port rule |

## Demo mode

That nothing in demo mode reaches a real firewall, and that its state is not
persistent.

| id | English text |
|---|---|
| `demo_notice` | Nothing reaches a real firewall; state resets periodically. |
| `demo_readonly` | This is the demo — the form works, the change is not saved. |
| `login_demo_notice` | Credentials are on the demo landing page. State resets when the server restarts. |
