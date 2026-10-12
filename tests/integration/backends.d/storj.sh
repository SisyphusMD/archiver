# Storj: a real local network from Storj's own storj-up image (a satellite on Postgres and
# Redis, one storage node), not a fake: Duplicacy's uplink and rclone speak DRPC to the
# satellite and the node. The redundancy scheme is cut to 1/1/1/1 so one node holds every piece.
CASES+=(storj)

# Digest-pinned like backends.sh's images (Renovate does not scan shell scripts; bump by hand).
# Satellite and node v1.158.2 serve Duplicacy 3.2.5's uplink v1.12.1 as well as rclone's.
STORJ_IMAGE="ghcr.io/storj/up/storj:1.158.2@sha256:d33eb84607ccf56423b0cd0c83e6fa0d355fc2688302ed57c518689fd0bda13c"
STORJ_PG_IMAGE="postgres:18.6-alpine@sha256:77f585114c32fbca283dc835b0596f4e52b51b4c6662d7810b2f4084f60a1873"
STORJ_REDIS_IMAGE="redis:8.10.2-alpine@sha256:3811787313eba226a2ef38658c6ccb91cd5e110edc89c37767de373120a0e5a0"
# storj-up's predefined satellite identity; uplink needs the node ID for any satellite it does not know.
STORJ_SAT="12whfK1EDvHJtajBiAUeajQLYcWqxcQmdYQU5zX5cCf6bAxfgu4@satellite-api:7777"
STORJ_PASS="storj-test-passphrase"

storj_ready() { # NAME N: the bucket exists and a remote (non-inline) piece can be stored
  [ "$2" = 1 ] || return 0
  docker exec -e RCLONE_CONFIG_SJ_TYPE=storj -e RCLONE_CONFIG_SJ_PROVIDER=new -e RCLONE_CONFIG_SJ_SATELLITE_ADDRESS="$STORJ_SAT" \
    -e RCLONE_CONFIG_SJ_API_KEY="$STORJ_KEY" -e RCLONE_CONFIG_SJ_PASSPHRASE="$STORJ_PASS" "$P-$1" bash -c '
      head -c 20000 /dev/urandom > /tmp/probe
      for _ in $(seq 1 60); do
        rclone --config /dev/null mkdir "SJ:$1" 2>/dev/null && rclone --config /dev/null copyto /tmp/probe "SJ:$1/probe" 2>/dev/null \
          && rclone --config /dev/null deletefile "SJ:$1/probe" && exit 0
        sleep 1
      done; exit 1' _ "$BUCKET"
}

