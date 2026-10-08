# 39. Which CI checks block and which report

- Status: Accepted
- Date: 2026-10-08

## Context

CI enforces gofmt, go vet, unit tests with the race detector, the integration scripts, the
black-box suite and the emulated backends. It does not lint, measure coverage or fuzz. A
check that blocks must catch real defects without inviting busywork.

## Options

For each of linting, coverage and fuzzing: enforced (blocks a change) or advisory (reports
only).

## Decision

Linting enforced; coverage advisory; fuzzing enforced briefly per change and advisory at
length weekly (Cody accepted this after an explanation of the trade-offs).

## Consequences

- golangci-lint (staticcheck, errcheck, ineffassign, unused, gosec) runs on every change;
  existing findings are fixed once, and any new one fails the change.
- Coverage is reported per package in each run, with no floor, which would reward tests
  written for the number.
- Fuzz targets cover configuration and secret parsing, storage URL building,
  SERVICE_DIRECTORIES expansion and preferences parsing: about 30 seconds each per change,
  longer weekly; a crash found becomes a regression test.
- The backend matrix (emulated types in CI, real services weekly, ADR 27) stays as it is.
