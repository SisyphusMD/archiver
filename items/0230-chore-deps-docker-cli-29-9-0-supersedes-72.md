# #230 chore(deps): docker CLI 29.9.0 (supersedes #72)

- Kind: pull, state: closed, by cody, created 2026-10-11T20:53:53-07:00
- Labels: none; milestone: none
- refs/pull/230/head -> main, merged: False

Renovate #72 bumps the version but cannot update the archive checksums. Checksums computed from download.docker.com archives (amd64 33e1ab8b…, arm64 811f6eee…); the binary reports build f415da8, the commit of docker/cli tag v29.9.0, built with go1.26.9 (the 29.6.2 binary was go1.26.5, which trivy flags for 11 stdlib HIGHs). Renovate closes #72 once this merges.

## cody, 2026-10-11T21:15:07-07:00

Superseded by the update-everything PR, which carries the same docker CLI 29.9.0 bump and checksums.
