# 48. A drill hook proves a restored service is usable

- Status: Accepted
- Date: 2026-10-13

## Context

A drill (ADR 28) proves files come back; only the user knows whether the service can use
them (a dump that loads, a database that opens). The NAS ran such checks in a separate
container (archiver-drill) that started its own containers; that container is to go away.

## Options

1. A per-service `drill` hook, run by Archiver after a drill restore.
2. The same hook, forbidden from using Docker.
3. Application checks outside Archiver (a separate container or a cluster job).

## Decision

Option 1. After a drill restores a revision to its scratch directory, Archiver runs the
service's executable `drill` hook (from the service directory or `HOOKS_DIR`) in that
directory. It receives the paths, snapshot ID, revision and storage as `ARCHIVER_*`
variables, never storage credentials or the RSA passphrase; it is held to the hook rules
(ADR 45); exit 0 passes; it has a time limit; the scratch copy is deleted afterwards.

## Consequences

- A failed or timed-out hook fails the drill, with the same incident, status and metrics.
- A hook may use whatever the deployment mounts, Docker included: the NAS keeps the socket
  (restores and its drill hooks start and stop containers), and its cold-garage checks move
  into a hook in its own repository. Moving cold-garage to a cluster job is a later option.
