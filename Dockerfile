# Multi-architecture Dockerfile for Archiver
# Supports: linux/amd64, linux/arm64

# The Go CLI cross-compiles on the build platform, so an arm64 image is not built under
# emulation. Pure Go with CGO off: the binary needs nothing from the runtime image.
FROM --platform=$BUILDPLATFORM golang:1.27.2-trixie@sha256:e58d6f83b3416618d8bcac2b3dde1b7f7e3c4a77d25e88637f8bbae81536c48d AS cli
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
# CI points this at the NAS artifact cache; the default keeps the image buildable anywhere.
ARG GOPROXY=https://proxy.golang.org,direct
COPY go.mod go.sum ./
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags='-s -w' -o /out/archiver ./cmd/archiver

# Duplicacy is built from its pinned source with reviewed patches (ADR 26): build/duplicacy/
# dropbox-app.patch lets Dropbox refresh tokens with your own app instead of duplicacy.com, and
# highwayhash-arm64.patch renames an arm64 assembly table in a vendored fork whose name clash
# with a Go function current Go's linker rejects, keeping the exact bytes the released 3.2.5
# arm64 binary read there (so its hashes match the release's). Modules are vendored against
# the source's go.sum; the CLI still runs as a child process.
FROM --platform=$BUILDPLATFORM golang:1.27.2-trixie@sha256:e58d6f83b3416618d8bcac2b3dde1b7f7e3c4a77d25e88637f8bbae81536c48d AS duplicacy
ARG TARGETOS
ARG TARGETARCH
ARG GOPROXY=https://proxy.golang.org,direct
ARG DEBIAN_MIRROR=http://deb.debian.org
ARG GITHUB_MIRROR=https://github.com
# renovate: datasource=github-releases depName=gilbertchen/duplicacy extractVersion=^v(?<version>.+)$
ENV DUPLICACY_VERSION=3.2.5
ARG DUPLICACY_SOURCE_SHA256=9e289409b884d0c20f5b2e8b2fec64a019534fd394167346a3677ba785809b4c
COPY build/duplicacy/ /patches/
RUN { sed -i "s#http://deb.debian.org#${DEBIAN_MIRROR}#g" /etc/apt/sources.list.d/debian.sources 2>/dev/null || true; } && \
    apt-get update && apt-get install -y --no-install-recommends patch && \
    curl -fsSL --retry 5 --retry-delay 3 --retry-all-errors --connect-timeout 15 --max-time 300 \
        "${GITHUB_MIRROR}/gilbertchen/duplicacy/archive/refs/tags/v${DUPLICACY_VERSION}.tar.gz" -o /tmp/duplicacy.tar.gz && \
    echo "$DUPLICACY_SOURCE_SHA256  /tmp/duplicacy.tar.gz" | sha256sum -c - && \
    mkdir /src && tar -xzf /tmp/duplicacy.tar.gz -C /src --strip-components=1 && \
    cd /src && patch -p1 < /patches/dropbox-app.patch && \
    go mod vendor && patch -p1 < /patches/highwayhash-arm64.patch && \
    go test -count=1 -run TestDropboxAppTokens ./src/ && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags='-s -w' -o /out/duplicacy ./duplicacy

# rclone places the recovery kit on every storage type (ADR 24). Its release is a zip, so a
# throwaway stage unpacks it and only the binary reaches the image.
FROM debian:trixie-20260112-slim@sha256:77ba0164de17b88dd0bf6cdc8f65569e6e5fa6cd256562998b62553134a00ef0 AS rclone
ARG TARGETARCH
ARG DEBIAN_MIRROR=http://deb.debian.org
ARG GITHUB_MIRROR=https://github.com
# renovate: datasource=github-releases depName=rclone/rclone extractVersion=^v(?<version>.+)$
ENV RCLONE_VERSION=1.75.1
ARG RCLONE_SHA256_AMD64=982b5aa772841168f8e380f139e9e787b2a105403e32b94da8676a0e1c0a13ab
ARG RCLONE_SHA256_ARM64=03f2504174034b6d004152ed7369251c9a9ec1f7e0836eda420f5c7a5ec0dff9
RUN { sed -i "s#http://deb.debian.org#${DEBIAN_MIRROR}#g" /etc/apt/sources.list.d/debian.sources 2>/dev/null || true; } && \
    apt-get update && apt-get install -y --no-install-recommends curl ca-certificates unzip && \
    if [ "$TARGETARCH" = "amd64" ]; then SHA256="$RCLONE_SHA256_AMD64"; \
    elif [ "$TARGETARCH" = "arm64" ]; then SHA256="$RCLONE_SHA256_ARM64"; \
    else echo "Unsupported architecture: $TARGETARCH" && exit 1; fi && \
    curl -fsSL --retry 5 --retry-delay 3 --retry-all-errors --connect-timeout 15 --max-time 300 \
        "${GITHUB_MIRROR}/rclone/rclone/releases/download/v${RCLONE_VERSION}/rclone-v${RCLONE_VERSION}-linux-${TARGETARCH}.zip" -o /tmp/rclone.zip && \
    echo "$SHA256  /tmp/rclone.zip" | sha256sum -c - && \
    unzip -j /tmp/rclone.zip "*/rclone" -d /out && chmod 755 /out/rclone

