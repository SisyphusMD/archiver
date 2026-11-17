# #129 fix: keep the Pushover credentials off argv

- Kind: pull, state: closed, by cody, created 2026-10-01T07:05:08-07:00
- Labels: none; milestone: none
- refs/pull/129/head -> main, merged: True

Contract violation (secrets never on argv): send_pushover_notification passed the token and user key as curl --form-string arguments. The e2e secrets-hidden test now runs with a notifier configured and fails on 0.11.2; the fix feeds them to curl via -K - on stdin. To be cherry-picked to release/0.11 for 0.11.3.
