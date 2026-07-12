#!/usr/bin/env bats
# lib/core/lockfile.sh arbitrates all backup concurrency: acquisition, stale recovery,
# stage/context tracking for stop/status/pause, pause-time accounting, and the stop flag.
# LOCKFILE/STOP_FLAG (declared in common.sh) are re-pointed at a per-test tmpdir.

setup() {
  REPO_ROOT="$(cd "$(dirname "${BATS_TEST_FILENAME}")/.." && pwd)"
  COMMON_SH_SOURCED=true
  LOGGING_SH_SOURCED=true
  source_if_not_sourced() { :; }
  log_message() { :; }
  # The functions operate on ARCHIVER_LOCKFILE/ARCHIVER_STOP_FLAG_FILE (captured from
  # LOCKFILE/STOP_FLAG at source time), so point them at the tmpdir BEFORE sourcing.
  LOCKFILE="${BATS_TEST_TMPDIR}/archiver-main.lock"
  STOP_FLAG="${BATS_TEST_TMPDIR}/archiver-stop-requested"
  STORAGE_LOCK_PREFIX="${BATS_TEST_TMPDIR}/archiver-storage-"
  # shellcheck source=/dev/null
  source "${REPO_ROOT}/lib/core/lockfile.sh"
}

@test "acquire_lock: fresh acquisition writes PID, context, stage, and a running record" {
  run_status=0; acquire_lock || run_status=$?
  [ "${run_status}" -eq 0 ]
  head -n1 "${LOCKFILE}" | grep -q "^$$ duplicacy pre-backup$"
  sed -n '2p' "${LOCKFILE}" | grep -qE '^[0-9]+ running$'
}

@test "acquire_lock: refused while a live process holds the lock" {
  sleep 5 &
  holder=$!
  echo "${holder} duplicacy backup" >"${LOCKFILE}"
  run_status=0; acquire_lock || run_status=$?
  [ "${run_status}" -eq 1 ]
  # untouched: still the holder's lock
  [ "$(head -n1 "${LOCKFILE}")" = "${holder} duplicacy backup" ]
  kill "${holder}" 2>/dev/null || true
}

@test "acquire_lock: stale lock (dead PID) is recovered AND the lock is actually re-taken" {
  echo "999999 duplicacy backup" >"${LOCKFILE}"
  run_status=0; acquire_lock || run_status=$?
  [ "${run_status}" -eq 2 ]
  # the recovered run must hold the lock itself, not proceed lockless
  head -n1 "${LOCKFILE}" | grep -q "^$$ duplicacy pre-backup$"
  sed -n '2p' "${LOCKFILE}" | grep -qE '^[0-9]+ running$'
}

@test "acquire_lock: a malformed lock (empty PID) counts as stale and is re-taken" {
  printf '\n' >"${LOCKFILE}"
  run_status=0; acquire_lock || run_status=$?
  [ "${run_status}" -eq 2 ]
  head -n1 "${LOCKFILE}" | grep -q "^$$ duplicacy pre-backup$"
}

@test "update_lock_stage: rewrites line 1, preserves PID and state history" {
  acquire_lock
  record_state_change "paused"
  update_lock_stage "service:/data/svc" "backup"
  [ "$(get_lock_pid)" = "$$" ]
  [ "$(get_lock_context)" = "service:/data/svc" ]
  [ "$(get_lock_stage)" = "backup" ]
  # history intact: running then paused
  sed -n '2p' "${LOCKFILE}" | grep -qE '^[0-9]+ running$'
  sed -n '3p' "${LOCKFILE}" | grep -qE '^[0-9]+ paused$'
}

@test "is_paused reflects the LAST state record" {
  acquire_lock
  ! is_paused
  record_state_change "paused"
  is_paused
  record_state_change "running"
  ! is_paused
}

@test "get_backup_start_time returns the first state record's timestamp" {
  {
    echo "$$ duplicacy pre-backup"
    echo "1000 running"
    echo "2000 paused"
  } >"${LOCKFILE}"
  [ "$(get_backup_start_time)" = "1000" ]
}

@test "calculate_total_pause_time sums closed pause windows" {
  {
    echo "$$ duplicacy pre-backup"
    echo "100 running"
    echo "110 paused"
    echo "130 running"
    echo "150 paused"
    echo "155 running"
  } >"${LOCKFILE}"
  [ "$(calculate_total_pause_time)" = "25" ]
}

