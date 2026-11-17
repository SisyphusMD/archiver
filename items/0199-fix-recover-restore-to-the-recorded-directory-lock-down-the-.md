# #199 fix(recover): restore to the recorded directory; lock down the output directory

- Kind: pull, state: closed, by cody, created 2026-10-09T13:41:50-07:00
- Labels: none; milestone: none
- refs/pull/199/head -> main, merged: True

Two review findings on the just-merged recover (#197):

1. With SERVICE_DIRECTORIES=/srv/apps/*/:/srv/databases/*/ a service backed up from /srv/databases/db was recovered into /srv/apps/db (both globs take the name; the first won). The kit's RECREATE.txt now records each service's absolute directory, one per line (spaces survive), and recover restores there. For kits without the list, a name two globs could take is left for a person instead of guessed.
2. An existing --out directory others could write to kept its mode, and copies followed links. The directory is now made 0700 (a link refused) before anything is written; every file is created fresh (old entry removed, O_EXCL|O_NOFOLLOW).

Rejected: keeping a parser for the previous one-line services format. It existed only on main since #197 earlier today and never shipped, so no kit carries it.

tests/integration/recover.sh now uses two glob roots and a world-writable pre-existing output dir; unit tests for the parser, ambiguous globs and link-safe writes. recovery-kit, restore-preview, doctor, envelope integration pass locally.
