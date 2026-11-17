# #71 chore(deps): update dependency moby/moby to v29.7.2 - autoclosed

- Kind: pull, state: closed, by renovate, created 2026-07-30T15:03:18-07:00
- Labels: none; milestone: none
- renovate/moby-moby-29.x -> main, merged: False

This PR contains the following updates:

| Package | Update | Change |
|---|---|---|
| [moby/moby](https://github.com/moby/moby) | minor | `29.6.2` → `29.7.2` |

---

### Release Notes

<details>
<summary>moby/moby (moby/moby)</summary>

### [`v29.7.2`](https://github.com/moby/moby/releases/tag/docker-v29.7.2): v29.7.2

[Compare Source](https://github.com/moby/moby/compare/docker-v29.7.1...docker-v29.7.2)

##### 29.7.2

For a full list of pull requests and changes in this release, refer to the relevant GitHub milestones:

- [docker/cli, 29.7.2 milestone](https://github.com/docker/cli/issues?q=is%3Aclosed+milestone%3A29.7.2)
- [moby/moby, 29.7.2 milestone](https://github.com/moby/moby/issues?q=is%3Aclosed+milestone%3A29.7.2)

##### Bug fixes and enhancements

- Fix `docker service create` and `docker service update` panicking when the same environment variable is passed more than once. [docker/cli#7145](https://github.com/docker/cli/pull/7145)
- Fix a regression introduced in Docker Engine 29.7.0 that caused image pulls to reject images containing absolute hardlink targets, as produced by some image builders. [moby/moby#53305](https://github.com/moby/moby/pull/53305)
- Fix a regression introduced in Docker Engine 29.7.0 that could cause image pulls and `docker cp` to fail on older Linux kernels when applying file permissions, including for device nodes. [moby/moby#53305](https://github.com/moby/moby/pull/53305)

##### Packaging updates

- Update BuildKit to [v0.32.2](https://github.com/moby/buildkit/releases/tag/v0.32.2). [moby/moby#53300](https://github.com/moby/moby/pull/53300)

##### Networking

- Improve compatibility with more nftables releases by terminating base-chain policies with a semicolon. [moby/moby#53303](https://github.com/moby/moby/pull/53303)

### [`v29.7.1`](https://github.com/moby/moby/releases/tag/docker-v29.7.1): v29.7.1

[Compare Source](https://github.com/moby/moby/compare/docker-v29.7.0...docker-v29.7.1)

##### 29.7.1

For a full list of pull requests and changes in this release, refer to the relevant GitHub milestones:

- [docker/cli, 29.7.1 milestone](https://github.com/docker/cli/issues?q=is%3Aclosed+milestone%3A29.7.1)
- [moby/moby, 29.7.1 milestone](https://github.com/moby/moby/issues?q=is%3Aclosed+milestone%3A29.7.1)

##### Bug fixes and enhancements

- Fix a regression that prevented pulling images whose layers contain directories without explicit parent directory entries. [moby/moby#53260](https://github.com/moby/moby/pull/53260)
- Fix a regression where the `CopyToContainer` rejects container paths that traverse absolute symlinks, such as `/var/run` -> `/run`. [moby/moby#53261](https://github.com/moby/moby/pull/53261)

### [`v29.7.0`](https://github.com/moby/moby/releases/tag/docker-v29.7.0): v29.7.0

[Compare Source](https://github.com/moby/moby/compare/docker-v29.6.2...docker-v29.7.0)

##### 29.7.0

For a full list of pull requests and changes in this release, refer to the relevant GitHub milestones:

- [docker/cli, 29.7.0 milestone](https://github.com/docker/cli/issues?q=is%3Aclosed+milestone%3A29.7.0)
- [moby/moby, 29.7.0 milestone](https://github.com/moby/moby/issues?q=is%3Aclosed+milestone%3A29.7.0)

##### New

- Add an experimental `embedded-containerd` feature that runs containerd inside the daemon process instead of as a separate managed process. [moby/moby#52898](https://github.com/moby/moby/pull/52898)
- Mount type `image` is no longer experimental. [moby/moby#52998](https://github.com/moby/moby/pull/52998)
- Add the `default-stop-timeout` daemon option to configure the stop timeout assigned to containers without an explicit timeout. [moby/moby#53146](https://github.com/moby/moby/pull/53146)

##### Security

This release includes a fix for a security vulnerability affecting Docker Engine and related components.

- Update github.com/moby/go-archive to v0.3.0 to fix [CVE-2026-17106](https://www.cve.org/CVERecord?id=CVE-2026-17106) / [GHSA-hfg8-hc9c-6c3h](https://github.com/moby/go-archive/security/advisories/GHSA-hfg8-hc9c-6c3h). [moby/moby#53247](https://github.com/moby/moby/pull/53247), [docker/cli#7139](https://github.com/docker/cli/pull/7139)

##### Networking

- Fix a daemon panic when cleanup of a container's network interface fails while the container is being disconnected from a network. [moby/moby#53237](https://github.com/moby/moby/pull/53237)
- Fix a daemon panic when removing swarm ingress ports after failing to bind an ingress proxy listener. [moby/moby#53022](https://github.com/moby/moby/pull/53022)

##### Rootless

- Keep the cgroup mount for containers with `--net=host`. [moby/moby#52318](https://github.com/moby/moby/pull/52318)

##### Bug fixes and enhancements

- Add shell completion for `--filter` names and known values to `docker service ls`, `docker service ps`, and `docker node ps`. [docker/cli#7124](https://github.com/docker/cli/pull/7124)
- containerd image store: Fix daemon-wide concurrent download and upload limits for pulls and pushes not being honored. [moby/moby#53081](https://github.com/moby/moby/pull/53081)
  - To preserve the previous unlimited startup behavior, configure "max-concurrent-downloads" and "max-concurrent-uploads" to 0
- Fix `docker cp -a` using the wrong file owner when copying files into containers with user namespace remapping enabled. [moby/moby#53084](https://github.com/moby/moby/pull/53084)
- Fix `docker cp` from a Windows container silently returning a file instead of an error when the source path ends with a separator but is not a directory. [moby/moby#53123](https://github.com/moby/moby/pull/53123)
- Fix `docker stats` reporting all zeros for running Windows containers when using the containerd runtime. [moby/moby#53101](https://github.com/moby/moby/pull/53101)
- Fix `docker stats` reporting empty network stats for running Windows containers when using the containerd runtime. [moby/moby#53219](https://github.com/moby/moby/pull/53219)
- Fix a typo in the `docker create --pull` flag description. [docker/cli#7103](https://github.com/docker/cli/pull/7103)
- Fix Swarm service updates failing due to "file exists" errors when a VIP IP alias already exists on the LB endpoint interface. [moby/moby#51657](https://github.com/moby/moby/pull/51657)
- Fix Swarm tasks being rejected when their image could not be pulled from the registry but was already present on the node. [moby/moby#53212](https://github.com/moby/moby/pull/53212)
- Improve the error returned when a container hostname exceeds Linux's 64-byte limit. [moby/moby#53121](https://github.com/moby/moby/pull/53121)
- Prevent live-restored volumes from retaining active mount references when containers exit during daemon startup. [moby/moby#53115](https://github.com/moby/moby/pull/53115)
- Suppress the “No such container” error when `docker rm --force` succeeds for a nonexistent container. [docker/cli#7110](https://github.com/docker/cli/pull/7110)

##### Packaging updates

- Update Go runtime to [1.26.5](https://go.dev/doc/devel/release#go1.26.5). [docker/cli#7087](https://github.com/docker/cli/pull/7087)
- Update BuildKit to [v0.32.0](https://github.com/moby/buildkit/releases/tag/v0.32.0). [moby/moby#53234](https://github.com/moby/moby/pull/53234)
- Update containerd (static binaries) to [v2.3.3](https://github.com/containerd/containerd/releases/tag/v2.3.3). [moby/moby#53050](https://github.com/moby/moby/pull/53050)
- Update runc (in static binaries) to [v1.4.3](https://github.com/opencontainers/runc/releases/tag/v1.4.3). [moby/moby#50960](https://github.com/moby/moby/pull/50960)

</details>

---

### Configuration

📅 **Schedule**: (UTC)

- Branch creation
  - At any time (no schedule defined)
- Automerge
  - At any time (no schedule defined)

🚦 **Automerge**: Disabled by config. Please merge this manually once you are satisfied.

♻ **Rebasing**: Whenever PR becomes conflicted, or you tick the rebase/retry checkbox.

🔕 **Ignore**: Close this PR and you won't be reminded about this update again.

---

 - [ ] <!-- rebase-check -->If you want to rebase/retry this PR, check this box

---

This PR has been generated by [Mend Renovate CLI](https://github.com/renovatebot/renovate).
<!--renovate-debug:eyJjcmVhdGVkSW5WZXIiOiI0My4yODguMCIsInVwZGF0ZWRJblZlciI6IjQ0LjE3LjIiLCJ0YXJnZXRCcmFuY2giOiJtYWluIiwibGFiZWxzIjpbIm1pbm9yLXVwZGF0ZSJdfQ==-->
