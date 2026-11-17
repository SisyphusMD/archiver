# #220 fix(security): a restore never writes through a symbolic link in its destination

- Kind: pull, state: closed, by cody, created 2026-10-10T06:17:23-07:00
- Labels: none; milestone: none
- refs/pull/220/head -> main, merged: True

The V1 security follow-up from the early audit. Duplicacy 3.2.5 wrote a snapshot's files through an existing link to a directory (and into a link to a file when restoring in place), so whoever could write the destination chose where restored files landed; #186 only warned.

`build/duplicacy/restore-links.patch` (applied after resume-point.patch; its test runs in the image build):
- A link where the snapshot has a directory or a file: replaced with it when overwriting, the restore stops otherwise (RESTORE_LINKDIR / RESTORE_LINKFILE).
- Nothing (file, directory, link) is created, replaced or removed below a link to a directory that the restore does not replace (a partial restore): fails that entry (counted with allowFailures, else the restore stops).
- `-delete` does not delete the directory that replaced a link (a relative or nested link lists as `d`, the directory as `d/`).
- Quick mode skips a file as unchanged only when a regular file is really there (the listing follows top-level links, so a linked file looks unchanged).

TestRestoreNeverFollowsLinks covers refusal, replacement, partial restores below a linked parent (file, directory, link, nested file), -delete over relative and nested links, and quick mode over a linked file and a linked directory; nothing a link points to may change. Negative runs: the unpatched restore writes through both links; removing the deletion skip or the quick-mode guard each fails the test. (One quick-mode directory case Codex raised did not reproduce; its guard stays, exercised by the linked-file case.) Upstream TestBackupManager, TestPersistRestore, TestResumePoint pass.

Archiver: the pre-restore warning now says what happens; restore-commands.sh adds a linked file refused without OVERWRITE and replaced with it, the target untouched. restore-drill, recover, roundtrip pass; e2e passed.

Codex: 5 rounds (partial restores below a linked parent, -delete, quick mode found and fixed).

Upstream: to be offered in teaching mode with Cody only when he approves.
