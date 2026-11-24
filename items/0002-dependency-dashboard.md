# #2 Dependency Dashboard

- Kind: issue, state: open, by cody, created 2026-05-05T16:30:21-07:00
- Labels: none; milestone: none

This issue lists Renovate updates and detected dependencies. Read the [Dependency Dashboard](https://docs.renovatebot.com/key-concepts/dashboard/) docs to learn more.

## Open

The following updates have all been created. To force a retry/rebase of any, click on a checkbox below.

 - [ ] <!-- rebase-branch=renovate/debian-2026.11.x -->[chore(deps): update debian docker tag to trixie-20261123-slim](pulls/386)
 - [ ] <!-- rebase-branch=renovate/gilbertchen-duplicacy-3.3.x -->[chore(deps): update dependency gilbertchen/duplicacy to v3.3.1](pulls/346)
 - [ ] <!-- rebase-branch=renovate/moby-moby-29.x -->[chore(deps): update dependency moby/moby to v29.10.0](pulls/341)
 - [ ] <!-- rebase-branch=renovate/docker-29.x -->[chore(deps): update docker docker tag to v29.10.0](pulls/343)
 - [ ] <!-- rebase-branch=renovate/quay.io-podman-stable-6.x -->[chore(deps): update quay.io/podman/stable docker tag to v6.2.0](pulls/362)
 - [ ] <!-- rebase-branch=renovate/quay.io-skopeo-stable-1.x -->[chore(deps): update quay.io/skopeo/stable docker tag to v1.25.1](pulls/363)
 - [ ] <!-- rebase-all-open-prs -->**Click on this checkbox to rebase all open PRs at once**

## Detected Dependencies

<details><summary>dockerfile (3)</summary>
<blockquote>

<details><summary>Dockerfile (4)</summary>

 - `golang 1.27.4-trixie@sha256:f18c3d73a184fb0582828510722e723b63a93f7188d11dd223d9be6f6880e65d`
 - `golang 1.27.4-trixie@sha256:f18c3d73a184fb0582828510722e723b63a93f7188d11dd223d9be6f6880e65d`
 - `debian trixie-20261104-slim@sha256:a72b4433de1cf7d6bae0c4cee74d0547f58601a643b3375f070c51600d63b6b2` → [Updates: `trixie-20261123-slim`]
 - `debian trixie-20261104-slim@sha256:a72b4433de1cf7d6bae0c4cee74d0547f58601a643b3375f070c51600d63b6b2` → [Updates: `trixie-20261123-slim`]

</details>

<details><summary>tests/e2e/Dockerfile (2)</summary>

 - `docker 29.9.3-cli@sha256:5a7af0dcb0e46c803efe9b45371dd21cb500e5b4453250394ac6f06d49b6e6f1` → [Updates: `29.10.0-cli`]
 - `golang 1.27.4-trixie@sha256:f18c3d73a184fb0582828510722e723b63a93f7188d11dd223d9be6f6880e65d`

</details>

<details><summary>tests/fakes/Dockerfile (2)</summary>

 - `golang 1.27.4-trixie@sha256:f18c3d73a184fb0582828510722e723b63a93f7188d11dd223d9be6f6880e65d`
 - `debian trixie-20261104-slim@sha256:a72b4433de1cf7d6bae0c4cee74d0547f58601a643b3375f070c51600d63b6b2` → [Updates: `trixie-20261123-slim`]

</details>

</blockquote>
</details>

<details><summary>github-actions (4)</summary>
<blockquote>

<details><summary>.forgejo/workflows/docker-publish.yml (34)</summary>

 - `actions/checkout v7.0.1@3d3c42e5aac5ba805825da76410c181273ba90b1`
 - `koalaman/shellcheck v0.11.0@sha256:61862eba1fcf09a484ebcc6feea46f1782532571a34ed51fedf90dd25f925a8d`
 - `actions/checkout v7.0.1@3d3c42e5aac5ba805825da76410c181273ba90b1`
 - `actions/checkout v7.0.1@3d3c42e5aac5ba805825da76410c181273ba90b1`
 - `actions/checkout v7.0.1@3d3c42e5aac5ba805825da76410c181273ba90b1`
 - `actions/checkout v7.0.1@3d3c42e5aac5ba805825da76410c181273ba90b1`
 - `actions/checkout v7.0.1@3d3c42e5aac5ba805825da76410c181273ba90b1`
 - `actions/checkout v7.0.1@3d3c42e5aac5ba805825da76410c181273ba90b1`
 - `actions/checkout v7.0.1@3d3c42e5aac5ba805825da76410c181273ba90b1`
 - `actions/checkout v7.0.1@3d3c42e5aac5ba805825da76410c181273ba90b1`
 - `docker/setup-qemu-action v4.4.0@99012661954931238ded8c8b007157a8430204e1`
 - `docker/setup-buildx-action v4.4.1@f87e5991a6d7451dcb8d9637bfbc97413f497069`
 - `docker/build-push-action v7.4.0@c3c9e263c25d99ce0380d002d59b67737d91b0dc`
 - `docker/build-push-action v7.4.0@c3c9e263c25d99ce0380d002d59b67737d91b0dc`
 - `actions/checkout v7.0.1@3d3c42e5aac5ba805825da76410c181273ba90b1`
 - `docker/setup-buildx-action v4.4.1@f87e5991a6d7451dcb8d9637bfbc97413f497069`
 - `docker/build-push-action v7.4.0@c3c9e263c25d99ce0380d002d59b67737d91b0dc`
 - `actions/checkout v7.0.1@3d3c42e5aac5ba805825da76410c181273ba90b1`
 - `docker/setup-buildx-action v4.4.1@f87e5991a6d7451dcb8d9637bfbc97413f497069`
 - `docker/build-push-action v7.4.0@c3c9e263c25d99ce0380d002d59b67737d91b0dc`
 - `actions/checkout v7.0.1@3d3c42e5aac5ba805825da76410c181273ba90b1`
 - `docker/setup-buildx-action v4.4.1@f87e5991a6d7451dcb8d9637bfbc97413f497069`
 - `docker/build-push-action v7.4.0@c3c9e263c25d99ce0380d002d59b67737d91b0dc`
 - `actions/checkout v7.0.1@3d3c42e5aac5ba805825da76410c181273ba90b1`
 - `docker/setup-buildx-action v4.4.1@f87e5991a6d7451dcb8d9637bfbc97413f497069`
 - `docker/build-push-action v7.4.0@c3c9e263c25d99ce0380d002d59b67737d91b0dc`
 - `actions/checkout v7.0.1@3d3c42e5aac5ba805825da76410c181273ba90b1`
 - `docker/setup-qemu-action v4.4.0@99012661954931238ded8c8b007157a8430204e1`
 - `docker/setup-buildx-action v4.4.1@f87e5991a6d7451dcb8d9637bfbc97413f497069`
 - `docker/login-action v4.8.0@8aa5831fb8d48d8f76594c7ddde61ae58019f550`
 - `docker/metadata-action v6.2.0@dc802804100637a589fabce1cb79ff13a1411302`
 - `docker/build-push-action v7.4.0@c3c9e263c25d99ce0380d002d59b67737d91b0dc`
 - `docker/build-push-action v7.4.0@c3c9e263c25d99ce0380d002d59b67737d91b0dc`
 - `actions/checkout v7.0.1@3d3c42e5aac5ba805825da76410c181273ba90b1`

</details>

<details><summary>.forgejo/workflows/fuzz-weekly.yml (1)</summary>

 - `actions/checkout v7.0.1@3d3c42e5aac5ba805825da76410c181273ba90b1`

</details>

<details><summary>.forgejo/workflows/release.yml (5)</summary>

 - `actions/checkout v7.0.1@3d3c42e5aac5ba805825da76410c181273ba90b1`
 - `docker/setup-qemu-action v4.4.0@99012661954931238ded8c8b007157a8430204e1`
 - `docker/setup-buildx-action v4.4.1@f87e5991a6d7451dcb8d9637bfbc97413f497069`
 - `docker/build-push-action v7.4.0@c3c9e263c25d99ce0380d002d59b67737d91b0dc`
 - `actions/checkout v7.0.1@3d3c42e5aac5ba805825da76410c181273ba90b1`

</details>

<details><summary>.forgejo/workflows/scan-weekly.yml (1)</summary>

 - `actions/checkout v7.0.1@3d3c42e5aac5ba805825da76410c181273ba90b1`

</details>

</blockquote>
</details>

<details><summary>gomod (1)</summary>
<blockquote>

<details><summary>go.mod (2)</summary>

 - `go 1.27.3`
 - `github.com/aptible/supercronic v0.2.51`

</details>

</blockquote>
</details>

<details><summary>regex (6)</summary>
<blockquote>

<details><summary>.forgejo/workflows/docker-publish.yml (11)</summary>

 - `bats/bats 1.14.0@sha256:5322b877351fda0cc435de8c6116de7d0a2ec79d7c680132a0ef329a633bc66f`
 - `golang 1.27.4-trixie@sha256:f18c3d73a184fb0582828510722e723b63a93f7188d11dd223d9be6f6880e65d`
 - `golangci/golangci-lint v2.15.0@sha256:fbfcfd12a6f8d7f1b97b3362ca80fc2a244d46863148c73cb88e7793af96c481`
 - `golang 1.27.4-trixie@sha256:f18c3d73a184fb0582828510722e723b63a93f7188d11dd223d9be6f6880e65d`
 - `quay.io/skopeo/stable v1.24.1@sha256:4b440098ada2bc2623308b0f60f5a5454084b98683e9e7dcecd77bba69797c56` → [Updates: `v1.25.1`]
 - `quay.io/skopeo/stable v1.24.1@sha256:4b440098ada2bc2623308b0f60f5a5454084b98683e9e7dcecd77bba69797c56` → [Updates: `v1.25.1`]
 - `ghcr.io/sigstore/cosign/cosign v3.1.4@sha256:0f06154836d2de34f887a5c273506d52ca43edaa1d9e53e6510fa78a48f69c2d`
 - `quay.io/skopeo/stable v1.24.1@sha256:4b440098ada2bc2623308b0f60f5a5454084b98683e9e7dcecd77bba69797c56` → [Updates: `v1.25.1`]
 - `aquasec/trivy 0.76.2@sha256:8f6bb947dbc528902e11f2f5be52f7b519f6a39d5d148f41166f2040499768fb`
 - `quay.io/skopeo/stable v1.24.1@sha256:4b440098ada2bc2623308b0f60f5a5454084b98683e9e7dcecd77bba69797c56` → [Updates: `v1.25.1`]
 - `quay.io/skopeo/stable v1.24.1@sha256:4b440098ada2bc2623308b0f60f5a5454084b98683e9e7dcecd77bba69797c56` → [Updates: `v1.25.1`]

</details>

<details><summary>.forgejo/workflows/fuzz-weekly.yml (1)</summary>

 - `golang 1.27.4-trixie@sha256:f18c3d73a184fb0582828510722e723b63a93f7188d11dd223d9be6f6880e65d`

</details>

<details><summary>.forgejo/workflows/scan-weekly.yml (2)</summary>

 - `quay.io/skopeo/stable v1.24.1@sha256:4b440098ada2bc2623308b0f60f5a5454084b98683e9e7dcecd77bba69797c56` → [Updates: `v1.25.1`]
 - `aquasec/trivy 0.76.2@sha256:8f6bb947dbc528902e11f2f5be52f7b519f6a39d5d148f41166f2040499768fb`

</details>

<details><summary>Dockerfile (3)</summary>

 - `gilbertchen/duplicacy 3.3.0` → [Updates: `3.3.1`]
 - `rclone/rclone 1.76.0`
 - `moby/moby 29.9.3` → [Updates: `29.10.0`]

</details>

<details><summary>Dockerfile (3)</summary>

 - `gilbertchen/duplicacy 3.3.0` → [Updates: `3.3.1`]
 - `rclone/rclone 1.76.0`
 - `moby/moby 29.9.3` → [Updates: `29.10.0`]

</details>

<details><summary>tests/integration/podman.sh (1)</summary>

 - `quay.io/podman/stable v6.1.3@sha256:1d280cfab8e4cdaa221229c121f681c67463fbd82ad2fa6d13fdcf91c6fc747e` → [Updates: `v6.2.0`]

</details>

</blockquote>
</details>
