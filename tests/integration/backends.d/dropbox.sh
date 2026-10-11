# Dropbox through the user's own app (tests/fakes/dropbox.go): the fake knows one app and its
# refresh token, refreshes only with the app's key and secret in Basic auth at Dropbox's own
# token endpoint, and answers only calls bearing a token it issued, so the round trip proves the
# app's credentials reach the refresh and never go by way of duplicacy.com.
CASES+=(dropbox)
FAKE_HOSTS+=(api.dropboxapi.com content.dropboxapi.com)
DROPBOX_KEY=archiver-test-app-key DROPBOX_SECRET=archiver-test-app-secret DROPBOX_REFRESH=archiver-test-refresh-token

case_dropbox() {
  FIELDS=(TYPE=dropbox "DROPBOX_PATH=archiver/%D" "DROPBOX_APP_KEY=$DROPBOX_KEY")
  SECRETS=("DROPBOX_APP_SECRET=$DROPBOX_SECRET" "DROPBOX_TOKEN=$DROPBOX_REFRESH") PREP=""
  run_case dropbox archiver/%D TYPE=dropbox "CLIENT_ID=$DROPBOX_KEY" "CLIENT_SECRET=$DROPBOX_SECRET" \
    "TOKEN={\"refresh_token\":\"$DROPBOX_REFRESH\",\"expiry\":\"2000-01-01T00:00:00Z\"}"
}
