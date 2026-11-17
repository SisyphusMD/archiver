# #72 chore(deps): update dependency moby/moby to v29.9.0 - autoclosed

- Kind: pull, state: closed, by renovate, created 2026-08-10T11:01:00-07:00
- Labels: none; milestone: none
- renovate/moby-moby-29.x -> main, merged: False

This PR contains the following updates:

| Package | Update | Change |
|---|---|---|
| [moby/moby](https://github.com/moby/moby) | minor | `29.6.2` → `29.9.0` |

---

### Release Notes

<details>
<summary>moby/moby</summary>

### [`v29.9.0`](https://github.com/moby/moby/releases/tag/docker-v29.9.0): v29.9.0

[Compare Source](https://github.com/moby/moby/compare/docker-v29.8.2...docker-v29.9.0-rc.2)

##### 29.9.0

For a full list of pull requests and changes in this release, refer to the relevant GitHub milestones:

- [docker/cli, 29.9.0 milestone](https://github.com/docker/cli/issues?q=is%3Aclosed+milestone%3A29.9.0)
- [moby/moby, 29.9.0 milestone](https://github.com/moby/moby/issues?q=is%3Aclosed+milestone%3A29.9.0)

##### Security

The Go runtime update fixes the following security vulnerabilities in Docker Engine:

- **CVE-2026-97032**: An HTTP/2 client could crash the daemon by changing the HPACK header table size while sending requests. [golang/go#81867](https://go.dev/issue/81867)
- **CVE-2026-78659**: An HTTP/2 client could exhaust daemon memory by declaring a large number of fields in a `Trailer` header, bypassing the header size limits. [golang/go#81857](https://go.dev/issue/81857)
- **CVE-2026-78663**: An HTTP/2 client could bypass the connection-level flow control limit by resetting streams, making the daemon buffer more request data than the limit allows. [golang/go#81743](https://go.dev/issue/81743)
- **CVE-2026-78669**: An HTTP/2 client could cause excessive daemon CPU use by opening many streams and repeatedly changing the initial window size. [golang/go#81742](https://go.dev/issue/81742)
- **CVE-2026-56857**: On Windows, the daemon could create a directory outside its data root if someone with write access to the data root had placed a junction there. [golang/go#81739](https://go.dev/issue/81739)

The `golang.org/x/net` update to v0.60.0 applies the same HTTP/2 fixes to the deprecated `/grpc` endpoint and to the gRPC server that BuildKit runs for frontend containers, such as images referenced by a `# syntax=` directive.

##### Bug fixes and enhancements

- containerd image store: Add the `lazy-pull` daemon feature to control whether `docker pull` skips downloading layer content that the snapshotter already provides. Lazy pulls are enabled by default for known remote snapshotters (`nydus`, `overlaybd`, `soci`, `stargz`). [moby/moby#53877](https://github.com/moby/moby/pull/53877)
- containerd image store: Fix pulls skipping required layer blobs when unpacked layers already exist, leaving images runnable but incomplete for export or push. [moby/moby#53615](https://github.com/moby/moby/pull/53615)
- Fix `docker container create --name` reporting a misleading validation error mentioning invalid characters instead of invalid name length. [moby/moby#53484](https://github.com/moby/moby/pull/53484)
- Fix a connection leak to the RootlessKit API socket on every `GET /version` request in rootless mode. [moby/moby#53836](https://github.com/moby/moby/pull/53836)
- Improve Windows service registration and unregistration cleanup, including making service unregistration (`--unregister-service`) idempotent. [moby/moby#53845](https://github.com/moby/moby/pull/53845)

##### Packaging updates

- Update BuildKit to [v0.34.0](https://github.com/moby/buildkit/releases/tag/v0.34.0). [moby/moby#53882](https://github.com/moby/moby/pull/53882)
- Update Go runtime to [1.26.9](https://go.dev/doc/devel/release#go1.26.9). [docker/cli#7363](https://github.com/docker/cli/pull/7363)
- Update containerd (static binaries) to [v2.4.1](https://github.com/containerd/containerd/releases/tag/v2.4.1). [moby/moby#53773](https://github.com/moby/moby/pull/53773)

##### Networking

- Allow IPv6 Neighbour Discovery between containers on a bridge network with inter-container communication disabled, matching the existing IPv4 behaviour. [moby/moby#53723](https://github.com/moby/moby/pull/53723)
- Fix `docker ps` and `GET /containers/json` omitting published ports for networks using routed gateway mode. [moby/moby#53693](https://github.com/moby/moby/pull/53693)
- Fix a bug where a restarted daemon could be dropped from a peer's service discovery and load balancing until it rejoined the gossip cluster. [moby/moby#53688](https://github.com/moby/moby/pull/53688)
- Fix a published port being unreachable from another container on the same network when inter-container communication is disabled, including Swarm services published through the routing mesh. [moby/moby#53723](https://github.com/moby/moby/pull/53723)
- Fix an issue where errors programming the kernel to encrypt the overlay network data-plane could in some circumstances lead to encrypted-overlay-network traffic to some nodes being transmitted in cleartext. As the receiving peer would drop cleartext packets for encrypted overlay networks as spoofed, the loss of confidentiality is limited to unidirectional flows (e.g. UDP DNS queries) and handshake attempts that never proceed (e.g. TCP SYN). [moby/moby#53420](https://github.com/moby/moby/pull/53420)
- Fix connecting live-restored containers with an implicit `host-gateway` mapping to additional networks. [moby/moby#53093](https://github.com/moby/moby/pull/53093)
- Fix overlay peers becoming unreachable after a node rejoins the cluster or a service is redeployed, when the VXLAN device had already learned a dynamic FDB entry for the peer. [moby/moby#53663](https://github.com/moby/moby/pull/53663)
- Fix Swarm tasks on overlay networks being rejected when the daemon can't write to `/var/lib/docker`. [moby/moby#53848](https://github.com/moby/moby/pull/53848)
- Published Swarm-service ports are accessible at the host's IPv6 addresses when the userland proxy is enabled. A change introduced in v29.8.0 incidentally enabled this functionality; it is a tested and supported feature as of v29.9.0. [moby/moby#53727](https://github.com/moby/moby/pull/53727)
- Release a node's IPsec security associations and policies when the last container leaves an encrypted overlay network, instead of leaking them until the daemon restarts. [moby/moby#53420](https://github.com/moby/moby/pull/53420)
- Restore the logic to remove the empty `DOCKER-INGRESS` iptables chain, and the `FORWARD` rule that jumps to it, left behind by Docker Engine 28.0.0 and earlier. [moby/moby#53825](https://github.com/moby/moby/pull/53825)

##### Rootless

- Update RootlessKit (3.2.0). [moby/moby#53607](https://github.com/moby/moby/pull/53607)

### [`v29.8.2`](https://github.com/moby/moby/releases/tag/docker-v29.8.2): v29.8.2

[Compare Source](https://github.com/moby/moby/compare/docker-v29.8.1...docker-v29.8.2)

##### 29.8.2

For a full list of pull requests and changes in this release, refer to the relevant GitHub milestones:

- [docker/cli, 29.8.2 milestone](https://github.com/docker/cli/issues?q=is%3Aclosed+milestone%3A29.8.2)
- [moby/moby, 29.8.2 milestone](https://github.com/moby/moby/issues?q=is%3Aclosed+milestone%3A29.8.2)

##### Security

This release fixes the following security vulnerabilities in Docker Engine:

- **CVE-2026-53493**: Pulling a crafted OCI image index with deeply nested or widely fanned-out descriptors could cause unbounded CPU and memory use. [GHSA-pg57-6jwg-q645](https://github.com/containerd/containerd/security/advisories/GHSA-pg57-6jwg-q645)
- **CVE-2026-92543**: A malicious DNS response could make registry connections skip TLS certificate verification or fall back to HTTP, exposing registry credentials or allowing image substitution. [GHSA-7cfq-22r6-qp73](https://github.com/moby/moby/security/advisories/GHSA-7cfq-22r6-qp73)
- **CVE-2026-92542**: Unprivileged users on a Swarm node could inject forged Ethernet frames into encrypted overlay networks on peer nodes. [GHSA-6m9p-4h64-m6vh](https://github.com/moby/moby/security/advisories/GHSA-6m9p-4h64-m6vh)

The BuildKit update fixes the following security vulnerabilities:

- **CVE-2026-93315**: A build step could redirect proxy CA cleanup outside the build root filesystem, block it with a special file, or let the build succeed when cleanup failed. [GHSA-2f5p-x9ph-g97x](https://github.com/moby/buildkit/security/advisories/GHSA-2f5p-x9ph-g97x)
- **CVE-2026-93316**: A build that requested CDI devices could cause a daemon panic when CDI support was disabled, for example with `"features": {"cdi": false}` in `daemon.json`. [GHSA-r456-g3gm-cvxf](https://github.com/moby/buildkit/security/advisories/GHSA-r456-g3gm-cvxf)
- **CVE-2026-93317**: With the containerd image store, a client using the low-level LLB API could poison the build cache with container blob contents that did not match their claimed digest. [GHSA-p3rc-w3hc-pqvv](https://github.com/moby/buildkit/security/advisories/GHSA-p3rc-w3hc-pqvv)
- **CVE-2026-93318**: A malicious image could poison the build cache with layer DiffIDs that did not match the actual layer contents. [GHSA-f2v9-hprr-32q3](https://github.com/moby/buildkit/security/advisories/GHSA-f2v9-hprr-32q3)
- **CVE-2026-93319**: A malicious external frontend could crash the daemon through gateway container lifecycle races or malformed requests and definitions. [GHSA-4hgw-qrhw-fhg8](https://github.com/moby/buildkit/security/advisories/GHSA-4hgw-qrhw-fhg8)
- **CVE-2026-93320**: Daemon-side snapshot reads and LLB `mkfile` operations did not safely handle special files. [GHSA-9728-qjrv-2xh2](https://github.com/moby/buildkit/security/advisories/GHSA-9728-qjrv-2xh2)
- **CVE-2026-93321**: A malformed LLB file operation with invalid symlink owner inputs could crash the daemon. [GHSA-fjj4-h6vf-m9hj](https://github.com/moby/buildkit/security/advisories/GHSA-fjj4-h6vf-m9hj)
- **CVE-2026-93322**: A malformed LLB merge operation with mismatched input counts could crash the daemon. [GHSA-cv6p-7w7g-xjwq](https://github.com/moby/buildkit/security/advisories/GHSA-cv6p-7w7g-xjwq)
- **CVE-2026-93323**: An oversized Dockerfile, `.dockerignore`, gateway file, or nested LLB definition could exhaust daemon memory. [GHSA-mgqf-486f-49vp](https://github.com/moby/buildkit/security/advisories/GHSA-mgqf-486f-49vp)
- **CVE-2026-93326**: A crafted Git build source could bypass source policy rules that match on the repository URL, through a Git bundle locator or a full remote URL that did not match the source identifier. [GHSA-66hf-6vf5-87hc](https://github.com/moby/buildkit/security/advisories/GHSA-66hf-6vf5-87hc)

##### Bug fixes and enhancements

- Fix `docker cp` failing on a container with a bind-mounted socket nested inside another bind mount. [moby/moby#53724](https://github.com/moby/moby/pull/53724)
- Fix `docker info` failing with an “invalid Prefix” error after reloading a daemon with custom default address pools. [moby/moby#53812](https://github.com/moby/moby/pull/53812)

##### Packaging updates

- Update BuildKit to [v0.33.1](https://github.com/moby/buildkit/releases/tag/v0.33.1). [moby/moby#53823](https://github.com/moby/moby/pull/53823)
- Update containerd (static binaries) to [v2.3.6](https://github.com/containerd/containerd/releases/tag/v2.3.6). [moby/moby#53778](https://github.com/moby/moby/pull/53778)
- Update runc (in static binaries) to [v1.5.2](https://github.com/opencontainers/runc/releases/tag/v1.5.2). [moby/moby#53811](https://github.com/moby/moby/pull/53811)

### [`v29.8.1`](https://github.com/moby/moby/releases/tag/docker-v29.8.1): v29.8.1

[Compare Source](https://github.com/moby/moby/compare/docker-v29.8.0-rc.1...docker-v29.8.1)

##### 29.8.1

For a full list of pull requests and changes in this release, refer to the relevant GitHub milestones:

- [docker/cli, 29.8.1 milestone](https://github.com/docker/cli/issues?q=is%3Aclosed+milestone%3A29.8.1)
- [moby/moby, 29.8.1 milestone](https://github.com/moby/moby/issues?q=is%3Aclosed+milestone%3A29.8.1)

##### Bug fixes and enhancements

- containerd image store: Fix `docker load` leaving dangling images after loading an image that already exists. [moby/moby#53595](https://github.com/moby/moby/pull/53595)
- Fix managed containerd logging a CRI pod-sandbox plugin dependency warning during startup. [moby/moby#53585](https://github.com/moby/moby/pull/53585)
- Fix user-namespace detection on OpenVZ, where namespace inode numbers are virtualized. [moby/moby#53605](https://github.com/moby/moby/pull/53605)
- Windows: Preserve hard links when committing a container, so a file with multiple names is stored once in the resulting image layer instead of once per name. [moby/moby#53624](https://github.com/moby/moby/pull/53624)

##### Packaging updates

- Update containerd (static binaries) to [v2.3.5](https://github.com/containerd/containerd/releases/tag/v2.3.5). [moby/moby#53589](https://github.com/moby/moby/pull/53589)

##### Networking

- Fix `GET /networks` returning a 500 status if an invalid 'type' filter was provided. [moby/moby#53678](https://github.com/moby/moby/pull/53678)

### [`v29.8.0`](https://github.com/moby/moby/releases/tag/docker-v29.8.0): v29.8.0

[Compare Source](https://github.com/moby/moby/compare/docker-v29.7.2...docker-v29.8.0-rc.1)

#### 29.8.0

For a full list of pull requests and changes in this release, refer to the relevant GitHub milestones:

- [docker/cli, 29.8.0 milestone](https://github.com/docker/cli/issues?q=is%3Aclosed+milestone%3A29.8.0)
- [moby/moby, 29.8.0 milestone](https://github.com/moby/moby/issues?q=is%3Aclosed+milestone%3A29.8.0)

##### New

- Add `HostConfig.Umask` option and a corresponding `--umask <octal>` flag to `docker create`/`docker run` to set the umask for a container's main process, execs, and healthchecks. [moby/moby#53463](https://github.com/moby/moby/pull/53463), [docker/cli#7108](https://github.com/docker/cli/pull/7108)
- Add support for attaching service names, environments, and custom CloudWatch entity attributes to logs from the `awslogs` logging driver. [moby/moby#52632](https://github.com/moby/moby/pull/52632)

##### Security

- Add daemon support for configuring the default container AppArmor profile template. [moby/moby#52771](https://github.com/moby/moby/pull/52771)
  - See <https://docker.docker.com/engine/security/apparmor/#customize-the-default-profile>
- Prevent containers from using the 32-bit `socketcall(2)` path to create `AF_VSOCK` sockets and communicate with host virtual machines by adding AppArmor and SELinux policy rules. [moby/moby#53551](https://github.com/moby/moby/pull/53551)

##### Networking

- Fix `docker network inspect` failing to find a healthy Swarm network when another Swarm network could not be allocated. [moby/moby#53325](https://github.com/moby/moby/pull/53325)
- Fix a node gossiping a superseded value for a Swarm service discovery entry after concurrent updates to the same key. [moby/moby#53479](https://github.com/moby/moby/pull/53479)
- Fix Swarm service names failing to resolve on a node indefinitely after it misses a network membership announcement. [moby/moby#53437](https://github.com/moby/moby/pull/53437)
- Fix Swarm service names failing to resolve on healthy nodes after a transient node failure. [moby/moby#53142](https://github.com/moby/moby/pull/53142)
- Prevent dockerd from hanging when the nft command produces enough stderr output to fill its pipe. [moby/moby#53517](https://github.com/moby/moby/pull/53517)
- Reduce gossip traffic generated by a node that repeatedly disconnects and rejoins the cluster. [moby/moby#53479](https://github.com/moby/moby/pull/53479)
- Remote network-driver plugins can now set the container-side interface name via the `DstName` field in their `Join` response. [moby/moby#52866](https://github.com/moby/moby/pull/52866)
- Reserve network names "container" and "container:" to prevent creation of unusable networks. [moby/moby#51973](https://github.com/moby/moby/pull/51973)
- Spread the daemon's periodic Swarm overlay network gossip and synchronization work over time, avoiding recurring bursts of CPU and network usage. [moby/moby#53475](https://github.com/moby/moby/pull/53475)
- Swarm service-mesh published ports now use the same infrastructure as published ports for local containers. [moby/moby#53118](https://github.com/moby/moby/pull/53118)

##### Rootless

- Fix `--disable-host-loopback` not being enforced for the `pasta` network driver in rootless mode. [moby/moby#53358](https://github.com/moby/moby/pull/53358)
- Update RootlessKit to v3.1.0, adding support for the `pesto` port driver in rootless mode. Set `DOCKERD_ROOTLESS_ROOTLESSKIT_PORT_DRIVER=pesto` to use it; it requires the `pasta` network driver and supports IPv4 only. [moby/moby#53358](https://github.com/moby/moby/pull/53358)

##### Go SDK

- Update minimum supported Go version to 1.26. [docker/cli#7258](https://github.com/docker/cli/pull/7258)

##### Bug fixes and enhancements

- Add `annotation` filter to container listings (`docker ps`, `GET /containers/json`) allowing to filter containers by their annotations. [moby/moby#53538](https://github.com/moby/moby/pull/53538)
- containerd image store: Fix `docker image inspect` reporting a smaller image size than `docker image ls`. [moby/moby#53426](https://github.com/moby/moby/pull/53426)
- containerd image store: Fix slower image pulls caused by repeated registry authentication within a single pull. [moby/moby#53497](https://github.com/moby/moby/pull/53497)
- Do not log expected image signature identity misses as errors for containerd image store images. [moby/moby#53495](https://github.com/moby/moby/pull/53495)
- dockerd now uses the embedded containerd if no system containerd service is configured and containerd is not installed. [moby/moby#53388](https://github.com/moby/moby/pull/53388)
- Fix `GET /images/{name}/json` not including unpacked snapshot usage in `Size` when using the containerd image store. [moby/moby#53426](https://github.com/moby/moby/pull/53426)
- Fix classic-builder cache for Dockerfile stages that select a non-host platform with `FROM --platform`. [moby/moby#53503](https://github.com/moby/moby/pull/53503)
- Fix CLI panic when `DOCKER_HOST` or `-H` specifies an invalid host. [docker/cli#7280](https://github.com/docker/cli/pull/7280)
- Fix health checks being delayed for too long when the start interval is longer than the start period. [moby/moby#52317](https://github.com/moby/moby/pull/52317)
- Fix inconsistent mount ordering in `docker inspect` output (`GET /containers/{id}/json`) and container listings (`docker ps`, `GET /containers/json`). [moby/moby#53534](https://github.com/moby/moby/pull/53534)
- Fix NRI container metadata so `Container.Args` includes the resolved executable as `argv[0]`, matching the process launched in the container instead of only the Docker `Cmd`. [moby/moby#53423](https://github.com/moby/moby/pull/53423)
- Fix Swarm service creation failing when an automatically generated name is already in use. [moby/moby#53468](https://github.com/moby/moby/pull/53468)
- Fix the container root directory `/` being world-writable when using the `btrfs` storage driver. [moby/moby#53500](https://github.com/moby/moby/pull/53500)
- Fixed `docker ps` sorting published ports lexicographically instead of numerically. [docker/cli#7144](https://github.com/docker/cli/pull/7144)
- Preserve service mount order during forced updates to avoid an unnecessary rollout on the next stack deploy. [docker/cli#7227](https://github.com/docker/cli/pull/7227)
- Prevent containerd's v2 CRI plugins from loading when CRI is disabled. [moby/moby#53564](https://github.com/moby/moby/pull/53564)
- Print plugin hook output (e.g. the "What's next:" hint) after the command's error message instead of before it. [docker/cli#6976](https://github.com/docker/cli/pull/6976)
- Reject checkpoint IDs containing path separators to prevent access outside the container checkpoint directory. [moby/moby#53377](https://github.com/moby/moby/pull/53377)

##### Packaging updates

- Update BuildKit to [v0.33.0](https://github.com/moby/buildkit/releases/tag/v0.33.0). [moby/moby#53554](https://github.com/moby/moby/pull/53554)
- Update containerd (static binaries) to [v2.3.4](https://github.com/containerd/containerd/releases/tag/v2.3.4). [moby/moby#53409](https://github.com/moby/moby/pull/53409)
- Update Go runtime to [1.26.8](https://go.dev/doc/devel/release#go1.26.8). [moby/moby#53550](https://github.com/moby/moby/pull/53550), [docker/cli#7274](https://github.com/docker/cli/pull/7274)
- Update runc (in static binaries) to [v1.5.1](https://github.com/opencontainers/runc/releases/tag/v1.5.1). [moby/moby#52306](https://github.com/moby/moby/pull/52306)

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

[Compare Source](https://github.com/moby/moby/compare/docker-v29.7.0-rc.1...docker-v29.7.1)

##### 29.7.1

For a full list of pull requests and changes in this release, refer to the relevant GitHub milestones:

- [docker/cli, 29.7.1 milestone](https://github.com/docker/cli/issues?q=is%3Aclosed+milestone%3A29.7.1)
- [moby/moby, 29.7.1 milestone](https://github.com/moby/moby/issues?q=is%3Aclosed+milestone%3A29.7.1)

##### Bug fixes and enhancements

- Fix a regression that prevented pulling images whose layers contain directories without explicit parent directory entries. [moby/moby#53260](https://github.com/moby/moby/pull/53260)
- Fix a regression where the `CopyToContainer` rejects container paths that traverse absolute symlinks, such as `/var/run` -> `/run`. [moby/moby#53261](https://github.com/moby/moby/pull/53261)

### [`v29.7.0`](https://github.com/moby/moby/releases/tag/docker-v29.7.0): v29.7.0

[Compare Source](https://github.com/moby/moby/compare/docker-v29.6.2...docker-v29.7.0-rc.1)

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
<!--renovate-debug:eyJjcmVhdGVkSW5WZXIiOiI0NC4yMS4wIiwidXBkYXRlZEluVmVyIjoiNDQuMTQ2LjMiLCJ0YXJnZXRCcmFuY2giOiJtYWluIiwibGFiZWxzIjpbIm1pbm9yLXVwZGF0ZSJdfQ==-->
