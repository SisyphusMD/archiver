# #258 fix(kit): never chmod the recovery kit on a Synology ACL share

- Kind: pull, state: closed, by cody, created 2026-10-18T17:43:31-07:00
- Labels: none; milestone: none
- refs/pull/258/head -> main, merged: True

On a Synology share mounted into the container, an ACL file shows a mode that depends on its owner: the freshly placed kit stats 0 while root-owned and 0700 after the chown to config's owner. `adjust()` compared the pre-chown stat with config's 0700, chmod'ed, and any chmod on synoacl converts the file to plain Linux mode, dropping the inherited ACL that grants the mirror user read. The readability check passed because config also reads 0700 inside the container.

- re-stat after the chown
- when config carries a Synology ACL (`system.syno_acl_inherit`/`_self` xattr), never chmod, and require the kit to carry one too (else Unverified, re-placed next run)
- stateVersion 5 -> 6 so existing deployments re-place once

Proven on the NAS in a scratch share with the rc.1 image and caps: rclone copyto, rename and chown keep the ACL; chown + chmod reproduces the 0700 Linux-mode kit; a probe of the new logic detects the ACL through /proc/self/fd and flags a stripped file. Unit tests, go vet and gofmt pass.
