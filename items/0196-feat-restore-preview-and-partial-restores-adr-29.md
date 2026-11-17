# #196 feat: restore preview and partial restores (ADR 29)

- Kind: pull, state: closed, by cody, created 2026-10-09T12:02:30-07:00
- Labels: none; milestone: none
- refs/pull/196/head -> main, merged: True

Stacked on #195 (doctor): merge that first; this PR then shows only 6e309ae. DRY_RUN=1 for auto-restore shows what would be added, replaced, deleted, and conflicts (differing files with overwrite off, where duplicacy stops part-way), from a scratch repository: the destination is never created or touched. The interactive restore shows the same preview before restoring (NO_PREVIEW=1 skips; Restore now? asked only at a terminal), suggests another directory when the snapshot is a configured service, connects and creates the destination only after confirmation, and offers RESTORE_PATHS in the advanced options. RESTORE_PATHS: anchored escaped regex include patterns (i:^path(/.*)?$ plus each ancestor dir itself for links); empty selections refused. Preview models duplicacy 3.2.5: 1s mtime tolerance, empty files truncate without -overwrite, no deletions with patterns, nested .duplicacy skipped, symlinks left out, right-aligned sizes and fixed 64-char hash column. tests/integration/restore-preview.sh. Full integration + e2e pass locally.
