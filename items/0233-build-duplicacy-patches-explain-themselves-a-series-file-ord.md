# #233 build(duplicacy): patches explain themselves, a series file orders them, and Duplicacy's x/ dependencies are current

- Kind: pull, state: closed, by cody, created 2026-10-12T15:07:05-07:00
- Labels: none; milestone: none
- refs/pull/233/head -> main, merged: True

Two commits.

1. Each patch in build/duplicacy/ opens with what it changes, why, the test that proves it and its upstream status; build/duplicacy/series lists the order. The Dockerfile applies the series (patches touching vendor/ after go mod vendor), collects the tests from the patches' `Test: TestX` lines and requires every one to pass. The old Dockerfile comment (which had missed gcd-drive-id) shrinks to a pointer.
2. dependencies.patch: Duplicacy's golang.org/x/crypto v0.12.0 -> v0.58.0, x/net v0.10.0 -> v0.61.0, x/text, x/oauth2 to latest (Go 1.19 -> 1.26 language version; grpc stays v1.28.1, pinned by Google's cloud libraries). Patch tests run with -vet=off (Go 1.26 vet rejects upstream format strings). Duplicacy's TestPersistRestore needs GODEBUG=randseednop=0 under Go 1.24+ and is not run by the build; with it the upstream suite matches the unpatched baseline.

Verified locally on the image: the 5 patch tests; sftp-runtime; every backends.sh type; all 43 round-trip scripts; the e2e cross-version suite; arm64: the official 3.2.5 arm64 binary and ours restore each other's erasure-coded backups byte-identical with check -chunks clean. The first commit alone builds and passes its tests.
