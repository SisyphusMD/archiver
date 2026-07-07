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
    procps \
    nano \
    vim \
    iputils-ping \
    systemd \
    zfsutils-linux \
    && rm -rf /var/lib/apt/lists/*

# Scheduler: supercronic replaces Debian cron so scheduled backups run without the
# SETGID capability (cron forks setgid to exec jobs; supercronic runs them as the
# container user) and without cron's env-scrubbing. Trust anchor is HTTPS to github.com;
# supercronic ships no sidecar checksum file, so a build-time hash check adds nothing
# over TLS. Renovate tracks the pin via the comment below (dockerfileVersions preset).
# renovate: datasource=github-releases depName=aptible/supercronic
ARG SUPERCRONIC_VERSION=v0.2.47
ARG SUPERCRONIC_URL=https://github.com/aptible/supercronic/releases/download/${SUPERCRONIC_VERSION}/supercronic-linux-${TARGETARCH}
RUN curl -fsSL --retry 5 --retry-delay 3 --retry-all-errors --connect-timeout 15 --max-time 300 "$SUPERCRONIC_URL" -o /usr/local/bin/supercronic && \
    chmod +x /usr/local/bin/supercronic

# renovate: datasource=github-releases depName=gilbertchen/duplicacy extractVersion=^v(?<version>.+)$
ENV DUPLICACY_VERSION=3.2.5

# moby/moby's release tag pattern is `docker-vX.Y.Z`; client/api/* tags
# are filtered out by the extractVersion anchor. The static binary
# archive at download.docker.com/linux/static/stable/<arch>/docker-<v>.tgz
# is published shortly after each engine release.
# renovate: datasource=github-releases depName=moby/moby extractVersion=^docker-v(?<version>.+)$
ENV DOCKER_CLI_VERSION=29.6.1

# Pull the docker CLI binary directly from Docker's static archive
# instead of installing the docker-ce-cli debian package. The apt path
# uses a Debian-package version (`5:29.x.y-1~debian.13~trixie`) which has
# no clean Renovate datasource; the static binary uses plain SemVer.
RUN ARCH_SUFFIX="" && \
    if [ "$TARGETARCH" = "amd64" ]; then \
        ARCH_SUFFIX="x86_64"; \
    elif [ "$TARGETARCH" = "arm64" ]; then \
        ARCH_SUFFIX="aarch64"; \
    else \
        echo "Unsupported architecture: $TARGETARCH" && exit 1; \
    fi && \
    curl -fsSL --retry 5 --retry-delay 3 --retry-all-errors --connect-timeout 15 --max-time 300 "https://download.docker.com/linux/static/stable/${ARCH_SUFFIX}/docker-${DOCKER_CLI_VERSION}.tgz" \
        -o /tmp/docker-cli.tgz && \
    tar -xzC /usr/local/bin --strip-components=1 -f /tmp/docker-cli.tgz docker/docker && \
    rm /tmp/docker-cli.tgz

RUN ARCH_SUFFIX="" && \
    if [ "$TARGETARCH" = "amd64" ]; then \
        ARCH_SUFFIX="x64"; \
    elif [ "$TARGETARCH" = "arm64" ]; then \
        ARCH_SUFFIX="arm64"; \
    else \
        echo "Unsupported architecture: $TARGETARCH" && exit 1; \
    fi && \
    curl -fsSL --retry 5 --retry-delay 3 --retry-all-errors --connect-timeout 15 --max-time 300 "https://github.com/gilbertchen/duplicacy/releases/download/v${DUPLICACY_VERSION}/duplicacy_linux_${ARCH_SUFFIX}_${DUPLICACY_VERSION}" \
        -o /usr/local/bin/duplicacy && \
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

ENV CRON_SCHEDULE=""

# Volumes
# /opt/archiver/bundle/bundle.tar.enc - Optional: encrypted bundle (bundle mode / cold restore)
# /opt/archiver/bundle/ - For setup mode: directory to save generated bundle
# /opt/archiver/logs - Optional: persistent logs directory
# User must also mount their service directories to backup

VOLUME ["/opt/archiver/logs", "/opt/archiver/bundle"]

# Health check: Use archiver's built-in healthcheck command
# Runs comprehensive checks including config, keys, logs, and disk space
HEALTHCHECK --interval=5m --timeout=10s --start-period=1m --retries=3 \
    CMD archiver healthcheck >/dev/null 2>&1 || exit 1

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD []
