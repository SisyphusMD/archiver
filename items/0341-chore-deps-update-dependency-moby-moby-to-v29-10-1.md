# #341 chore(deps): update dependency moby/moby to v29.10.1

- Kind: pull, state: open, by renovate, created 2026-11-16T05:02:10-08:00
- Labels: none; milestone: none
- renovate/moby-moby-29.x -> main, merged: False

This PR contains the following updates:

| Package | Update | Change |
|---|---|---|
| [moby/moby](https://github.com/moby/moby) | minor | `29.9.3` → `29.10.1` |

---

### Release Notes

<details>
<summary>moby/moby</summary>

### [`v29.10.1`](https://github.com/moby/moby/releases/tag/docker-v29.10.1): v29.10.1

[Compare Source](https://github.com/moby/moby/compare/docker-v29.10.0-rc.2...docker-v29.10.1)

#### 29.10.1

For a full list of pull requests and changes in this release, refer to the relevant GitHub milestones:

- [docker/cli, 29.10.1 milestone](https://github.com/docker/cli/issues?q=is%3Aclosed+milestone%3A29.10.1)
- [moby/moby, 29.10.1 milestone](https://github.com/moby/moby/issues?q=is%3Aclosed+milestone%3A29.10.1)

##### Security

This release includes fixes for multiple security vulnerabilities affecting Docker Engine and related components.

- **CVE-2026-135844** Fix unbounded reads when parsing user, group, and ID mapping files\
  [GHSA-ffh6-8jcg-cjmw](https://github.com/moby/sys/security/advisories/GHSA-ffh6-8jcg-cjmw)
- **CVE-2026-135124** A process in a running container could make a read-only bind mount writable while `docker cp` copied files into the container, letting the copied files modify the host directory behind that mount\
  [GHSA-vfmq-r373-x89f](https://github.com/moby/moby/security/advisories/GHSA-vfmq-r373-x89f)

##### Bug fixes and enhancements

- Fix image builds failing with "http: invalid Read on closed Body" when uploading a buid context with `Expect: 100-continue` header. [moby/moby#54228](https://github.com/moby/moby/pull/54228)
- Fix the default AppArmor profile blocking processes in the same container from signalling or tracing each other on hosts with `kernel.apparmor_restrict_unprivileged_unconfined=1`, such as recent Ubuntu releases. [moby/profiles#58](https://github.com/moby/profiles/pull/58). [moby/moby#54271](https://github.com/moby/moby/pull/54271)
- Update `docker login` to mention OATs as an additional authentication mechanism. [docker/cli#7539](https://github.com/docker/cli/pull/7539)
- Update BuildKit to [v0.35.1](https://github.com/moby/buildkit/releases/tag/v0.35.1). [moby/moby#54231](https://github.com/moby/moby/pull/54231)
- Update the Windows docker.exe icon to the new Docker logo. [docker/cli#7534](https://github.com/docker/cli/pull/7534)
- Update the Windows dockerd.exe icon to the new Docker logo. [moby/moby#54226](https://github.com/moby/moby/pull/54226)

##### Networking

- Fix `docker network rm` failing with "has active endpoints" for endpoints that no longer exist, after containers on the network were restarted or disconnected concurrently with other network operations. [moby/moby#54281](https://github.com/moby/moby/pull/54281)
- Fix a race between connecting a container to a Swarm network and removing its endpoint that could bring back an endpoint whose IP addresses were already released. [moby/moby#54282](https://github.com/moby/moby/pull/54282)

### [`v29.10.0`](https://github.com/moby/moby/releases/tag/docker-v29.10.0): v29.10.0

[Compare Source](https://github.com/moby/moby/compare/docker-v29.9.3...docker-v29.10.0-rc.2)

#### 29.10.0

For a full list of pull requests and changes in this release, refer to the relevant GitHub milestones:

- [docker/cli, 29.10.0 milestone](https://github.com/docker/cli/issues?q=is%3Aclosed+milestone%3A29.10.0)
- [moby/moby, 29.10.0 milestone](https://github.com/moby/moby/issues?q=is%3Aclosed+milestone%3A29.10.0)

##### ⚠️ Potential breaking change: local registry connections

If you explicitly configure `insecure-registries` in `daemon.json` or use `--insecure-registry`, Docker Engine no longer automatically treats loopback addresses (`127.0.0.0/8` and `::1/128`) as insecure registries.

> \[!WARNING]
> After upgrading, pulls and pushes to local registries that use HTTP or untrusted TLS certificates may fail unless those registries are explicitly included in your insecure registry configuration.
> Configure TLS with a trusted certificate, or add only the required local registry addresses, such as `localhost:5000`, to your existing insecure registry configuration.

When `insecure-registries` is omitted, loopback registries remain insecure by default.
Setting `"insecure-registries": []` in `daemon.json` disables all default insecure registries. [moby/moby#53868](https://github.com/moby/moby/pull/53868)

##### 🧻 Engine API specification moves to OpenAPI 3.2

The Engine API specification now uses OpenAPI 3.2 as its source format, in [`api/openapi.yaml`](https://github.com/moby/moby/blob/api/v1.57.0/api/openapi.yaml).
A Swagger 2.0 specification generated from it is still published for existing tooling, in [`api/swagger.yaml`](https://github.com/moby/moby/blob/api/v1.57.0/api/swagger.yaml).
For the transition, this release also includes a Swagger 2.0 copy pinned to API v1.57, in [`api/docs/v1.57-swagger.yaml`](https://github.com/moby/moby/blob/api/v1.57.0/api/docs/v1.57-swagger.yaml).
Future API versions won't get a per-version Swagger 2.0 copy, so update tooling that depends on these files to use the OpenAPI 3.2 specifications. [moby/moby#53630](https://github.com/moby/moby/pull/53630)

##### 📦 Packages for AlmaLinux, Oracle Linux, and Rocky Linux

Docker Engine packages are available for AlmaLinux, Oracle Linux, and Rocky Linux.
For installation instructions, see [AlmaLinux](/manuals/engine/install/almalinux.md), [Oracle Linux](/manuals/engine/install/oraclelinux.md), and [Rocky Linux](/manuals/engine/install/rockylinux.md).

***

##### New

- Add experimental support for jobs: run one-shot and cron-scheduled workloads natively managed by the daemon, as an opt-in built-in extension enabled with `--enable-extension org.mobyproject.jobs.v0`. [moby/moby#53485](https://github.com/moby/moby/pull/53485)
- Add support for customizing Docker’s embedded containerd using configuration files in `/etc/docker/containerd.d/`. [moby/moby#53946](https://github.com/moby/moby/pull/53946)
- Introduce `restarting` attribute for  `die` events to let consumer know container is going to restart. [moby/moby#50908](https://github.com/moby/moby/pull/50908)

##### Bug fixes and enhancements

- Block obsolete socket families in the default AppArmor and seccomp profiles. [moby/moby#54093](https://github.com/moby/moby/pull/54093)
- containerd image store: Fix `docker image inspect` and `docker image ls` reporting an empty image when the host platform of a multi-platform image is not pulled but another platform is. [moby/moby#54040](https://github.com/moby/moby/pull/54040)
- Don't wait for the telemetry export timeout when the OTLP endpoint refuses connections. [docker/cli#7495](https://github.com/docker/cli/pull/7495)
- Fix `docker <unknown-command> --version` printing the CLI version instead of an "unknown command" error. [docker/cli#7408](https://github.com/docker/cli/pull/7408)
- Fix `docker cp` printing a "Successfully copied" message when the copy fails. [docker/cli#7515](https://github.com/docker/cli/pull/7515)
- Fix `docker manifest annotate` rejecting valid platforms not included in its predefined list of OS/architecture combinations, such as `linux/loong64`. [docker/cli#7448](https://github.com/docker/cli/pull/7448)
- Fix `docker ps` printing an empty `NAMES` column when all of a container's names contain a slash, such as when connected to a classic Swarm API. [docker/cli#7481](https://github.com/docker/cli/pull/7481)
- Fix the daemon deleting the docker0 interface on restart when it is configured with a user-managed bridge and default-address-pools. [moby/moby#54157](https://github.com/moby/moby/pull/54157)
- Fix Windows layer exports containing NTFS sparse files. [moby/moby#53745](https://github.com/moby/moby/pull/53745)
- Fix: Include HTTP status codes in JSON streaming errors to allow clients to correctly classify failures. [moby/moby#54133](https://github.com/moby/moby/pull/54133)
- Improve image selection on ARM64 Linux hosts to prefer the highest image variant supported by the host CPU. [moby/moby#53910](https://github.com/moby/moby/pull/53910)
- manifest annotate: add shell-completion for `--os` and `--arch`. [docker/cli#7449](https://github.com/docker/cli/pull/7449)
- Reject duplicate GPU counts consistently when shorthand and explicit count options are mixed. [docker/cli#7312](https://github.com/docker/cli/pull/7312)
- Stop automatically treating loopback registries as insecure when `insecure-registries` is explicitly configured, and allow disabling the default insecure registries by setting `"insecure-registries": []` in `daemon.json`. [moby/moby#53868](https://github.com/moby/moby/pull/53868)
- Update runc (in static binaries) to [v1.5.3](https://github.com/opencontainers/runc/releases/tag/v1.5.3). [moby/moby#54183](https://github.com/moby/moby/pull/54183)

##### Packaging updates

- Update BuildKit to [v0.35.0](https://github.com/moby/buildkit/releases/tag/v0.35.0). [moby/moby#54180](https://github.com/moby/moby/pull/54180)
- Update Go runtime to [1.27.4](https://go.dev/doc/devel/release#go1.27.4). [moby/moby#54176](https://github.com/moby/moby/pull/54176), [docker/cli#7506](https://github.com/docker/cli/pull/7506)

##### Networking

- Engines using the nftables firewall backend can now join a Swarm. (Experimental). [moby/moby#53125](https://github.com/moby/moby/pull/53125)
- Fix `docker network ls --filter type=builtin --filter type=custom` listing a random subset of networks. [moby/moby#54127](https://github.com/moby/moby/pull/54127)
- Fix a "rule exists" error when publishing more than one host port for the same container port with the nftables firewall backend and the userland proxy disabled. [moby/moby#54031](https://github.com/moby/moby/pull/54031)
- Fix a container starting with no networking after an earlier start failed with a network error, such as a published port already being in use. [moby/moby#52480](https://github.com/moby/moby/pull/52480)
- Fix DNS resolution on user-defined networks when the host's /etc/resolv.conf is missing or contains no valid nameservers. [moby/moby#54140](https://github.com/moby/moby/pull/54140)
- Fix iptables rules and an empty `DOCKER` chain left behind when the daemon switches to the nftables firewall backend after a release older than 28.0.1 ran with iptables. [moby/moby#54129](https://github.com/moby/moby/pull/54129)
- Fix iptables rules not being restored after firewalld is stopped, or started after the daemon. [moby/moby#54012](https://github.com/moby/moby/pull/54012)
- Fix overlay network driver errors when a container gets the IP address of a container that was just removed on another node. [moby/moby#53833](https://github.com/moby/moby/pull/53833)
- Fix overlay networking failing to reconverge after a network partition when a Swarm manager has been replaced at a new address. [moby/moby#53715](https://github.com/moby/moby/pull/53715)
- Fix swarm services leaking a virtual IP address on their other networks each time allocation fails on one network, which could exhaust the subnets of those networks. [moby/moby#54148](https://github.com/moby/moby/pull/54148)
- Fix Swarm tasks failing to reach a service's published port via their own node's address when a host firewall, such as ufw, drops unsolicited incoming traffic. [moby/moby#54006](https://github.com/moby/moby/pull/54006)
- Fix the MTU of overlay network interfaces being 20 bytes too large when the Swarm data path is IPv6. [moby/moby#54067](https://github.com/moby/moby/pull/54067)
- Reduce nftables port-publishing overhead by batching firewall updates per endpoint and address family. [moby/moby#54011](https://github.com/moby/moby/pull/54011)
- Stop nodes in a Swarm cluster from exchanging full overlay network state with every manager once a minute when the cluster is healthy. [moby/moby#53715](https://github.com/moby/moby/pull/53715)

##### Rootless

- Allow the overlay2 storage driver in rootless mode when SELinux is enabled. [moby/moby#54048](https://github.com/moby/moby/pull/54048)
- Fix `docker cp` failing with "remount-ro ... operation not permitted" in rootless mode on containers with a read-only bind mount from a `nosuid` or `nodev` filesystem. [moby/moby#53981](https://github.com/moby/moby/pull/53981)

##### API

- Update API version to 1.57. [moby/moby#53972](https://github.com/moby/moby/pull/53972)

</details>

---

### Configuration

📅 **Schedule**: (UTC)

- Branch creation
  - At any time (no schedule defined)
- Automerge
  - At any time (no schedule defined)

🚦 **Automerge**: Disabled by config. Please merge this manually once you are satisfied.

♻ **Rebasing**: Whenever PR is behind base branch, or you tick the rebase/retry checkbox.

🔕 **Ignore**: Close this PR and you won't be reminded about this update again.

---

 - [ ] <!-- rebase-check -->If you want to rebase/retry this PR, check this box

---

Refs #349: standing issue for dependency updates.
<!--renovate-debug:eyJjcmVhdGVkSW5WZXIiOiI0NC4yNDQuMSIsInVwZGF0ZWRJblZlciI6IjQ0LjI2MC4zIiwidGFyZ2V0QnJhbmNoIjoibWFpbiIsImxhYmVscyI6WyJtaW5vci11cGRhdGUiXX0=-->


## forgejo-actions, 2026-11-17T15:02:37-08:00

<!-- ai-review -->
**AI review** of `6757db056b`: The dependency bump is appropriate, but the Docker CLI checksums were not updated for 29.10.1, so the image build will fail during archive verification.

### Open

- **F1** (**blocking**, `Dockerfile:159`) SHA256 checksums not updated for new version: The version is bumped to 29.10.1 but DOCKER_CLI_SHA256_AMD64 (line 159) and DOCKER_CLI_SHA256_ARM64 (line 160) still hold the digests of the 29.9.3 archives. The Dockerfile comment says these are 'the digests of the reviewed archives', so the build's checksum verification will reject the 29.10.1 downloads and the image build will fail. Update both ARG values to the SHA256 digests of the 29.10.1 docker-cli archives (amd64 and arm64) before merging.
  - Reviewer: No reply from the author and the diff is unchanged. The checksums must match the new archives or the build fails at the verification step.


Disagree? Reply on this PR with your reasoning; the reviewer re-reads every reply.
<!-- ai-review-state W3siaWQiOiAiRjEiLCAiZmlsZSI6ICJEb2NrZXJmaWxlIiwgImxpbmUiOiAxNTksICJzZXZlcml0eSI6ICJibG9ja2luZyIsICJzdGF0dXMiOiAib3BlbiIsICJ0aXRsZSI6ICJTSEEyNTYgY2hlY2tzdW1zIG5vdCB1cGRhdGVkIGZvciBuZXcgdmVyc2lvbiIsICJkZXRhaWwiOiAiVGhlIHZlcnNpb24gaXMgYnVtcGVkIHRvIDI5LjEwLjEgYnV0IERPQ0tFUl9DTElfU0hBMjU2X0FNRDY0IChsaW5lIDE1OSkgYW5kIERPQ0tFUl9DTElfU0hBMjU2X0FSTTY0IChsaW5lIDE2MCkgc3RpbGwgaG9sZCB0aGUgZGlnZXN0cyBvZiB0aGUgMjkuOS4zIGFyY2hpdmVzLiBUaGUgRG9ja2VyZmlsZSBjb21tZW50IHNheXMgdGhlc2UgYXJlICd0aGUgZGlnZXN0cyBvZiB0aGUgcmV2aWV3ZWQgYXJjaGl2ZXMnLCBzbyB0aGUgYnVpbGQncyBjaGVja3N1bSB2ZXJpZmljYXRpb24gd2lsbCByZWplY3QgdGhlIDI5LjEwLjEgZG93bmxvYWRzIGFuZCB0aGUgaW1hZ2UgYnVpbGQgd2lsbCBmYWlsLiBVcGRhdGUgYm90aCBBUkcgdmFsdWVzIHRvIHRoZSBTSEEyNTYgZGlnZXN0cyBvZiB0aGUgMjkuMTAuMSBkb2NrZXItY2xpIGFyY2hpdmVzIChhbWQ2NCBhbmQgYXJtNjQpIGJlZm9yZSBtZXJnaW5nLiIsICJyZXNwb25zZSI6ICJObyByZXBseSBmcm9tIHRoZSBhdXRob3IgYW5kIHRoZSBkaWZmIGlzIHVuY2hhbmdlZC4gVGhlIGNoZWNrc3VtcyBtdXN0IG1hdGNoIHRoZSBuZXcgYXJjaGl2ZXMgb3IgdGhlIGJ1aWxkIGZhaWxzIGF0IHRoZSB2ZXJpZmljYXRpb24gc3RlcC4ifV0= -->
