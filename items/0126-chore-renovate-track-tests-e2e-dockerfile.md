# #126 chore(renovate): track tests/e2e/Dockerfile

- Kind: pull, state: closed, by cody, created 2026-09-30T22:31:36-07:00
- Labels: none; milestone: none
- refs/pull/126/head -> main, merged: True

includePaths listed only the root Dockerfile, so the e2e runner golang digest drifted. Also brings it to the current pin.
