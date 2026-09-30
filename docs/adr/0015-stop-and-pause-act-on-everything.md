# 15. Stop and pause act on everything

- Status: Accepted
- Date: 2026-09-30

## Context

Copies now run apart from the backup (ADR 11), so stop and pause could gain per-target forms, and a stopped worker could either retry later or stay held. People mostly use stop and pause on a one-off run they are watching.

## Options

1. Stop and pause apply to all running work at once, with no per-target controls.
2. Per-target `stop copies` / `pause copies`, with a stopped worker held until resumed.

## Decision

Option 1. Stop interrupts everything and nothing resumes it. Pause freezes everything, resume continues it where it was, and nothing new starts while paused.

## Consequences

- After a stop there is no retry or catch-up; the next scheduled backup and maintenance run as normal, and each worker copies again when local next changes.
- A stopped run still exits non-zero, and no copy, check or prune follows a stopped backup (contract behaviors).
- No held state exists to be forgotten; `archiver status` shows a pause and when it began.
- A container restart ends a pause, since frozen processes do not survive it.
