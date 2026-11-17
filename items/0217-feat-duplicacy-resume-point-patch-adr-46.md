# #217 feat: Duplicacy resume-point patch (ADR 46)

- Kind: pull, state: closed, by cody, created 2026-10-10T04:59:38-07:00
- Labels: none; milestone: none
- refs/pull/217/head -> main, merged: True

The Duplicacy half of ADR 46: `build/duplicacy/resume-point.patch`, applied in the Dockerfile after the Dropbox patch, with its test run during the build.

- `EntryList.SaveResumePoint`: saves where a backup is without ending it. `incomplete_snapshot` (the entries, unchanged during the upload) is written once per backup and `incomplete_chunks` is replaced atomically each time; files and the directory are fsynced. It holds the upload-completion lock and is called from the packing goroutine (`uploadChunkFunc`) every `DUPLICACY_RESUME_INTERVAL` seconds (default 300; 0 = every chunk, negative = never). A crash at any instant leaves the previous point or this one; a pair from two backups fails the token check.
- Incremental (quick mode) backups now load an incomplete snapshot too, and `RunAtError` saves for every non-dry-run backup (was hash mode only).
- Before using an incomplete snapshot, every chunk it names that the cache does not already hold (the previous revision's, or the whole listing on a fast-listing first backup) is looked up on the storage; one missing discards it (`INCOMPLETE_DISCARD`). Covers an exhaustive prune deleting an interrupted backup's chunks once its snapshot ID counts as inactive (7 days), and preserved chunks from a first interruption after a second.
- A packed file's size and hash are written under the same lock, so the SIGINT save never sees one without the other.

Tests: `TestResumePoint` (first/incremental x in-memory/on-disk, a pruned uploaded chunk, interrupted twice with a first-attempt chunk pruned): stops a backup just after a resume point, checks the files finished then are not read again and every file restores byte for byte; -race clean; each fix has a negative run that fails without it. Upstream TestBackupManager/TestEntryList/TestPersistRestore pass. fault-interrupt.sh gains a SIGKILLed incremental backup (no signal handled) that resumes from its periodic save and restores byte for byte. Cross-version e2e suite passed on the final image (634 s).

Codex: diff reviews clean after fixes; a cold `cx verify` of the never-wrong-revision claim found the prune and torn-save holes (fixed). Rejected: a file rewritten with the same size and mtime keeps its old content on resume; that is quick mode's own trust model, and upstream's hash-mode resume already has it.

Upstream: to be offered to gilbertchen/duplicacy in teaching mode, with Cody, only when he approves.
