# 29. Restores preview, target other paths and files, and recover a host step by step

- Status: Accepted
- Date: 2026-10-08

## Context

`archiver restore` asks for a storage, snapshot ID, directory and revision, then restores
the whole snapshot, merged into the directory unless told to overwrite or delete. It shows
nothing before writing, restores only whole snapshots, and rebuilding a lost host from the
recovery kit is a manual sequence.

## Options

Each independently: a dry-run preview; restoring to another path as a first-class choice;
restoring single files or subfolders; a guided disaster-recovery command.

## Decision

All four.

## Consequences

- A preview lists what a restore would add, replace and delete, and its size, from the
  revision's file list and a size-and-time walk of the destination (duplicacy's restore has
  no dry run). It costs the metadata download, which the restore then reuses from cache.
  Interactive restores show it by default (skippable); `auto-restore` with `DRY_RUN=1`.
- Restoring a revision into another directory leaves the live service and its repository
  untouched, and is the suggested choice while the service is running.
- `RESTORE_PATHS` (and the interactive flow) restore matching paths only.
- `archiver recover` rebuilds keys, secrets and configuration from the kit and its password
  (or the break-glass envelope) on a fresh host, then restores every service, checking each
  step.
