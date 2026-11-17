# #137 fix: the archiver dispatcher exits with the command status (port from release/0.11)

- Kind: pull, state: closed, by cody, created 2026-10-03T20:32:28-07:00
- Labels: none; milestone: none
- refs/pull/137/head -> main, merged: True

A failed `archiver restore`, `migrate`, `stop`, `pause` or `resume` exited 0 because archiver.sh ended with `exit 0`. New dispatch-exit-codes integration test fails on the current main image. Test pipelines that piped these commands into `grep -q` under pipefail now capture first (the writer SIGPIPE status would otherwise fail them).
