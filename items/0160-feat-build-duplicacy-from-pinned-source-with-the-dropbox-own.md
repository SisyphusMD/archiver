# #160 feat: build Duplicacy from pinned source with the Dropbox own-app patch (ADR 26)

- Kind: pull, state: closed, by cody, created 2026-10-06T08:01:32-07:00
- Labels: none; milestone: none
- refs/pull/160/head -> main, merged: True

Stacked after #159 (its commit shows here until #159 merges).

Duplicacy 3.2.5 is now built from the pinned source tarball (SHA-256 checked, modules checked against its go.sum) with one patch, `build/duplicacy/dropbox-app.patch`, instead of the released binary (ADR 26):

- A Dropbox storage with `dropbox_app_key` / `dropbox_app_secret` refreshes its token directly with Dropbox through the user's own app; without them nothing changes (duplicacy.com, as before).
- A token Dropbox rejects before its expiry is dropped, so go-dropbox's own 401 retry gets a fresh one. `TestDropboxAppTokensReplaceRejected` runs in the image build.
- Every other storage type is untouched: same version, same storages and snapshots.

Verified locally on the built image: all integration scripts and the full e2e suite pass. The same patch goes upstream as a PR to gilbertchen/duplicacy (Cody reviews it first).
