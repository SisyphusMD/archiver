# #183 fix(kit): adjust placed kit files without following links

- Kind: pull, state: closed, by cody, created 2026-10-08T16:25:15-07:00
- Labels: none; milestone: none
- refs/pull/183/head -> main, merged: True

Early security pass (ADR 42), scan finding F7: localAccess chowned and chmodded LOCAL_PATH/<kit> by path, following symlinks, in a directory the docs make user-owned. Now config must be a regular file (Lstat) and each placed file is opened O_NOFOLLOW, checked regular, and adjusted with fchown/fchmod. A link where the kit belongs makes the placement Unverified (retried next run). Test: TestLocalAccessFollowsNoLink.
