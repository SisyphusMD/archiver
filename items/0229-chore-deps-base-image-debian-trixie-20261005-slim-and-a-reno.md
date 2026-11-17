# #229 chore(deps): base image Debian trixie-20261005-slim and a Renovate rule for dated Debian tags

- Kind: pull, state: closed, by cody, created 2026-10-11T20:48:40-07:00
- Labels: none; milestone: none
- refs/pull/229/head -> main, merged: True

The image was on Debian 13.3 (trixie-20260112-slim). trivy against a main build found 72 HIGH/CRITICAL OS package vulnerabilities with Debian fixes; on trixie-20261005-slim (13.7) none with a fix remain.

Renovate listed the debian dependency but never proposed an update: trixie-YYYYMMDD-slim is not SemVer. The new packageRule parses it as compatibility-major.minor.patch, so newer dated tags are proposed.
