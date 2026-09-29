#!/usr/bin/env bash
# Print the version a release dispatch cuts from HEAD, or fail with the reason.
#
#   next-version.sh <branch> <patch|minor|major|alpha>
#
# main is the v1 line (ADR 9): it cuts 0.x releases only until its first 1.0.0 alpha, then
# alphas and 1.0.0 itself. release/<major>.<minor> branches cut that line's patches. Only
# tags reachable from HEAD count, so a release branch never sees main's newer tags.
set -euo pipefail

die() { echo "$*" >&2; exit 1; }

[ $# -eq 2 ] || die "usage: next-version.sh <branch> <patch|minor|major|alpha>"
branch=$1 bump=$2

stable=$(git tag --merged HEAD -l 'v*.*.*' | grep -vE -- '-' | sed 's/^v//' | sort -V | tail -n1 || true)
stable=${stable:-0.0.0}
IFS=. read -r major minor patch <<<"${stable}"
prerelease=$(git tag --merged HEAD -l 'v*.*.*-*' | sort -V | tail -n1 || true)
# Mid-prerelease while the newest prerelease targets a version beyond the newest stable one;
# once that version ships, its alphas stay reachable but no longer hold main back.
pre_target=${prerelease#v}
pre_target=${pre_target%%-*}
in_prerelease=false
if [ -n "${prerelease}" ] && [ "${pre_target}" != "${stable}" ] \
  && [ "$(printf '%s\n%s\n' "${stable}" "${pre_target}" | sort -V | tail -n1)" = "${pre_target}" ]; then
  in_prerelease=true
fi

case "${branch}" in
  main)
    if [ "${in_prerelease}" = true ] && { [ "${bump}" = patch ] || [ "${bump}" = minor ]; }; then
      die "main already carries ${prerelease}; cut ${major}.${minor} patches from release/${major}.${minor}"
    fi
    ;;
  release/*)
    line=${branch#release/}
    [ "${bump}" = patch ] || die "${branch} cuts patch releases only, not ${bump}"
    [ "${major}.${minor}" = "${line}" ] || die "the newest release on ${branch} is ${stable}, not a ${line}.x"
    ;;
  *)
    die "releases are cut from main or release/<major>.<minor>, not ${branch}"
    ;;
esac

case "${bump}" in
  patch) echo "${major}.${minor}.$((patch + 1))" ;;
  minor) echo "${major}.$((minor + 1)).0" ;;
  major) echo "$((major + 1)).0.0" ;;
  alpha)
    target="$((major + 1)).0.0"
    # All tags, not just reachable ones: an alpha number is never reused.
    n=$(git tag -l "v${target}-alpha.*" | sed "s/^v${target}-alpha\.//" | grep -E '^[0-9]+$' | sort -n | tail -n1 || true)
    echo "${target}-alpha.$((${n:-0} + 1))"
    ;;
  *) die "unknown bump ${bump}" ;;
esac
