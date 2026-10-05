# Multi-architecture Dockerfile for Archiver
# Supports: linux/amd64, linux/arm64

FROM debian:trixie-20260112-slim@sha256:77ba0164de17b88dd0bf6cdc8f65569e6e5fa6cd256562998b62553134a00ef0

ARG TARGETARCH

RUN echo "deb http://deb.debian.org/debian trixie contrib" >> /etc/apt/sources.list.d/contrib.list && \
    apt-get update && apt-get install -y \
    expect \
    openssh-client \
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
    && rm -rf /var/lib/apt/lists/*

# Every download below is checked against a pinned SHA-256 per architecture: HTTPS
# authenticates the host, not the artifact, so a moved tag or a replaced release asset
# would otherwise land in every image. A version bump must bring new digests with it, or
# the build fails closed (tests/lint/dockerfile-downloads.sh guards the pattern).

# Scheduler: supercronic replaces Debian cron so scheduled backups run without the
# SETGID capability (cron forks setgid to exec jobs; supercronic runs them as the
# container user) and without cron's env-scrubbing. Digests match GitHub's asset digests.
# Renovate tracks the pin via the comment below (dockerfileVersions preset).
# renovate: datasource=github-releases depName=aptible/supercronic
ARG SUPERCRONIC_VERSION=v0.2.49
ARG SUPERCRONIC_SHA256_AMD64=a53ae236602c7338aba3fbaff40bda6300eae3b9fedb8261eb06cfe3724430c1
ARG SUPERCRONIC_SHA256_ARM64=02aa0cb229ba09050cba6638059dadb9eedc2276632ea43d6a57a2f8c1629dd5
ARG SUPERCRONIC_URL=https://github.com/aptible/supercronic/releases/download/${SUPERCRONIC_VERSION}/supercronic-linux-${TARGETARCH}
RUN case "$TARGETARCH" in \
        amd64) SHA256="$SUPERCRONIC_SHA256_AMD64" ;; \
        arm64) SHA256="$SUPERCRONIC_SHA256_ARM64" ;; \
        *) echo "Unsupported architecture: $TARGETARCH" && exit 1 ;; \
    esac && \
    curl -fsSL --retry 5 --retry-delay 3 --retry-all-errors --connect-timeout 15 --max-time 300 "$SUPERCRONIC_URL" -o /usr/local/bin/supercronic && \
    echo "$SHA256  /usr/local/bin/supercronic" | sha256sum -c - && \
    chmod +x /usr/local/bin/supercronic

# Duplicacy publishes no checksums; these are the digests of the reviewed downloads.
# renovate: datasource=github-releases depName=gilbertchen/duplicacy extractVersion=^v(?<version>.+)$
ENV DUPLICACY_VERSION=3.2.5
ARG DUPLICACY_SHA256_AMD64=548526d462fb38c23f2bf62ea3b1177b8ad11cc1499fa3dbe092a607d68d84f5
ARG DUPLICACY_SHA256_ARM64=9c27d8ba149e67d0bc58406c6b3218661d870cb07e265aec31563540f8f20598

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
RUN ARCH_SUFFIX="" && \
    if [ "$TARGETARCH" = "amd64" ]; then \
        ARCH_SUFFIX="x86_64"; SHA256="$DOCKER_CLI_SHA256_AMD64"; \
    elif [ "$TARGETARCH" = "arm64" ]; then \
        ARCH_SUFFIX="aarch64"; SHA256="$DOCKER_CLI_SHA256_ARM64"; \
    else \
        echo "Unsupported architecture: $TARGETARCH" && exit 1; \
    fi && \
    curl -fsSL --retry 5 --retry-delay 3 --retry-all-errors --connect-timeout 15 --max-time 300 "https://download.docker.com/linux/static/stable/${ARCH_SUFFIX}/docker-${DOCKER_CLI_VERSION}.tgz" \
        -o /tmp/docker-cli.tgz && \
    echo "$SHA256  /tmp/docker-cli.tgz" | sha256sum -c - && \
    tar -xzC /usr/local/bin --strip-components=1 -f /tmp/docker-cli.tgz docker/docker && \
    rm /tmp/docker-cli.tgz

RUN ARCH_SUFFIX="" && \
    if [ "$TARGETARCH" = "amd64" ]; then \
        ARCH_SUFFIX="x64"; SHA256="$DUPLICACY_SHA256_AMD64"; \
    elif [ "$TARGETARCH" = "arm64" ]; then \
        ARCH_SUFFIX="arm64"; SHA256="$DUPLICACY_SHA256_ARM64"; \
    else \
        echo "Unsupported architecture: $TARGETARCH" && exit 1; \
    fi && \
    curl -fsSL --retry 5 --retry-delay 3 --retry-all-errors --connect-timeout 15 --max-time 300 "https://github.com/gilbertchen/duplicacy/releases/download/v${DUPLICACY_VERSION}/duplicacy_linux_${ARCH_SUFFIX}_${DUPLICACY_VERSION}" \
        -o /usr/local/bin/duplicacy && \
    echo "$SHA256  /usr/local/bin/duplicacy" | sha256sum -c - && \
    chmod +x /usr/local/bin/duplicacy

WORKDIR /opt/archiver

COPY archiver.sh ./
COPY lib/ ./lib/
COPY docs/examples/ ./examples/

RUN mkdir -p /opt/archiver/logs \
    /opt/archiver/keys \
    /opt/archiver/exports \
    /opt/archiver/import

RUN chmod +x /opt/archiver/archiver.sh && \
    chmod +x /opt/archiver/lib/scripts/*.sh && \
    ln -s /opt/archiver/archiver.sh /usr/local/bin/archiver

COPY docker-entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

ENV BACKUP_SCHEDULE=""

# Volumes
# /opt/archiver/logs - Optional: persistent logs directory
# User must also mount their service directories to backup
# /opt/archiver/bundle is deliberately NOT declared: Compose carries the previous
# container's mount over for image-declared volume paths on recreate, so a bundle-mode
# deployment switching to env-native would inherit the old bundle mount and fail fast
# (bundle present, no password).

VOLUME ["/opt/archiver/logs"]

# Health check: Use archiver's built-in healthcheck command
# Runs comprehensive checks including config, keys, logs, and disk space
HEALTHCHECK --interval=5m --timeout=10s --start-period=1m --retries=3 \
    CMD archiver healthcheck >/dev/null 2>&1 || exit 1

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD []
