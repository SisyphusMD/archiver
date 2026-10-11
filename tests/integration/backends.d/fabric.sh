# shellcheck shell=bash
# File Fabric: tests/fakes/fabric.go behind a made-up appliance host (the endpoint is the
# user's own), the permanent token a test value.
CASES+=(fabric)
FAKE_HOSTS+=(fabric.test)
FABRIC_TOKEN_VALUE="fabric-permanent-test-token"

case_fabric() {
  # Duplicacy's File Fabric storage needs its directory to exist already.
  fabric_ready() { # NAME N
    [ "$2" = 1 ] || return 0
    docker exec -e RCLONE_CONFIG_FF_TYPE=filefabric -e RCLONE_CONFIG_FF_URL=https://fabric.test \
      -e RCLONE_CONFIG_FF_PERMANENT_TOKEN="$FABRIC_TOKEN_VALUE" "$P-$1" bash -c '
      for _ in $(seq 1 30); do rclone --config /dev/null mkdir FF:fabric/store 2>/dev/null && rclone --config /dev/null mkdir FF:fabric/copy && exit 0; sleep 1; done; exit 1'
  }
  FIELDS=(TYPE=fabric FABRIC_ENDPOINT=fabric.test "FABRIC_PATH=fabric/%D") SECRETS=("FABRIC_TOKEN=$FABRIC_TOKEN_VALUE") PREP=fabric_ready
  run_case fabric /fabric/%D TYPE=filefabric URL=https://fabric.test "PERMANENT_TOKEN=$FABRIC_TOKEN_VALUE"
}
