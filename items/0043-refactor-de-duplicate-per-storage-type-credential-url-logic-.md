# #43 refactor: de-duplicate per-storage-type credential/URL logic into shared helpers

- Kind: pull, state: closed, by cody, created 2026-07-06T15:27:07-07:00
- Labels: none; milestone: none
- refs/pull/43/head -> main, merged: True

Phase 1 of the config/overhaul work: retire the per-storage-type duplication behind the two do-spaces field incidents.

## What
Two shared helpers in `lib/core/config-loader.sh`, used by all three call sites (`duplicacy_primary_backup`, `duplicacy_add_backup`, `duplicacy_init_for_restore`):

- `build_storage_url <id>` — pure: the `local`/`sftp`/`b2`/`s3` URL string, one definition.
- `export_duplicacy_storage_secrets <id>` — the `sanitize -> UPPER -> DUPLICACY_<NAME>_*` credential-env mapping, one definition (the surface that broke in 0.8.10 and 0.8.11).

Once the URL is a variable, the `init`/`add` command is identical across types, so each function's 4-way branch collapses to one init/add call plus a small per-type block for the leftover `duplicacy set` credential writes. Net -92 lines behind the diffstat; 3 copies -> 1 for both concerns.

## No behavior change
- 11 new bats tests (`tests/storage_target.bats`) pin `build_storage_url` (all four types, region default, unsupported, target-specific id) and `export_duplicacy_storage_secrets` (per-type vars + the `do-spaces -> DUPLICACY_DO_SPACES_*` incident lock).
- Full suite 23/23 green; shellcheck clean (no new warnings); image builds; the `local` backup->restore roundtrip passes with content + mode + UID/GID preserved.

## Coverage note
The roundtrip only exercises `local` end-to-end (no creds/network in CI). `sftp`/`b2`/`s3` rest on the bats string-level tests + line-by-line preservation — which is the reason for collapsing them to a single definition each.
