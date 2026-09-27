#!/usr/bin/env bash
# Every file the image build downloads must be checked against a pinned SHA-256 in the same
# RUN step. HTTPS authenticates the host, not the artifact: a moved release tag or a
# replaced asset would otherwise be built into every image unnoticed.
#
#   bash tests/lint/dockerfile-downloads.sh [Dockerfile]

set -euo pipefail

file="${1:-Dockerfile}"
[ -r "$file" ] || { echo "no such file: $file" >&2; exit 2; }

# Joins backslash continuations so each instruction is one record, then flags every RUN
# that invokes curl or wget (options, a URL, or a $VAR follow, unlike an apt package name) but never
# runs `sha256sum -c`.
awk -v file="$file" '
  function check(text, line) {
    if (text ~ /^[[:space:]]*RUN[[:space:]]/ && text ~ /(curl|wget)[[:space:]]+(-|"?https?:|"?[$])/ && text !~ /sha256sum[[:space:]]+(-c|--check)/) {
      printf "::error file=%s,line=%d::RUN downloads without a sha256sum -c check\n", file, line
      failed = 1
    }
  }
  {
    if (buf == "") start = NR
    line = $0
    if (sub(/\\[[:space:]]*$/, "", line)) { buf = buf line " "; next }
    buf = buf line
    check(buf, start)
    buf = ""
  }
  END { if (buf != "") check(buf, start); exit (failed ? 1 : 0) }
' "$file"

echo "OK: every download in $file is checksum-verified"
