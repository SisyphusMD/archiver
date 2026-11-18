# #354 Close pull requests opened on the GitHub mirror

- Kind: pull, state: closed, by cody, created 2026-11-17T14:37:53-08:00
- Labels: none; milestone: none
- refs/pull/354/head -> main, merged: True

Closes #353

Copy the project-rules workflow that redirects new GitHub pull requests to Forgejo. The GitHub mirror will enable only the pinned lockdown action after the workflow is mirrored.

Validated the canonical file with actionlint and yamllint.

## forgejo-actions, 2026-11-17T14:38:47-08:00

<!-- ai-review -->
**AI review** of `6e29c7b737`: Clean, single-purpose workflow file that implements the mirror-lockdown described in issue #353. Permissions are minimal, the action is pinned by commit hash, the `if` guard correctly prevents execution on Forgejo or forks, and the PR is within all size and scope rules.

No findings.

Disagree? Reply on this PR with your reasoning; the reviewer re-reads every reply.
<!-- ai-review-state W10= -->