@test "is_lock_valid: false without a file, false for a dead PID, true for a live one" {
  ! is_lock_valid
  echo "999999 duplicacy backup" >"${LOCKFILE}"
  ! is_lock_valid
  echo "$$ duplicacy backup" >"${LOCKFILE}"
  is_lock_valid
}

@test "release_lock removes the lock and any pending stop flag" {
  acquire_lock
  request_stop
  is_stop_requested
  release_lock
  [ ! -e "${LOCKFILE}" ]
  ! is_stop_requested
}

# ── Per-storage locks ──────────────────────────────────────────────────────────

@test "try_acquire_storage_lock: fresh acquisition writes 'PID label' with content" {
  run_status=0; try_acquire_storage_lock "s1" "backup-copy" || run_status=$?
  [ "${run_status}" -eq 0 ]
  [ "$(cat "${STORAGE_LOCK_PREFIX}s1.lock")" = "$$ backup-copy" ]
}

@test "try_acquire_storage_lock: refused (returns 1) while a live holder owns it, file untouched" {
  sleep 5 &
  holder=$!
  echo "${holder} maintenance" >"${STORAGE_LOCK_PREFIX}s2.lock"
  run_status=0; try_acquire_storage_lock "s2" "backup-copy" || run_status=$?
  [ "${run_status}" -eq 1 ]
  [ "$(cat "${STORAGE_LOCK_PREFIX}s2.lock")" = "${holder} maintenance" ]
  kill "${holder}" 2>/dev/null || true
}

@test "try_acquire_storage_lock: dead-PID lock is reaped and re-taken by us" {
  echo "999999 maintenance" >"${STORAGE_LOCK_PREFIX}s3.lock"
  run_status=0; try_acquire_storage_lock "s3" "backup-copy" || run_status=$?
  [ "${run_status}" -eq 0 ]
  [ "$(cat "${STORAGE_LOCK_PREFIX}s3.lock")" = "$$ backup-copy" ]
}

@test "try_acquire_storage_lock: empty/malformed lock counts as stale and is re-taken" {
  printf '\n' >"${STORAGE_LOCK_PREFIX}s4.lock"
  run_status=0; try_acquire_storage_lock "s4" "backup-copy" || run_status=$?
  [ "${run_status}" -eq 0 ]
  [ "$(cat "${STORAGE_LOCK_PREFIX}s4.lock")" = "$$ backup-copy" ]
}

@test "try_acquire_storage_lock: re-acquire by the same process returns 0 (reentrant)" {
  try_acquire_storage_lock "s5" "backup-copy"
  run_status=0; try_acquire_storage_lock "s5" "maintenance" || run_status=$?
  [ "${run_status}" -eq 0 ]
  # unchanged: still our original label, not rewritten
  [ "$(cat "${STORAGE_LOCK_PREFIX}s5.lock")" = "$$ backup-copy" ]
}

@test "get_storage_lock_holder returns the (multi-word) label" {
  echo "4242 backup-copy-source" >"${STORAGE_LOCK_PREFIX}s6.lock"
  [ "$(get_storage_lock_holder "s6")" = "backup-copy-source" ]
}

@test "release_storage_lock: no-op for a foreign live holder, removes our own" {
  sleep 5 &
  holder=$!
  echo "${holder} maintenance" >"${STORAGE_LOCK_PREFIX}s7.lock"
  release_storage_lock "s7" || true       # owner-guard makes this a no-op (non-owner)
  [ -e "${STORAGE_LOCK_PREFIX}s7.lock" ]   # foreign lock preserved
  kill "${holder}" 2>/dev/null || true

  try_acquire_storage_lock "s8" "backup-copy"
  release_storage_lock "s8"
  [ ! -e "${STORAGE_LOCK_PREFIX}s8.lock" ] # our own lock removed
}

@test "acquire_storage_lock: returns 1 after a short timeout while a live holder owns it" {
  sleep 10 &
  holder=$!
  echo "${holder} maintenance" >"${STORAGE_LOCK_PREFIX}s9.lock"
  run_status=0; acquire_storage_lock "s9" "backup-copy" 0 || run_status=$?
  [ "${run_status}" -eq 1 ]
  kill "${holder}" 2>/dev/null || true
}

@test "acquire_storage_lock: returns 130 when a stop is requested while waiting" {
  sleep 10 &
  holder=$!
  echo "${holder} maintenance" >"${STORAGE_LOCK_PREFIX}s10.lock"
  request_stop
  run_status=0; acquire_storage_lock "s10" "backup-copy" 43200 || run_status=$?
  [ "${run_status}" -eq 130 ]
  kill "${holder}" 2>/dev/null || true
}
