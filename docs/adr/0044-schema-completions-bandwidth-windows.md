# 44. A configuration schema, shell completions, and bandwidth limits with backup windows

- Status: Accepted
- Date: 2026-10-08

## Context

Proposed for v1 beyond the planned phases (metrics and the web page are ADRs 35 and 38):
help writing configuration, help typing commands, and control over when and how fast data
leaves the host.

## Options

Each independently: a configuration schema for editors; shell completions; per-storage
bandwidth limits and windows for copies.

## Decision

All three.

## Consequences

- A JSON schema of every setting, generated with the reference (ADR 43), lets editors
  autocomplete and validate compose and env files.
- `archiver completion bash|zsh|fish` prints completions for commands and flags.
- `STORAGE_TARGET_N_UPLOAD_LIMIT` caps a storage's upload rate (duplicacy's `-limit-rate` for
  a backup to it, `-upload-limit-rate` for a copy to it),
  and `STORAGE_TARGET_N_COPY_WINDOW` (such as `01:00-06:00`) holds its copy worker outside
  those hours; backups to the primary are unaffected.
