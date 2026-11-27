# #396 ci: map PROJECT_RULES_TOKEN into the shared jobs

- Kind: pull, state: closed, by cody, created 2026-11-26T21:11:00-08:00
- Labels: none; milestone: none
- refs/pull/396/head -> main, merged: True

Refs SisyphusMD/project-rules#143

Trigger: the shared Lint, Secrets and AI review jobs are about to read the owner's private `.project-rules` manifest with the owner's read-only Actions secret `PROJECT_RULES_TOKEN`, and a called workflow sees only the secrets its caller maps in. This maps it, as the canonical callers in SisyphusMD/project-rules do. Until SisyphusMD/project-rules#140 merges the jobs ignore it, so nothing changes yet.

No tests: caller wiring only; the next PR after SisyphusMD/project-rules#140 merges proves it, since its Lint, Secrets and AI review checks read the manifest with this token.


## forgejo-actions, 2026-11-26T21:12:27-08:00

<!-- ai-review -->
**AI review** of `0cddfb909e`: Clean, minimal CI wiring PR that maps PROJECT_RULES_TOKEN into the shared-lint and shared-review callers exactly as specified in project-rules#143 step 2. The diff is small, the CHANGELOG entry is appropriate, and the 'No tests' justification (caller wiring only, proven by the next PR after #140 merges) is reasonable for a workflow YAML change.

No findings.

Disagree? Reply on this PR with your reasoning; the reviewer re-reads every reply.
<!-- ai-review-state W10= -->
