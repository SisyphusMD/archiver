# #110 fix(ci): pull skopeo through the NAS image mirror

- Kind: pull, state: closed, by cody, created 2026-09-28T12:39:20-07:00
- Labels: none; milestone: none
- refs/pull/110/head -> main, merged: True

quay garbage-collects a re-pushed tag's old digest, so a pinned skopeo digest could 404 and fail a publish after its tag was cut (health-vault 0.60.7 shipped without an image this way). The NAS zot mirror now keeps every quay.io/skopeo digest it serves. Renovate still tracks quay.io/skopeo/stable and bumps the mirror-prefixed pin.
