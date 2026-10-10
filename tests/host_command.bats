#!/usr/bin/env bats
# lib/host/archiver, the host-side command: which runtime it picks, when it adds sudo and a
# TTY, and that it passes the arguments on. Fake runtimes record how they were called.

bats_require_minimum_version 1.5.0

setup() {
  BIN=$(mktemp -d)
  export LOG=$BIN/calls
  fake() { # fake NAME [exit of `container inspect`: 1 = this user cannot see the container]
    printf '#!/bin/sh\n[ "$1" = container ] && exit %s\necho "%s $*" >> "$LOG"\n' "${2:-0}" "$1" > "$BIN/$1"
    chmod +x "$BIN/$1"
  }
  # A sudo that records itself and runs the rest.
  printf '#!/bin/sh\necho "sudo" >> "$LOG"\nexec "$@"\n' > "$BIN/sudo"
  chmod +x "$BIN/sudo"
  ln -s "$(command -v id)" "$BIN/id"
  SCRIPT=$BATS_TEST_DIRNAME/../lib/host/archiver
}

teardown() { rm -rf "$BIN"; }

run_host() { PATH=$BIN:/bin run sh "$SCRIPT" "$@" </dev/null; }

@test "docker is preferred, stdin is passed, arguments go through" {
  fake docker; fake podman
  run_host restore --yes "two words"
  [ "$status" -eq 0 ]
  [ "$(cat "$LOG")" = "docker exec -i archiver archiver restore --yes two words" ]
}

@test "podman when there is no docker; ARCHIVER_CONTAINER names the container" {
  fake podman
  ARCHIVER_CONTAINER=backups run_host status
  [ "$(cat "$LOG")" = "podman exec -i backups archiver status" ]
}

@test "ARCHIVER_RUNTIME wins" {
  fake docker; fake podman
  ARCHIVER_RUNTIME=$BIN/podman run_host status
  [ "$(cat "$LOG")" = "podman exec -i archiver archiver status" ]
}

@test "sudo when this user cannot see the container (a rootful one from a rootless store, Synology's docker)" {
  [ "$(id -u)" != 0 ] || skip "runs as root: sudo is never needed"
  fake docker 1
  run_host status
  [ "$(head -1 "$LOG")" = "sudo" ]
  [ "$(tail -1 "$LOG")" = "docker exec -i archiver archiver status" ]
}

@test "no runtime is a clear error" {
  PATH=$BIN:/bin run -127 sh "$SCRIPT" status </dev/null
  [[ "$output" == *"neither docker nor podman"* ]]
}
