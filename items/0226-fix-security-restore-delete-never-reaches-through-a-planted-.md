# #226 fix(security): restore delete never reaches through a planted link (scan F4)

- Kind: pull, state: closed, by cody, created 2026-10-11T09:26:28-07:00
- Labels: none; milestone: none
- refs/pull/226/head -> main, merged: True

Security scan 2026-10-11, finding F4: Duplicacy's local listing follows a top-level link to an absolute path, so restore -delete removed everything under the link's target that the snapshot lacked, as root. The delete loop now skips any extra entry reached through a link (linkedAncestor), and removes an extra entry that is itself a link as the link (no trailing slash).

The patch's test gains the planted-link case; it fails on the current patch (shown) and passes with this one. restore-extras, restore-commands and restore-preview pass on the built image.

F11 (the check-then-write race while someone changes the destination during a restore) is not addressed here: docs/restore.md already says to restore into a directory nothing else changes meanwhile; a real fix needs fd-relative writes through Duplicacy's restore path.
