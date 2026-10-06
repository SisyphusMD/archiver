# 27. Real-service backends are tested weekly and before each release

- Status: Accepted
- Date: 2026-10-06
- Supersedes: 25 (its schedule only)

## Context

ADR 25 tests the backends without an emulator against dedicated test accounts every night.
The job is ours alone: users never run it. It guards against providers changing under us
(OAuth rules, APIs, quotas), which happens rarely and rolls out over weeks, so a nightly run
mostly re-confirms that nothing changed, and outside services' bad days surface as noise.

## Options

1. Nightly (ADR 25).
2. Weekly, plus a run before every release.
3. Before releases only.

## Decision

Option 2.

## Consequences

- The job runs weekly and on demand, and the release workflow runs it before tagging.
- A pre-release failure stops the release; a `skip_real_backends` input lets a release
  proceed when the failure is an outside service's outage, and the release notes say so.
- Weekly use keeps the test accounts' tokens alive (Microsoft's expire after 90 idle days).
- Everything else in ADR 25 stands.
