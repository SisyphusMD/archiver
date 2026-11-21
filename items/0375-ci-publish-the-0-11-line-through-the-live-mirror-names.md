# #375 ci: publish the 0.11 line through the live mirror names

- Kind: pull, state: closed, by cody, created 2026-11-20T18:16:04-08:00
- Labels: none; milestone: none
- refs/pull/375/head -> release/0.11, merged: True

Closes #374.

The v0.11.8 tag publish failed before pushing anything: release/0.11 still pulled skopeo from the retired image-mirror.nas name and mirrored to forgejo.nas. This uses the .lan names main has published every rc through since ead5944, and main's skopeo v1.24.1 digest (the v1.22.3 digest is gone from quay and the mirror). skopeo 1.24.1 accepts every flag these steps pass (checked with copy/sync --help). CHANGELOG notes that v0.11.8 has a tag but no image.

No tests: the publish jobs run only on a tag push, so the next release run is the proof.
