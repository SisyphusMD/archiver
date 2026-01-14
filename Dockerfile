# Multi-architecture Dockerfile for Archiver
# Supports: linux/amd64, linux/arm64

FROM debian:trixie-20260112-slim

# Set build argument for target architecture
ARG TARGETARCH

# Install required packages
RUN apt-get update && apt-get install -y \
    expect \
    openssh-client \
    openssl \
    wget \
    cron \
    curl \
    ca-certificates \
    gnupg \
    lsb-release \
    sqlite3 \
    && rm -rf /var/lib/apt/lists/*

# Set versions
ENV DUPLICACY_VERSION=3.2.5
ENV DOCKER_CLI_VERSION=5:29.1.4-1

# Install Docker CLI
RUN install -m 0755 -d /etc/apt/keyrings && \
    curl -fsSL https://download.docker.com/linux/debian/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg && \
    chmod a+r /etc/apt/keyrings/docker.gpg && \
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/debian trixie stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null && \
    apt-get update && \
    apt-get install -y docker-ce-cli=${DOCKER_CLI_VERSION}~debian.13~trixie && \
    rm -rf /var/lib/apt/lists/*

# Download and install Duplicacy based on architecture
RUN ARCH_SUFFIX="" && \
    if [ "$TARGETARCH" = "amd64" ]; then \
        ARCH_SUFFIX="x64"; \
    elif [ "$TARGETARCH" = "arm64" ]; then \
        ARCH_SUFFIX="arm64"; \
    else \
        echo "Unsupported architecture: $TARGETARCH" && exit 1; \
    fi && \
    wget -q "https://github.com/gilbertchen/duplicacy/releases/download/v${DUPLICACY_VERSION}/duplicacy_linux_${ARCH_SUFFIX}_${DUPLICACY_VERSION}" \
    -O /usr/local/bin/duplicacy && \
    chmod +x /usr/local/bin/duplicacy

# Create application directory
WORKDIR /opt/archiver

# Copy application files
COPY archiver.sh ./
COPY lib/ ./lib/
COPY examples/ ./examples/

# Create required directories
RUN mkdir -p /opt/archiver/logs \
    /opt/archiver/keys \
    /opt/archiver/exports \
    /opt/archiver/import

# Make archiver.sh executable and create symlink
RUN chmod +x /opt/archiver/archiver.sh && \
    ln -s /opt/archiver/archiver.sh /usr/local/bin/archiver

# Copy entrypoint script
COPY docker-entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

# Environment variables with defaults
ENV BUNDLE_PASSWORD="" \
    CRON_SCHEDULE=""

# Volumes
# /opt/archiver/bundle/bundle.tar.enc - Required: encrypted bundle file (mount as single file)
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
