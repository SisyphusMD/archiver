#!/usr/bin/env bats
# .forgejo/scripts/next-version.sh decides which version a release dispatch cuts. It runs
# against a throwaway repo shaped like archiver's: 0.x on main, a release/0.11 branch, and
# main moving on to 1.0.0 alphas.

setup() {
  SCRIPT="$(cd "$(dirname "${BATS_TEST_FILENAME}")/.." && pwd)/.forgejo/scripts/next-version.sh"
  cd "${BATS_TEST_TMPDIR}"
  git init -q -b main repo
  cd repo
  git config user.email t@example.com
  git config user.name t
  commit() { git commit -q --allow-empty -m "$1"; }
  commit "0.11.0"; git tag v0.11.0
  commit "0.11.1"; git tag v0.11.1
  git branch release/0.11
}

next() { run bash "${SCRIPT}" "$@"; }

@test "main before any alpha: patch, minor, and major bump the newest stable tag" {
  next main patch; [ "$status" -eq 0 ]; [ "$output" = "0.11.2" ]
  next main minor; [ "$status" -eq 0 ]; [ "$output" = "0.12.0" ]
  next main major; [ "$status" -eq 0 ]; [ "$output" = "1.0.0" ]
}

@test "main: the first alpha targets the next major, later ones count up" {
  next main alpha; [ "$status" -eq 0 ]; [ "$output" = "1.0.0-alpha.1" ]
  git commit -q --allow-empty -m a1; git tag v1.0.0-alpha.1
  next main alpha; [ "$status" -eq 0 ]; [ "$output" = "1.0.0-alpha.2" ]
}

@test "main after an alpha refuses 0.x patch and minor releases but allows 1.0.0" {
  git commit -q --allow-empty -m a1; git tag v1.0.0-alpha.1
  next main patch; [ "$status" -ne 0 ]; [[ "$output" == *"release/0.11"* ]]
  next main minor; [ "$status" -ne 0 ]
  next main major; [ "$status" -eq 0 ]; [ "$output" = "1.0.0" ]
}

@test "main after 1.0.0 ships cuts 1.x patches and minors again, and alphas target 2.0.0" {
  git commit -q --allow-empty -m a1; git tag v1.0.0-alpha.1
  git commit -q --allow-empty -m r; git tag v1.0.0
  next main patch; [ "$status" -eq 0 ]; [ "$output" = "1.0.1" ]
  next main minor; [ "$status" -eq 0 ]; [ "$output" = "1.1.0" ]
  next main alpha; [ "$status" -eq 0 ]; [ "$output" = "2.0.0-alpha.1" ]
}

@test "release/0.11 cuts 0.11 patches and never sees main's newer tags" {
  git commit -q --allow-empty -m a1; git tag v1.0.0-alpha.1
  git checkout -q release/0.11
  next release/0.11 patch; [ "$status" -eq 0 ]; [ "$output" = "0.11.2" ]
  git commit -q --allow-empty -m p; git tag v0.11.2
  next release/0.11 patch; [ "$status" -eq 0 ]; [ "$output" = "0.11.3" ]
}

@test "release branches refuse anything but a patch of their own line" {
  git checkout -q release/0.11
  next release/0.11 minor; [ "$status" -ne 0 ]
  next release/0.11 alpha; [ "$status" -ne 0 ]
  next release/0.12 patch; [ "$status" -ne 0 ]; [[ "$output" == *"0.11.1"* ]]
}

@test "other branches cannot release" {
  next feature/x patch; [ "$status" -ne 0 ]
}

@test "an unknown bump is refused" {
  next main nightly; [ "$status" -ne 0 ]
}
