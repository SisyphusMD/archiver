# 4. Drop bundle mode in v1

- Status: Accepted
- Date: 2026-09-26

## Context

Bundle mode keeps the configuration in `bundle.tar.enc`, whose `config.sh` is bash that Archiver sources at start-up. Env-native configuration (environment variables plus secret files) has been the primary mode since 0.9.0, and removing bundles was already planned in July 2026. nas and vps are env-native; humblepixels/do-volume-ops still runs in bundle mode. Bundles are not on the never-break list in ADR 1.

## Options

1. Drop bundle mode in v1, with a tested migration first.
2. Keep bundle import through a small bash step that sources `config.sh`.
3. Parse the `config.sh` subset that `init` generates, without bash.

## Decision

Option 1. v1 is env-native only. When it finds a bundle, it refuses to start and points to the migration command. A tested migration command converts a bundle into `archiver.env` plus secret files; today's `archiver migrate` already does this from a running bash container.

## Consequences

- do-volume-ops moves to env-native before it upgrades to v1.
- `bundle export`, `bundle import`, and the bundle-password secret are removed in v1. The recovery kit replaces the bundle as the disaster-recovery artifact.
- v1 never executes a configuration file as code.
- `init` emits env-native materials only.
