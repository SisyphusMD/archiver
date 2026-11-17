# #298 fix(status): a secondary check waits for the catch-up, not OVERDUE

- Kind: pull, state: closed, by cody, created 2026-10-29T04:56:07-07:00
- Labels: none; milestone: none
- refs/pull/298/head -> main, merged: True

A copy worker checks only when idle, so during a multi-day catch-up (the NAS backblaze/cowanserver re-carry) status said check OVERDUE as if something were broken. While the worker is copying it now says the check waits for the catch-up; idle workers still show OVERDUE, and a failed check still shows LAST CHECK FAILED.
