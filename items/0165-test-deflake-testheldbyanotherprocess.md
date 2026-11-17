# #165 test: deflake TestHeldByAnotherProcess

- Kind: pull, state: closed, by cody, created 2026-10-06T16:07:26-07:00
- Labels: none; milestone: none
- refs/pull/165/head -> main, merged: True

The test slept 300ms and assumed flock(1) held the lock by then. On a busy runner it did not, so `Read` treated the file as a dead reader's and removed it (`got &{holdings:[]}`, seen on the Renovate golang-digest PR). It now polls a non-blocking flock until another process holds the lock (10s deadline). 20 runs under -race pass.
