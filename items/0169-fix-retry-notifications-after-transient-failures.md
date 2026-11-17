# #169 fix: retry notifications after transient failures

- Kind: pull, state: closed, by cody, created 2026-10-07T22:11:28-07:00
- Labels: none; milestone: none
- refs/pull/169/head -> main, merged: True

A notification made one attempt, so a single network error, rate limit or Pushover server error lost it, and the log blamed the secrets. Found as the black-box flake in #166: `TestRunsRefusedBackupNotifies` failed because the fake Pushover TLS handshake outlasted 10s on a busy runner.

Now: transient failures (network errors, 429, 5xx) are retried after 5s and 10s, with a 15s attempt timeout, so a send ends within about a minute. A refusal such as bad credentials is not retried and still says to check the Pushover secrets.

Unit tests cover retry-then-success, exhausted retries, a dropped connection and no retry on 400.
