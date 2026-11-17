# #41 test: backup->restore round-trip integration test (gates releases)

- Kind: pull, state: closed, by cody, created 2026-07-04T21:06:16-07:00
- Labels: none; milestone: none
- refs/pull/41/head -> main, merged: True

Adds tests/integration/roundtrip.sh: inside the built image, against an ephemeral local storage target (no S3/B2/SFTP, no network), backs up a fixture tree with root/1000/5000 ownership + varied modes + a symlink, restores into a fresh dir, and asserts content hash + mode + UID/GID are all preserved. Guards against a false green by hard-failing if it cannot set non-root ownership (missing CHOWN/FOWNER). Wired as a new `integration` CI job (amd64 build + round-trip via stdin under the restore caps) and added to build-and-push needs so it blocks releases. Verified locally: passes with caps, fails loudly without.
