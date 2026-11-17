# #252 fix(ci): pull the Podman test host through image-mirror

- Kind: pull, state: closed, by cody, created 2026-10-16T09:21:00-07:00
- Labels: none; milestone: none
- refs/pull/252/head -> main, merged: True

quay.io garbage-collected the pinned quay.io/podman/stable:v5.8.7 digest (77e53d9), so every PR fails Host-driven integration with failed to resolve reference. Repinned to the tag's current digest (2fb39ca9) and pulled through image-mirror.lan.bryantserver.com like the skopeo pins, since the mirror keeps digests it has served. Renovate still tracks it as quay.io/podman/stable.
