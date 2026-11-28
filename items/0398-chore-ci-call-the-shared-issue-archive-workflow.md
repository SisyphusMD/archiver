# #398 chore(ci): call the shared issue-archive workflow

- Kind: pull, state: closed, by cody, created 2026-11-27T07:11:20-08:00
- Labels: none; milestone: none
- refs/pull/398/head -> main, merged: True

Replaces the copied issue-archive job with the caller from project-rules main and deletes the copied script, which no longer runs.

Closes #397


## forgejo-actions, 2026-11-27T07:17:53-08:00

<!-- ai-review -->
**AI review** of `0cb10eb209`: Clean infrastructure change: replaces the copied issue-archive job and script with a call to project-rules' shared workflow, as specified in #397. The diff is minimal, the secret scoping is correct, and the CHANGELOG entry documents the change.

No findings.

Disagree? Reply on this PR with your reasoning; the reviewer re-reads every reply.
<!-- ai-review-state W10= -->
