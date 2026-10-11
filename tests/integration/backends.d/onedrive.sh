# OneDrive personal (one) and for Business (odb), through tests/fakes/onedrive.go behind
# Microsoft's hostnames: Graph, the older api.onedrive.com Duplicacy's personal storage calls,
# the token endpoint, and the pre-authenticated hosts uploads and downloads go to.
# shellcheck shell=bash disable=SC2034
CASES+=(one odb)
FAKE_HOSTS+=(login.microsoftonline.com graph.microsoft.com api.onedrive.com archivertest-my.sharepoint.com archivertest.files.1drv.com)

# The app registration tests/fakes/onedrive.go knows (odClientID, odClientSecret).
OD_CLIENT_ID="0a1b2c3d-0000-4000-8000-a2c41e7e5700"
OD_CLIENT_SECRET="archiver-test-client-secret"
OD_TOKEN_URL="https://login.microsoftonline.com/common/oauth2/v2.0/token"

# od_token REFRESH: a token file whose access token has expired, so every client refreshes it
# first. The fake grants only the access tokens it issued.
od_token() { printf '{"access_token":"expired","token_type":"Bearer","refresh_token":"%s","expiry":"2001-01-01T00:00:00Z"}' "$1"; }

# od_prep NAME N: the storage folder, which Duplicacy will not create, and the token file. Each
# container's refresh token is its own, so the fake's log shows how often it was exchanged.
od_prep() {
  local dir=store
  [ "$2" = 1 ] || dir=copy
  secret "$1" "storage_target_$2_${OD_TYPE}_token" "$(od_token "$OD_KIND-seed-$OD_TAG-$1-$2")" || return 1
  docker exec "$P-$1" bash -c '
    set -e
    for _ in $(seq 1 30); do
      tok=$(curl -fsS -d grant_type=refresh_token -d "refresh_token=$1-setup" -d "client_id=$2" -d "client_secret=$3" "$4" 2>/dev/null \
        | sed -n "s/.*\"access_token\":\"\([^\"]*\)\".*/\1/p") && [ -n "$tok" ] && break
      sleep 1
    done
    [ -n "$tok" ]
    prefix=""
    for part in ${6//\// }; do
      url="$5/root/children"
      [ -z "$prefix" ] || url="$5/root:/$prefix:/children"
      code=$(curl -sS -o /dev/null -w "%{http_code}" -X POST -H "Authorization: Bearer $tok" -H "Content-Type: application/json" \
        -d "{\"name\":\"$part\",\"folder\":{}}" "$url")
      [ "$code" = 201 ] || [ "$code" = 409 ]
      prefix="${prefix:+$prefix/}$part"
    done
  ' _ "$OD_KIND" "$OD_CLIENT_ID" "$OD_CLIENT_SECRET" "$OD_TOKEN_URL" "$OD_DRIVE_URL" "$OD_ROOT/$dir"
}

# od_rotated TYPE: each container exchanged its seeded refresh token exactly once: Duplicacy and
# the recovery kit wrote the refreshed token back and every later process used it.
od_rotated() {
  local n c logs
  logs="$(docker logs "$P-fakes" 2>&1)"
  for c in "$1:1" "$1-copy:2"; do
    n=$(grep -c "refresh token $OD_KIND-seed-$OD_TAG-${c%:*}-${c#*:} exchanged" <<<"$logs")
    [ "$n" = 1 ] || die "$1: the token seeded in ${c%:*} was exchanged $n times, not once: the refreshed token is not written back"
  done
  echo "=== $1 ($OD_TAG) wrote its refreshed tokens back ==="
}

# od_case TYPE KIND TAG ROOT DRIVE-ID DRIVE-TYPE [extra FIELDS...]
od_case() {
  local type="$1" up drive_url
  up="$(printf '%s' "$type" | tr '[:lower:]' '[:upper:]')"
  OD_TYPE="$type" OD_KIND="$2" OD_TAG="$3" OD_ROOT="$4"
  OD_DRIVE_URL="https://graph.microsoft.com/v1.0/drives/$5"
  FIELDS=("TYPE=$type" "${up}_PATH=$4/%D" "${up}_CLIENT_ID=$OD_CLIENT_ID" "${@:7}")
  SECRETS=("${up}_CLIENT_SECRET=$OD_CLIENT_SECRET") PREP=od_prep
  run_case "$type" "/$4/%D" TYPE=onedrive "CLIENT_ID=$OD_CLIENT_ID" "CLIENT_SECRET=$OD_CLIENT_SECRET" \
    "TOKEN=$(od_token "$2-check-$3")" "DRIVE_ID=$5" "DRIVE_TYPE=$6"
  od_rotated "$type"
}

case_one() { od_case one personal own one a1b2c3d4e5f60718 personal; }

# The user's own business drive, then a document library named by its drive ID.
case_odb() {
  od_case odb business own odb 'b!archivertest-own' business
  od_case odb business library odb 'b!archivertest-library' documentLibrary 'ODB_DRIVE_ID=b!archivertest-library'
}
