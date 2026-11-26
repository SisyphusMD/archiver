# #395 chore(gate): call the shared gate and AI review workflows

- Kind: pull, state: closed, by cody, created 2026-11-25T21:28:53-08:00
- Labels: none; milestone: none
- refs/pull/395/head -> main, merged: True

Closes #394

`ci.yml` and `ai-review.yml` carried copies of the gate and AI review jobs, and copies drift. They now call project-rules main's `gate.yml`, `gate-lint.yml` and `review.yml` (SisyphusMD/project-rules#78), as proven in SisyphusMD/homebrew-tap#34. They keep only the triggers, the concurrency groups and the one secret the jobs may see, `ISSUE_READ_TOKEN`. Check names stay the same; each calling job adds one check of its own (`CI / shared-gate` and so on), which nothing requires.

This PR is judged by the old copies, because `pull_request_target` runs main's files; the next PR here runs on the callers.

No tests: workflow wiring only, covered by project-rules' tests of the called files and the homebrew-tap proof run.

## forgejo-actions, 2026-11-25T21:31:54-08:00

<!-- ai-review -->
**AI review** of `e796c74f10`: Clean workflow-wiring change that replaces inline gate and AI-review job copies with `uses:` calls to project-rules' shared workflows, keeping only triggers, concurrency, and secret passthrough in the callers. Issue link, CHANGELOG, and the 'No tests' justification (workflow wiring validated by the called workflows' own tests and the homebrew-tap proof run) are all in order.

No findings.

Disagree? Reply on this PR with your reasoning; the reviewer re-reads every reply.
<!-- ai-review-state W10= -->
