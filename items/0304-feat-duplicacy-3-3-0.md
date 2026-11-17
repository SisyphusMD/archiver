# #304 feat(duplicacy): 3.3.0

- Kind: pull, state: closed, by cody, created 2026-11-01T10:48:51-08:00
- Labels: none; milestone: none
- refs/pull/304/head -> main, merged: True

Replaces Renovate #284, which bumped the version but not DUPLICACY_SOURCE_SHA256.

- Source v3.3.0, sha256 dea27ecb…3b38b.
- dependencies.patch dropped: 3.3.0's go.mod pins exactly the versions it set (x/crypto 0.58.0, x/net 0.61.0, x/oauth2 0.37.0, Go 1.26).
- resume-point.patch: FindChunk gained isMetadata in 3.3.0; the resume check looks up file chunks, so false (as upstream's storage tests).
- The other patches apply unchanged; all 8 patch tests pass in the build; the arm64 Duplicacy stage (highwayhash patch) builds.
- B2 now uses the v4 API: the B2 fake already answers v4 authorize (apiInfo.storageApi) and lists contentLength.
- Checked against 3.3.0 source: the messages our code parses (missing chunks, decrypt failures, snapshot deletions, -stats, verified_chunks) are unchanged.
