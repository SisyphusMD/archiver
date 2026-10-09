# 43. Documentation is markdown in the repository, with a generated reference

- Status: Accepted
- Date: 2026-10-08

## Context

The README carries most of the documentation, grown release by release, with guides under
`docs/guides`. v1 changes enough of the surface that the docs are rewritten (quick start,
task guides, configuration reference, flow diagrams).

## Options

1. Markdown in the repository.
2. A published documentation site built from the same content.

## Decision

Option 1.

## Consequences

- The README becomes an overview and quick start; task guides live in `docs/`, readable on
  Forgejo and GitHub with nothing to host.
- The configuration reference is generated from the code's settings table, and CI fails
  when it is out of date.
