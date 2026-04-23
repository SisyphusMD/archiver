#!/bin/bash

REQUIRE_CONTAINER_SH_SOURCED=true

# Detect whether we're running inside a container. First match wins;
# order goes from cheapest/most-specific to most-general.
#
#   1. /.dockerenv           - Docker
#   2. /run/.containerenv    - Podman
#   3. $KUBERNETES_SERVICE_HOST - Kubernetes (set automatically in every
#      Pod regardless of CRI; catches containerd/CRI-O that don't create
#      either marker file).
#   4. /proc/self/cgroup     - generic fallback. PID 1's cgroup path
#      contains the runtime's scope keyword in every common container
#      runtime. Handles standalone containerd, CRI-O, LXC, rkt, and
#      Kubernetes in the rare case KUBERNETES_SERVICE_HOST is unset
#      (e.g. hostNetwork Pods with enableServiceLinks: false).
is_container() {
  [ -f /.dockerenv ] && return 0
  [ -f /run/.containerenv ] && return 0
  [ -n "${KUBERNETES_SERVICE_HOST:-}" ] && return 0
  if [ -r /proc/self/cgroup ] && \
     grep -qE '/(docker|kubepods|containerd|crio|libpod|podman|lxc)' /proc/self/cgroup; then
    return 0
  fi
  return 1
}

if ! is_container; then
  echo "ERROR: Only container deployment is supported" >&2
  echo "" >&2
  echo "Archiver must be run inside a container (Docker, Podman, Kubernetes, etc.)." >&2
  echo "Direct installation on the host system is no longer supported." >&2
  echo "" >&2
  echo "See the README for deployment instructions." >&2
  echo "" >&2
  exit 1
fi
