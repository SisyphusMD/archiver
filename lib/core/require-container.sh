#!/bin/bash

REQUIRE_CONTAINER_SH_SOURCED=true

# Check if running in a container (Docker or Podman)
# Docker creates /.dockerenv, Podman creates /run/.containerenv
if [ ! -f "/.dockerenv" ] && [ ! -f "/run/.containerenv" ]; then
  echo "ERROR: Only container deployment is supported" >&2
  echo "" >&2
  echo "Archiver must be run inside a Docker or Podman container." >&2
  echo "Direct installation on the host system is no longer supported." >&2
  echo "" >&2
  echo "See the README for deployment instructions." >&2
  echo "" >&2
  exit 1
fi
