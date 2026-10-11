# Google Drive (tests/fakes/drive.go). www.googleapis.com and oauth2.googleapis.com are
# already held by the front for Cloud Storage, which sends Drive's paths to the fakes and
# grants any token request, so no host is added here.
CASES+=(gcd)

# gcd_token NAME N: the primary signs in with a service account, the copy with a token file
# from an OAuth app of the user's own in the form docs/storage.md gives (Duplicacy reads the
# endpoint's Go field names), expired so both clients refresh it at Google's endpoint first.
gcd_token() {
  docker exec "$P-$1" bash -c '
    set -e
    f=/run/secrets/storage_target_$1_gcd_token
    if [ "$1" = 1 ]; then
      openssl genrsa -out /tmp/sa.pem 2048 2>/dev/null
      key=$(awk "{printf \"%s\\\\n\", \$0}" /tmp/sa.pem)
      printf "{\"type\":\"service_account\",\"project_id\":\"archiver-test\",\"private_key_id\":\"1\",\"private_key\":\"%s\",\"client_email\":\"archiver@archiver-test.iam.gserviceaccount.com\",\"client_id\":\"1\",\"token_uri\":\"https://oauth2.googleapis.com/token\"}" "$key" > "$f"
      for _ in $(seq 1 30); do curl -fsS -o /dev/null -H "Authorization: Bearer probe" https://www.googleapis.com/drive/v3/about && exit 0; sleep 1; done
      exit 1
    fi
    cat > "$f" <<JSON
{"client_id":"archiver-test.apps.googleusercontent.com","client_secret":"gcd-client-secret",
 "end_point":{"TokenURL":"https://oauth2.googleapis.com/token"},
 "token":{"access_token":"gcd-expired","token_type":"Bearer","refresh_token":"gcd-refresh","expiry":"2020-01-01T00:00:00Z"}}
JSON
  ' _ "$2"
}

case_gcd() {
  FIELDS=(TYPE=gcd "GCD_PATH=gcd/%D") SECRETS=() PREP=gcd_token
  run_case gcd gcd/%D TYPE=drive SERVICE_ACCOUNT_FILE=/run/secrets/storage_target_1_gcd_token

  # A shared drive by the ID its URL shows, which both Duplicacy (build/duplicacy/gcd-drive-id.patch)
  # and rclone's TEAM_DRIVE (the kit, the probe) take.
  log "gcd: again in a shared drive, made as its owner would"
  local drive
  drive=$(docker run --rm --network "$NET" -v "$CFG":/cfg:ro --entrypoint curl "$IMAGE" -fsS --cacert /cfg/public.crt \
    -H "Authorization: Bearer setup" -H "Content-Type: application/json" -d '{"name":"Archiver Backups"}' \
    "https://www.googleapis.com/drive/v3/drives?requestId=archiver-test-$$" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
  [ -n "$drive" ] || die "gcd: the shared drive was not created"
  FIELDS=(TYPE=gcd "GCD_DRIVE=$drive" "GCD_PATH=gcd/%D")
  run_case gcd gcd/%D TYPE=drive SERVICE_ACCOUNT_FILE=/run/secrets/storage_target_1_gcd_token "TEAM_DRIVE=$drive"
}