case_storj() {
  local sn=(STORJUP_ROLE=storagenode STORJ_WAIT_FOR_SATELLITE=true STORJ_IDENTITY_DIR=/storj/.local/share/storj/identity/storagenode/
    STORJ_OPERATOR_EMAIL=node@example.com STORJ_OPERATOR_WALLET=0x0123456789012345678901234567890123456789
    STORJ_STORAGE2_TRUST_SOURCES="$STORJ_SAT" STORJ_STORAGE_ALLOCATED_DISK_SPACE=1G STORJ_STORAGE2_MONITOR_MINIMUM_DISK_SPACE=0
    STORJ_SERVER_ADDRESS=0.0.0.0:30001 STORJ_SERVER_PRIVATE_ADDRESS=0.0.0.0:30002 STORJ_CONSOLE_ADDRESS=0.0.0.0:30000
    STORJ_CONTACT_EXTERNAL_ADDRESS=storagenode1:30001)
  local db="postgres://postgres@postgres:5432/master?sslmode=disable"
  local sat=(STORJUP_ROLE=satellite-api STORJ_WAIT_FOR_DB=true STORJ_USE_PREDEFINED_IDENTITY=1
    STORJ_IDENTITY_DIR=/storj/.local/share/storj/identity/satellite-api/ STORJ_DATABASE="$db" STORJ_METAINFO_DATABASE_URL="$db"
    "STORJ_LIVE_ACCOUNTING_STORAGE_BACKEND=redis://redis:6379?db=0" "STORJ_SERVER_REVOCATION_DBURL=redis://redis:6379?db=1"
    STORJ_SERVER_ADDRESS=0.0.0.0:7777 STORJ_SERVER_PRIVATE_ADDRESS=0.0.0.0:5559 STORJ_CONTACT_EXTERNAL_ADDRESS=satellite-api:7777
    STORJ_CONSOLE_ADDRESS=0.0.0.0:10000 STORJ_CONSOLE_EXTERNAL_ADDRESS=http://satellite-api:10000/ STORJ_CONSOLE_AUTH_TOKEN_SECRET=storj-test-secret
    STORJ_CONSOLE_OPEN_REGISTRATION_ENABLED=true STORJ_CONSOLE_SIGNUP_ACTIVATION_CODE_ENABLED=false STORJ_CONSOLE_RATE_LIMIT_BURST=10000
    STORJ_METAINFO_RATE_LIMITER_ENABLED=false STORJ_METAINFO_RS=1/1/1/1-256B STORJ_VERSION_RUN_MODE=disable
    STORJ_ORDERS_ENCRYPTION_KEYS=0100000000000000=0100000000000000000000000000000000000000000000000000000000000000
    # A fresh dev network has only new, unvetted nodes, which the default selection would skip for minutes.
    STORJ_OVERLAY_NODE_MINIMUM_DISK_SPACE=500MB STORJ_OVERLAY_NODE_NEW_NODE_FRACTION=1 STORJ_OVERLAY_NODE_SELECTION_CACHE_STALENESS=5s)
  # Both: no version server, tracing or telemetry leaves the test network. The node refuses to
  # start with its version checker disabled, so it is pointed at a closed port instead.
  local both=(STORJ_DEFAULTS=dev STORJ_LOG_LEVEL=info STORJ_TRACING_ENABLED=false STORJ_METRICS_ADDR=
    STORJ_VERSION_SERVER_ADDRESS=http://127.0.0.1:1 STORJ_SERVER_EXTENSIONS_REVOCATION=false STORJ_SERVER_USE_PEER_CA_WHITELIST=false)
  local e kv
  envs() { e=(); for kv in "$@"; do e+=(-e "$kv"); done; }

  log "Storj: Postgres, Redis, a satellite and one storage node"
  # tmpfs over the images' VOLUMEs, so no anonymous volume outlives the run.
  docker run -d --name "$P-storj-pg" --network "$NET" --network-alias postgres --tmpfs /var/lib/postgresql \
    -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_DB=master "$STORJ_PG_IMAGE" >/dev/null || die "storj: postgres failed to start"
  docker run -d --name "$P-storj-redis" --network "$NET" --network-alias redis --tmpfs /data "$STORJ_REDIS_IMAGE" >/dev/null \
    || die "storj: redis failed to start"
  envs "${both[@]}" "${sat[@]}"
  docker run -d --name "$P-storj-sat" --network "$NET" --network-alias satellite-api "${e[@]}" "$STORJ_IMAGE" \
    satellite run api --defaults=dev --identity-dir=/storj/.local/share/storj/identity/satellite-api/ >/dev/null \
    || die "storj: the satellite failed to start"
  envs "${both[@]}" "${sn[@]}"
  docker run -d --name "$P-storj-sn1" --network "$NET" --network-alias storagenode1 "${e[@]}" "$STORJ_IMAGE" \
    storagenode run api --defaults=dev --identity-dir=/storj/.local/share/storj/identity/storagenode/ >/dev/null \
    || die "storj: the storage node failed to start"

  # storj-up registers a user through the satellite's console, then makes a project and its API key.
  STORJ_KEY="$(docker run --rm --name "$P-storj-creds" --network "$NET" --entrypoint storj-up "$STORJ_IMAGE" \
    credentials -e -r 180 -s satellite-api:7777 -c satellite-api:10000 2>/dev/null | sed -n 's/^export STORJ_API_KEY=//p')"
  [ -n "$STORJ_KEY" ] || { docker logs "$P-storj-sat" 2>&1 | tail -n 20 >&2; die "storj: no API key from the satellite"; }

  FIELDS=(TYPE=storj "STORJ_SATELLITE=$STORJ_SAT" "STORJ_BUCKET=$BUCKET" "STORJ_PATH=%D")
  SECRETS=("STORJ_KEY=$STORJ_KEY" "STORJ_PASSPHRASE=$STORJ_PASS") PREP=storj_ready
  run_case storj "$BUCKET/%D" TYPE=storj PROVIDER=new "SATELLITE_ADDRESS=$STORJ_SAT" "API_KEY=$STORJ_KEY" "PASSPHRASE=$STORJ_PASS"
}
