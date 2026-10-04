# 22. Bundles convert with the 0.11 image

- Status: Accepted
- Date: 2026-10-03

## Context

ADR 4 drops bundle mode in v1: on finding a bundle, v1 refuses to start and points to the migration command, which today runs in the bash image. It rejects keeping a bash step in v1 and parsing `config.sh` without bash, so v1 cannot read a bundle's configuration itself. Where the conversion runs was left open.

## Options

1. The 0.11 image converts. v1 contains no bundle code.
2. v1 converts, running bash once in a throwaway process to read `config.sh`.

## Decision

Option 1.

## Consequences

- v1 refuses to start when it finds a bundle or a `config.sh`, and its message gives the one-off `archiver migrate` command to run with the 0.11 image.
- `release/0.11` keeps the converter, legacy `-k` bundles included, for as long as it receives patches.
- A cross-version test proves that what 0.11's `migrate` writes starts v1 and keeps backing up to the same storages and snapshot IDs.
- v1 never executes a configuration file as code, as ADR 4 requires.
