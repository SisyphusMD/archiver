# #392 chore(gate): run the gate scripts and rules from project-rules main

- Kind: pull, state: closed, by cody, created 2026-11-24T19:11:36-08:00
- Labels: none; milestone: none
- refs/pull/392/head -> main, merged: True

Closes #391

This repo ran its own copies of the gate scripts and kept a copy of the rules for the AI review, and copies drift. `ci.yml` and `ai-review.yml` now fetch the scripts and `RULES.md` from project-rules main on every run and fail when they cannot (SisyphusMD/project-rules#78). The gate jobs and `ai-review.yml` are project-rules main's (6a6d41b4), and the copies are removed in the same change, so no run is left without either.

The removed copies also leave ruff.toml's exclude list. mirror-lockdown.yml is copied again for its source header.

Large diff: 890 of the removed lines are the deleted copies, and the added workflow lines come from project-rules main, where they were reviewed and tested.


## forgejo-actions, 2026-11-24T19:16:51-08:00

<!-- ai-review -->
**AI review** of `521ec223d1`: The PR correctly switches CI gate scripts and the AI review to fetch from project-rules main, removes the local copies, and updates ruff.toml and the changelog accordingly. The implementation matches the decision in project-rules#78 and issue #391.

No findings.

Disagree? Reply on this PR with your reasoning; the reviewer re-reads every reply.
<!-- ai-review-state W10= -->
