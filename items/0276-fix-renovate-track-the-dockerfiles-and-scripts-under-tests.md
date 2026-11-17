# #276 fix(renovate): track the Dockerfiles and scripts under tests/

- Kind: pull, state: closed, by cody, created 2026-10-24T08:42:38-07:00
- Labels: none; milestone: none
- refs/pull/276/head -> main, merged: True

config:recommended includes :ignoreModulesAndTests, whose ignorePaths has **/tests/**, so includePaths entries tests/e2e/Dockerfile, tests/fakes/Dockerfile and tests/integration/podman.sh were never updated. Their golang pin sat at 1.27.2 while the build image moved to 1.27.3, and #270 (go 1.27.3 in go.mod) failed the e2e build. Overrides ignorePaths with the preset minus the test globs and brings both test golang pins to the build image digest. renovate-config-validator --strict passes on Renovate 44.151.2.
