# 6. Go for v1, proven by a vertical slice first

- Status: Accepted
- Date: 2026-09-27

## Context

v1 adds a per-target circuit breaker with persisted state, alert deduplication, bounded concurrency, restore drills that verify hashes, metrics, fuzzed config parsing, and executable hooks. Bash has no data structures, error values, or concurrency control, which makes each of these fragile. The image must still carry duplicacy, the docker CLI, systemctl, zfs, openssl, and bash for hooks, so no language gets a minimal image. Cody built and maintains Archiver alone.

## Options

1. Go, committed only after a throwaway vertical slice proves it on the real failure paths.
2. Go, committed now.
3. The same slice in Go and in Python, chosen by reading them.
4. Stay with bash and cut the v1 scope.

## Decision

Option 1. Before Phase 3 starts, a throwaway Go slice must back up and copy to test storages, cancel duplicacy without skipping post hooks, keep circuit-breaker state across a restart, and pass a cross-version restore check against data written by the bash release. If it falls short, the language question reopens.

## Consequences

- July 2026's "stay bash" reasoning is superseded: its image-size point still holds but no longer decides anything, and its "Go wins are cheaper in bash" point does not survive v1's scope.
- The slice is discarded after the gate; it is evidence, not the first commit of v1.
- Cody learns Go concepts as they come up (context, errors as values, interfaces, os/exec process groups, testing with fuzzing and -race).
- Serena is reconfigured for Go before Phase 4.
