# #13 chore(deps): update dependency moby/moby to v29.5.3 - autoclosed

- Kind: pull, state: closed, by cody, created 2026-05-14T15:04:10-07:00
- Labels: none; milestone: none
- renovate/moby-moby-29.x -> main, merged: False

This PR contains the following updates:

| Package | Update | Change |
|---|---|---|
| [moby/moby](https://github.com/moby/moby) | minor | `29.4.3` → `29.5.3` |

---

### Release Notes

<details>
<summary>moby/moby (moby/moby)</summary>

### [`v29.5.3`](https://github.com/moby/moby/releases/tag/docker-v29.5.3): v29.5.3

[Compare Source](https://github.com/moby/moby/compare/docker-v29.5.2...docker-v29.5.3)

##### 29.5.3

For a full list of pull requests and changes in this release, refer to the relevant GitHub milestones:

- [docker/cli, 29.5.3 milestone](https://github.com/docker/cli/issues?q=is%3Aclosed+milestone%3A29.5.3)
- [moby/moby, 29.5.3 milestone](https://github.com/moby/moby/issues?q=is%3Aclosed+milestone%3A29.5.3)

##### Bug fixes and enhancements

- Reduce `docker system df` errors when images are pruned at the same time with the containerd image store. [moby/moby#52672](https://github.com/moby/moby/pull/52672)

##### Packaging updates

- Update containerd (static binaries only) to [v2.2.4](https://github.com/containerd/containerd/releases/tag/v2.2.4). [moby/moby#52683](https://github.com/moby/moby/pull/52683)
- Update Go runtime to [1.26.4](https://go.dev/doc/devel/release#go1.26.4). [moby/moby#52753](https://github.com/moby/moby/pull/52753), [docker/cli#7025](https://github.com/docker/cli/pull/7025)
- Update RootlessKit to [v3.0.1](https://github.com/rootless-containers/rootlesskit/releases/tag/v3.0.1). [moby/moby#52710](https://github.com/moby/moby/pull/52710)

##### Rootless

- Fix AWS IMDS access with `gvisor-tap-vsock` and UDP port forwarding for non-loopback clients. [moby/moby#52710](https://github.com/moby/moby/pull/52710)
- Fix installation of plugins that require host networking. [moby/moby#52735](https://github.com/moby/moby/pull/52735)

### [`v29.5.2`](https://github.com/moby/moby/releases/tag/docker-v29.5.2): v29.5.2

[Compare Source](https://github.com/moby/moby/compare/docker-v29.5.1...docker-v29.5.2)

##### 29.5.2

For a full list of pull requests and changes in this release, refer to the relevant GitHub milestones:

- [docker/cli, 29.5.2 milestone](https://github.com/docker/cli/issues?q=is%3Aclosed+milestone%3A29.5.2)
- [moby/moby, 29.5.2 milestone](https://github.com/moby/moby/issues?q=is%3Aclosed+milestone%3A29.5.2)

##### Bug fixes and enhancements

- Fix `docker cp` failing with "mkdirat: file exists" when a container has a bind mount whose target traverses an in-container symlink (e.g. `/var/run -> /run`). [moby/moby#52655](https://github.com/moby/moby/pull/52655)

### [`v29.5.1`](https://github.com/moby/moby/releases/tag/docker-v29.5.1): v29.5.1

[Compare Source](https://github.com/moby/moby/compare/docker-v29.5.0...docker-v29.5.1)

##### 29.5.1

For a full list of pull requests and changes in this release, refer to the relevant GitHub milestones:

- [docker/cli, 29.5.1 milestone](https://github.com/docker/cli/issues?q=is%3Aclosed+milestone%3A29.5.1)
- [moby/moby, 29.5.1 milestone](https://github.com/moby/moby/issues?q=is%3Aclosed+milestone%3A29.5.1)

##### Security

This release includes fixes for multiple security vulnerabilities affecting Docker Engine.

- **CVE-2026-41567** Fix a vulnerability in `docker cp` where archive decompression binaries (e.g. `xz`, `unpigz`) were resolved via `PATH` inside the container filesystem while running as host root, allowing a malicious container to execute arbitrary binaries with host root privileges.\
  [GHSA-x86f-5xw2-fm2r](https://github.com/moby/moby/security/advisories/GHSA-x86f-5xw2-fm2r)

- **CVE-2026-41568** Fix a TOCTOU vulnerability in `docker cp` that allowed a container process to create files or directories at arbitrary locations on the host filesystem.\
  [GHSA-vp62-88p7-qqf5](https://github.com/moby/moby/security/advisories/GHSA-vp62-88p7-qqf5)

- **CVE-2026-42306** Fix a TOCTOU vulnerability in `docker cp` that allowed a container process to redirect a bind mount to an arbitrary location on the host filesystem.\
  [GHSA-rg2x-37c3-w2rh](https://github.com/moby/moby/security/advisories/GHSA-rg2x-37c3-w2rh)

##### Networking

- Fix UDP conntrack entries not being deleted when not bound to a specific IP address. [moby/moby#52640](https://github.com/moby/moby/pull/52640)

### [`v29.5.0`](https://github.com/moby/moby/releases/tag/docker-v29.5.0): v29.5.0

[Compare Source](https://github.com/moby/moby/compare/docker-v29.4.3...docker-v29.5.0)

##### 29.5.0

For a full list of pull requests and changes in this release, refer to the relevant GitHub milestones:

- [docker/cli, 29.5.0 milestone](https://github.com/docker/cli/issues?q=is%3Aclosed+milestone%3A29.5.0)
- [moby/moby, 29.5.0 milestone](https://github.com/moby/moby/issues?q=is%3Aclosed+milestone%3A29.5.0)

> \[!NOTE]
> Rootless: `gvisor-tap-vsock` is now the new default rootless network driver and should be preferred over `slirp4netns` which is no longer installed via Docker packaging.

##### New

- Rootless: Add new default  `gvisor-tap-vsock` network driver. [moby/moby#52319](https://github.com/moby/moby/pull/52319)
- Enable private time namespace for containers by default on supported kernels. [moby/moby#52326](https://github.com/moby/moby/pull/52326)
- The `local` logging driver now has support for custom attributes, adding support for the `label`, `label-regex`, `env`, `env-regex`, and `tag` log options. [moby/moby#52348](https://github.com/moby/moby/pull/52348)
- Windows: The daemon now supports listening on a Unix socket (`-H unix://...`), with optional group-based access control via `--group`. [moby/moby#52365](https://github.com/moby/moby/pull/52365)

##### Security

- CVE-2026-32288: Fix a denial of service where pulling a maliciously crafted image could cause the daemon to allocate unbounded memory when processing sparse tar archives. [GHSA-x4jj-h2v8-hqqv](https://github.com/advisories/GHSA-x4jj-h2v8-hqqv). [moby/moby#52478](https://github.com/moby/moby/pull/52478)

##### Bug fixes and enhancements

- `docker ps --format` now supports a `.HealthStatus` placeholder to print container health state (`starting`, `healthy`, `unhealthy`) as a dedicated field. [docker/cli#6913](https://github.com/docker/cli/pull/6913)
- Add "time-namespaces" feature flag to disable time-namespaces. [moby/moby#52577](https://github.com/moby/moby/pull/52577)
- containerd integration: Fix auth token requests ignoring per-host TLS settings (custom CAs, insecure-registries). [moby/moby#52600](https://github.com/moby/moby/pull/52600)
- Daemon reload events now signify that the daemon reload has fully completed. [moby/moby#52589](https://github.com/moby/moby/pull/52589)
- Expose diagnostic data about userland proxy in `docker info`. [moby/moby#52321](https://github.com/moby/moby/pull/52321)
- Fix `docker image ls --filter reference=...` (`GET /images/json`) to also match fully qualified canonical image names (e.g. `docker.io/library/alpine`), not only the familiar short form. [moby/moby#52333](https://github.com/moby/moby/pull/52333)
- Fix a bug where leaving an autolock-enabled swarm could leave orphaned state, causing subsequent swarm init to fail with "Swarm is encrypted and needs to be unlocked". [moby/moby#52479](https://github.com/moby/moby/pull/52479)
- Fix an issue where logging errors appeared as empty strings in the daemon log instead of the message that failed to write. [moby/moby#52442](https://github.com/moby/moby/pull/52442)
- Fix incorrect SHARED SIZE and UNIQUE SIZE reporting in `docker system df -v` by including shared content blobs in size calculation. [moby/moby#52482](https://github.com/moby/moby/pull/52482)
- Fix support for CDI specifications that request additional group IDs. [moby/moby#52579](https://github.com/moby/moby/pull/52579)
- Fix volume subpath file mounts over an existing file in the image failing container creation with "not a directory". [moby/moby#52584](https://github.com/moby/moby/pull/52584)
- Sort labels in `volume`, `network`, `config`, and `secret` formatters for deterministic output. [docker/cli#6954](https://github.com/docker/cli/pull/6954)
- Swarm: Prevent corruption of Raft snapshots when swarm state is large. [moby/moby#52441](https://github.com/moby/moby/pull/52441)

##### Packaging updates

- Update BuildKit to [v0.30.0](https://github.com/moby/buildkit/releases/tag/v0.30.0). [moby/moby#52618](https://github.com/moby/moby/pull/52618)
- Update Go runtime to [1.26.3](https://go.dev/doc/devel/release#go1.26.3). [moby/moby#52572](https://github.com/moby/moby/pull/52572), [docker/cli#6967](https://github.com/docker/cli/pull/6967)

##### Networking

- Fix conntrack entries being incorrectly deleted for UDP containers sharing the same port on different IPs when one container is restarted. [moby/moby#52423](https://github.com/moby/moby/pull/52423)
- Fix stale VIP DNS records for swarm service network aliases not being removed during rolling updates. [moby/moby#52236](https://github.com/moby/moby/pull/52236)
- Fix the userland proxy silently dropping UDP datagrams when a previous write to an unavailable backend left a stale ECONNREFUSED error on the socket. [moby/moby#52483](https://github.com/moby/moby/pull/52483)
- Rootless: Properly support `--net=host` and localhost registries. [moby/moby#47103](https://github.com/moby/moby/pull/47103)

##### Rootless

- Update RootlessKit to [v3.0.0](https://github.com/rootless-containers/rootlesskit/releases/tag/v3.0.0). [moby/moby#52319](https://github.com/moby/moby/pull/52319)

##### Go SDK

- cli/config/configfile: `GetAuthConfig`, `GetCredentialsStore`: normalize hostname when resolving auth. [docker/cli#6846](https://github.com/docker/cli/pull/6846)

##### Deprecations

- cli/command/image/build: remove deprecated `DefaultDockerfileName` const. [docker/cli#6737](https://github.com/docker/cli/pull/6737)
- cli/command/image/build: remove deprecated `DetectArchiveReader` util. [docker/cli#6737](https://github.com/docker/cli/pull/6737)
- cli/command/image/build: remove deprecated `IsArchive` utility. [docker/cli#6737](https://github.com/docker/cli/pull/6737)
- cli/command/image/build: remove deprecated `ResolveAndValidateContextPath` util. [docker/cli#6737](https://github.com/docker/cli/pull/6737)
- cli/command/image/build: remove deprecated `WriteTempDockerfile` util. [docker/cli#6737](https://github.com/docker/cli/pull/6737)

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

This PR has been generated by [Mend Renovate](https://github.com/renovatebot/renovate).
<!--renovate-debug:eyJjcmVhdGVkSW5WZXIiOiI0My4xODAuMCIsInVwZGF0ZWRJblZlciI6IjQzLjIxMC4xIiwidGFyZ2V0QnJhbmNoIjoibWFpbiIsImxhYmVscyI6WyJtaW5vci11cGRhdGUiXX0=-->
