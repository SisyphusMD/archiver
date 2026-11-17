# #335 test(podman): name the log driver

- Kind: pull, state: closed, by cody, created 2026-11-13T02:10:46-08:00
- Labels: none; milestone: none
- refs/pull/335/head -> main, merged: True

The Podman 6 image sets journald as the log driver in a vendor drop-in that wins over its own containers.conf, and the test host runs no journal, so `podman logs` stayed empty and the readiness wait timed out. Archiver itself runs under Podman 6.1.3. With `--log-driver k8s-file` the test passes on 5.8.9 and 6.1.3, which unblocks #328.
