# 42. The security audit runs early and again before release

- Status: Accepted
- Date: 2026-10-08

## Context

v1 handles storage credentials, RSA keys, a kit password and hooks that run with the
container's capabilities, and adds a web page and new restore paths. An audit only at the
end finds design problems when they are most expensive to change.

## Options

1. One audit, after the v1 features are built.
2. An early pass now, and the full audit before release.

## Decision

Option 2.

## Consequences

- Each pass combines the Claude Security scan, an independent Codex audit, and a manual
  review of secrets on disk and on command lines, hooks and what they receive, restore and
  locking, the web page, and the supply chain.
- Findings are ranked and the top ones walked through with Cody before anything is fixed.
- The early pass covers today's code; the final one, the code that ships.
