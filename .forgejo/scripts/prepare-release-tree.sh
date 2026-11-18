#!/usr/bin/env bash
# Reproduce the release tree in a clean candidate checkout.
set -euo pipefail
channel="${1:?release channel}"
bump="${2:?version bump}"
DATE="${3:?UTC release date}"
[[ "$channel" =~ ^(main|release/(0|[1-9][0-9]*)\.(0|[1-9][0-9]*))$ ]]
[[ "$DATE" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]
here="$(cd "$(dirname "$0")" && pwd)"
VERSION=$(bash "$here/next-version.sh" "$channel" "$bump")
TAG="v$VERSION"
grep -q '^## \[Unreleased\]' CHANGELOG.md
if grep -Fq "## [$VERSION]" CHANGELOG.md || git show-ref --verify --quiet "refs/tags/$TAG"; then
    echo "::error::Version $VERSION already exists" >&2
    exit 1
fi
previous=$(git describe --tags --abbrev=0 --match 'v*.*.*' 2>/dev/null || true)
range=HEAD
if [ -n "$previous" ]; then
    range="$previous..HEAD"
fi
DEPS=$(git log "$range" --pretty=format:%s | grep -E '^(chore|fix)\(deps\):' || true)
export DEPS
# ENVIRON["DEPS"] avoids awk's -v backslash-escape processing,
# which would corrupt subjects containing backslashes.
awk -v ver="$VERSION" -v date="$DATE" '
  BEGIN { in_section = 0; emitted = 0; deps = ENVIRON["DEPS"] }
  /^## \[Unreleased\]/ {
    suffix = $0
    sub(/^## \[Unreleased\]/, "", suffix)
    print "## [Unreleased]"
    print ""
    print "## [" ver "] - " date suffix
    in_section = 1
    next
  }
  in_section && /^## \[/ {
    if (!emitted && deps != "") {
      print "### Dependencies"
      print ""
      n = split(deps, lines, "\n")
      for (i = 1; i <= n; i++) if (lines[i] != "") print "- " lines[i]
      print ""
      emitted = 1
    }
    in_section = 0
    print
    next
  }
  { print }
  END {
    if (in_section && !emitted && deps != "") {
      print ""
      print "### Dependencies"
      print ""
      n = split(deps, lines, "\n")
      for (i = 1; i <= n; i++) if (lines[i] != "") print "- " lines[i]
    }
  }
' CHANGELOG.md > CHANGELOG.md.new
mv CHANGELOG.md.new CHANGELOG.md

if [ "$bump" != alpha ] && [ "$bump" != rc ]; then
    mapfile -d '' -t docs < <(git ls-files -z 'docs/*.md' ':!docs/adr/*')
    sed -i -E "s|(forgejo\\.bryantserver\\.com/sisyphusmd/archiver:)[0-9]+\\.[0-9]+\\.[0-9]+|\\1$VERSION|g" \
        compose.yaml README.md "${docs[@]}"
fi
printf '%s\n' "$VERSION"
