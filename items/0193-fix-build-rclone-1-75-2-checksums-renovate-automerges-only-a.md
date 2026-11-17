# #193 fix(build): rclone 1.75.2 checksums; Renovate automerges only after CI

- Kind: pull, state: closed, by cody, created 2026-10-09T07:00:36-07:00
- Labels: none; milestone: none
- refs/pull/193/head -> main, merged: True

main does not build since Renovate #192 (rclone 1.75.1 -> 1.75.2) kept the 1.75.1 RCLONE_SHA256_* values. New values match rclone SHA256SUMS, whose clearsigned signature verifies with Nick Craig-Wood key FBF737EC...54FA from rclone.org/KEYS. Root cause: Renovate merged #192 3s after opening, with CI pending (Forgejo platform automerge, no required checks on main); #191 (skopeo digest) the same way. platformAutomerge:false makes Renovate wait for green statuses itself; checksum-pinned deps (rclone, moby, duplicacy) never automerge. Image built locally; rclone version reports 1.75.2.
