# 30. `archiver doctor` checks a deployment end to end

- Status: Accepted
- Date: 2026-10-08

## Context

When something seems off, or before an upgrade, the state of a deployment is spread over
status, healthcheck, logs and the storages themselves, and some problems (an unmounted logs
volume, a missing capability) show only when they bite.

## Options

Which checks one read-only command runs: configuration and secrets; storage reachability;
container setup; backup freshness and the recovery kit.

## Decision

All four groups.

## Consequences

- Configuration and secrets: every setting valid, every secret file present and non-empty,
  the keys decrypt with the passphrase, the kit password set, notifier credentials accepted
  (a test notification only with `--notify`).
- Storages: each reachable with its credentials, initialized with the expected encryption,
  this deployment's snapshot IDs present. Nothing is written.
- Container: capabilities, logs volume mounted, free space for logs and the drill directory,
  service directories present, hooks executable.
- Freshness: last successful backup per service, copy workers caught up, the kit current on
  every storage, the envelope confirmed and current, the last drill's result.
