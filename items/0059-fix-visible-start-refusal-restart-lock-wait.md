# #59 fix: visible start refusal + restart lock-wait

- Kind: pull, state: closed, by cody, created 2026-07-10T22:59:37-07:00
- Labels: none; milestone: none
- refs/pull/59/head -> main, merged: True

Owner-found: `archiver start` while a backup is running printed the success line and exited 0 — the refusal happens inside the backgrounded main.sh whose output is discarded. Sibling bug: `restart` raced the asynchronously-stopping backup and could stop-but-never-start, silently. Fix: the dispatcher pre-checks the lock (visible refusal, non-zero exit; stale locks still start) and restart waits up to 120s for lock release with a clear failure message if cleanup outlasts it. lockfile.sh no longer sources logging (only log_lockfile_summary needed it and its callers provide it), making it dependency-free so the dispatcher shares is_lock_valid/get_lock_pid instead of duplicating the check — sourcing the logging chain in the dispatcher mutates the environment the backgrounded child inherits (config-loader arrays SERVICE_DIRECTORIES, arrays do not export). lock-concurrency test extended: async-start refusal (exit code + message + no false success line) and restart stop-wait-start. NOT for immediate release — rides the next patch.
