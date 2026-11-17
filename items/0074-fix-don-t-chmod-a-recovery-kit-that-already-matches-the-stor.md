# #74 fix: don't chmod a recovery kit that already matches the storage (it discards the inherited ACL)

- Kind: pull, state: closed, by cody, created 2026-08-16T17:39:23-07:00
- Labels: none; milestone: none
- refs/pull/74/head -> main, merged: True

0.10.4 stages the kit on a fresh inode so it inherits the destination directory's ACL, then stamps it with the storage's own mode. On an ACL-backed share the stamp undoes the staging: any `chmod` there discards the inherited ACL, including a `chmod` to the mode the file already carries.

Observed on a Synology volume backing a `local` target. The two views disagree:

| | inside the container | on the host |
|---|---|---|
| `config` | `700`, no ACL visible | `777+`, ACL grants the mirror user |
| freshly created file | `0` | `777+` (inherited) |

So the kit was chowned (ACL survives), then chmod'ed `700` onto a file already reading `700`, which dropped the ACL. Verification compared `700` to `700`, passed, and the placement was recorded as done — a kit the mirror user could not read, on a target archiver considered current. Confirmed by experiment on the share: `cp` inherits the ACL, `chown` preserves it, `chmod` destroys it.

The placement now chmods only when that would change the mode, so a file already matching the storage keeps whatever the directory granted it. A placement below the reference is still stamped up to it, unchanged.

The SFTP path is deliberately untouched: there the mode reads back as the ACL-synthesized `777` and already matches, and applying the same skip rule would collide with `recovery_kit_sftp_mode` treating a bare `0` as unreadable, producing a permanent re-upload loop.

`RECOVERY_KIT_STATE_VERSION` is bumped to 5 so existing deployments re-place their kit once after upgrade.

Tests: two new cases in `tests/recovery_kit.bats` (no chmod when the mode already matches; still chmod'ed up when it does not). Full suite 88/88.
