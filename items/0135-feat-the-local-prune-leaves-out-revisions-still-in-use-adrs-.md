# #135 feat: the local prune leaves out revisions still in use (ADRs 19, 21)

- Kind: pull, state: closed, by cody, created 2026-10-03T08:02:21-07:00
- Labels: none; milestone: none
- refs/pull/135/head -> main, merged: True

The primary prune takes its deletions from duplicacy prune -dry-run, leaves out revisions a copy worker is copying or a restore is reading (internal/inuse registry, locked file per reader, per-storage gate), and prunes the rest with explicit -r per snapshot ID. Mirroring likewise keeps revisions a restore is reading from a secondary. ADR 21: the prune keeps running alongside backups (keeps the maintenance-overlap contract).