FROM debian:trixie-20260112-slim@sha256:77ba0164de17b88dd0bf6cdc8f65569e6e5fa6cd256562998b62553134a00ef0

ARG TARGETARCH

ARG DEBIAN_MIRROR=http://deb.debian.org
RUN echo "deb http://deb.debian.org/debian trixie contrib" >> /etc/apt/sources.list.d/contrib.list && \
    { sed -i "s#http://deb.debian.org#${DEBIAN_MIRROR}#g" /etc/apt/sources.list.d/debian.sources /etc/apt/sources.list.d/contrib.list 2>/dev/null || true; } && \
    apt-get update && apt-get install -y \
    openssh-client \
    tini \
    openssl \
    curl \
    ca-certificates \
    tzdata \
    sqlite3 \
    qrencode \
    procps \
    nano \
    vim \
    iputils-ping \
    systemd \
    zfsutils-linux \
    btrfs-progs \
    python3 \
    python3-lmdb \
    && { sed -i "s#${DEBIAN_MIRROR}#http://deb.debian.org#g" /etc/apt/sources.list.d/debian.sources /etc/apt/sources.list.d/contrib.list 2>/dev/null || true; } \
    && rm -rf /var/lib/apt/lists/*

# Every download below is checked against a pinned SHA-256 per architecture: HTTPS
# authenticates the host, not the artifact, so a moved tag or a replaced release asset
# would otherwise land in every image. A version bump must bring new digests with it, or
# the build fails closed (tests/lint/dockerfile-downloads.sh guards the pattern).

ARG GITHUB_MIRROR=https://github.com

# moby/moby's release tag pattern is `docker-vX.Y.Z`; client/api/* tags
# are filtered out by the extractVersion anchor. The static binary
# archive at download.docker.com/linux/static/stable/<arch>/docker-<v>.tgz
# is published shortly after each engine release, without checksums; these are the
# digests of the reviewed archives.
# renovate: datasource=github-releases depName=moby/moby extractVersion=^docker-v(?<version>.+)$
ENV DOCKER_CLI_VERSION=29.6.2
ARG DOCKER_CLI_SHA256_AMD64=d6204aea92238e2453d5445c885b9d2e5eb8f82915568ec50edf9dbe12a3ac74
ARG DOCKER_CLI_SHA256_ARM64=8d16d8b3b158c132a9fb9963d4b4345746f925e287e154c9ed880ac257baf292

# Pull the docker CLI binary directly from Docker's static archive
# instead of installing the docker-ce-cli debian package. The apt path
# uses a Debian-package version (`5:29.x.y-1~debian.13~trixie`) which has
# no clean Renovate datasource; the static binary uses plain SemVer.
ARG DOCKER_DOWNLOAD_MIRROR=https://download.docker.com
RUN ARCH_SUFFIX="" && \
    if [ "$TARGETARCH" = "amd64" ]; then \
        ARCH_SUFFIX="x86_64"; SHA256="$DOCKER_CLI_SHA256_AMD64"; \
    elif [ "$TARGETARCH" = "arm64" ]; then \
        ARCH_SUFFIX="aarch64"; SHA256="$DOCKER_CLI_SHA256_ARM64"; \
    else \
        echo "Unsupported architecture: $TARGETARCH" && exit 1; \
    fi && \
    curl -fsSL --retry 5 --retry-delay 3 --retry-all-errors --connect-timeout 15 --max-time 300 "${DOCKER_DOWNLOAD_MIRROR}/linux/static/stable/${ARCH_SUFFIX}/docker-${DOCKER_CLI_VERSION}.tgz" \
        -o /tmp/docker-cli.tgz && \
    echo "$SHA256  /tmp/docker-cli.tgz" | sha256sum -c - && \
    tar -xzC /usr/local/bin --strip-components=1 -f /tmp/docker-cli.tgz docker/docker && \
    rm /tmp/docker-cli.tgz


WORKDIR /opt/archiver

COPY lib/logos/ ./lib/logos/
COPY docs/examples/ ./examples/

RUN mkdir -p /opt/archiver/logs /opt/archiver/keys

COPY --from=cli /out/archiver /usr/local/bin/archiver
COPY --from=rclone /out/rclone /usr/local/bin/rclone
COPY --from=duplicacy /out/duplicacy /usr/local/bin/duplicacy


# Hooks are executables (ADR 20); the e2e harness reads this to write them in that form.
LABEL io.archiver.hooks="executable"

ENV BACKUP_SCHEDULE=""

# Volumes
# /opt/archiver/logs - Optional: persistent logs directory
# User must also mount their service directories to backup
# /opt/archiver/bundle is deliberately NOT declared: Compose carries the previous
# container's mount over for image-declared volume paths on recreate, so a deployment that
# converted from a bundle would inherit the old bundle mount and refuse to start.

VOLUME ["/opt/archiver/logs"]

# Health check: Use archiver's built-in healthcheck command
# Runs comprehensive checks including config, keys, logs, and disk space
HEALTHCHECK --interval=5m --timeout=10s --start-period=1m --retries=3 \
    CMD archiver healthcheck >/dev/null 2>&1 || exit 1

# tini is PID 1: it reaps processes orphaned by hooks (a Go PID 1 would not) and passes
# docker stop's SIGTERM to the entrypoint, which stops everything gracefully.
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/archiver", "entrypoint"]
CMD []
