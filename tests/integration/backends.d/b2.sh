# Backblaze B2, served by tests/fakes (b2.go): b2 downloads from the download URL the
# authorization hands out, b2-custom through a host of the user's own (a CDN in front of B2).
CASES+=(b2 b2-custom)
FAKE_HOSTS+=(api.backblazeb2.com api001.backblazeb2.com f001.backblazeb2.com pod-000-1000-00.backblazeb2.com b2-cdn.test)
B2_ID="0010123456789ab0000000001"
B2_KEY="K001archiverTestApplicationKey0123456"
B2_BUCKET="archiver-test"

b2_bucket() { # NAME N: the bucket exists (B2 has no implicit buckets; Duplicacy needs one)
  [ "$2" = 1 ] || return 0
  docker exec -e RCLONE_CONFIG_B2_TYPE=b2 -e RCLONE_CONFIG_B2_ACCOUNT="$B2_ID" -e RCLONE_CONFIG_B2_KEY="$B2_KEY" "$P-$1" bash -c '
    for _ in $(seq 1 30); do rclone --config /dev/null mkdir "B2:$1" 2>/dev/null && exit 0; sleep 1; done; exit 1' _ "$B2_BUCKET"
}

b2case() { # TYPE [EXTRA-FIELD]
  FIELDS=("TYPE=$1" "B2_BUCKETNAME=$B2_BUCKET" "B2_PATH=$1/%D" "${@:2}")
  SECRETS=("B2_ID=$B2_ID" "B2_KEY=$B2_KEY") PREP=b2_bucket
  run_case "$1" "$B2_BUCKET/$1/%D" TYPE=b2 "ACCOUNT=$B2_ID" "KEY=$B2_KEY"
}

case_b2() { b2case b2; }

case_b2_custom() {
  b2case b2-custom B2_DOWNLOAD_HOST=b2-cdn.test
  # Duplicacy and the envelope's fetch download by name from the custom host, not B2's own.
  local log; log="$(docker logs "$P-fakes" 2>&1)"
  grep -q ' GET b2-cdn.test /file/' <<<"$log" || die "b2-custom: nothing downloaded through the custom host"
}
