# #219 feat: host-side archiver command and Podman parity in CI (ADR 5)

- Kind: pull, state: closed, by cody, created 2026-10-10T05:21:12-07:00
- Labels: none; milestone: none
- refs/pull/219/head -> main, merged: True

Stacked on #218 (docs) on #217: merge those first. Closes the last two ADR 5 backlog items (the other two, stop's error count and Ctrl+C in `archiver logs`, were fixed by the Go rewrite).

- `lib/host/archiver` (POSIX sh, in the image at `/opt/archiver/host/archiver`): `archiver ARGS` on the host runs `archiver ARGS` in the container. Picks docker, then podman, then `/usr/local/bin/docker` (Synology), or `ARCHIVER_RUNTIME`; container `ARCHIVER_CONTAINER` (default archiver); adds sudo when this user cannot see the container (probes `container inspect`, so a rootless store answering does not hide a rootful container); `-it` only when stdin and stdout are terminals, `-i` otherwise. Shellchecked in CI.
- tests/host_command.bats: runtime choice, ARCHIVER_CONTAINER/ARCHIVER_RUNTIME, sudo fallback (as a non-root user), missing runtime.
- tests/integration/podman.sh (host-driven CI step): pinned quay.io/podman/stable in a privileged container (CI's DinD allows it); archiver under rootful Podman with cap-drop ALL + DAC_OVERRIDE/CHOWN/FOWNER and no-new-privileges; a pre-backup hook uses the docker CLI against Podman's API socket mounted as docker.sock (`docker exec` into another container, `docker ps`); the host command installed with `podman cp` runs backup and status; `podman stop` exits 0. Hook safety also checked under Podman (it refused a 777 parent until fixed). Volumes removed with `rm -fv`.
- Docs: commands.md "From the host", installation.md "Podman" (rootful, as tested).

Codex: found the rootless-store probe and the leaked Podman volumes (both fixed); then clean.
