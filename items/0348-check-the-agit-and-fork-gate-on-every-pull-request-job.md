# #348 Check the AGit and fork gate on every pull-request job

- Kind: pull, state: closed, by cody, created 2026-11-17T13:37:35-08:00
- Labels: none; milestone: none
- refs/pull/348/head -> main, merged: True

Closes #347

The PR workflow checks every `pull_request` job under `.forgejo/workflows/`. It requires the same-repository and standard-flow conditions together. Shellcheck and changelog validation can remain ungated as data-only jobs. The three release jobs must retain their exact tag-only conditions.

Tests: the checker passed on the current workflows and rejected copies with the whole guard removed, either clause removed, OR used between clauses, or a tag-only exemption changed.
