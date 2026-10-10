#!/bin/sh
# Runs every Fuzz target in the module for FUZZTIME each (ADR 39): 30 s per change in CI,
# longer weekly. Run inside the Go image with the tree in the working directory.
set -eu
: "${FUZZTIME:=30s}"
# Discovery is checked step by step: a package that fails to build must fail the run, not
# leave it with nothing to fuzz and a pass.
pkgs=$(go list ./cmd/... ./internal/...)
found=0
for pkg in $pkgs; do
  list=$(go test -list '^Fuzz' "$pkg")
  for target in $(printf '%s\n' "$list" | grep '^Fuzz' || true); do
    found=$((found + 1))
    echo "=== $target ($pkg), $FUZZTIME"
    if ! go test -run='^$' -fuzz="^${target}\$" -fuzztime="$FUZZTIME" "$pkg"; then
      # The container is thrown away: print the failing input so it can become a seed.
      dir=$(go list -f '{{.Dir}}' "$pkg")/testdata/fuzz/$target
      for f in "$dir"/*; do
        [ -f "$f" ] && { echo "--- failing input $f:"; cat "$f"; }
      done
      exit 1
    fi
  done
done
[ "$found" -gt 0 ] || { echo "no Fuzz targets found" >&2; exit 1; }
echo "fuzzed $found targets"
