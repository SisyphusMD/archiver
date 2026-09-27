# 7. v1 runs the pinned duplicacy binary

- Status: Accepted
- Date: 2026-09-27

## Context

Duplicacy is written in Go, so a Go rewrite could import it as a library instead of running its CLI. Its license (custom, non-OSI) permits modification and redistribution but makes commercial use of derivative works subject to its paid per-computer terms, which conflicts with Archiver's AGPL-3.0. Its library signals errors by panicking (`LOG_ERROR` panics unless a `LogFunction` is installed, in which case execution continues past the error), its top-level handler calls `os.Exit`, it relies on process-wide settings, and its dependencies are old (aws-sdk-go v1 from 2020, a deprecated jwt-go).

## Options

1. Keep executing the checksum-pinned duplicacy CLI as a child process.
2. Import duplicacy's Go package into the Archiver binary.

## Decision

Option 1.

## Consequences

- No license question arises from linking, and Archiver stays AGPL-3.0 without exceptions.
- A duplicacy failure cannot crash or corrupt the Archiver process; its exit code and output are the interface.
- Upgrading duplicacy stays a pinned version and checksum swap.
- Tests can keep injecting faults by replacing the binary on PATH.
- v1 must parse duplicacy's text output where it needs structured data (revision lists, stats), and pins its behavior with tests.
