# #150 test(e2e): cross-version upgrade with inherited HOSTNAME and sanitized storage names

- Kind: pull, state: closed, by cody, created 2026-10-05T16:23:07-07:00
- Labels: none; milestone: none
- refs/pull/150/head -> main, merged: True

Closes the never-break list's remaining cases before ADR 10 step 4 deletes the bash: a second installation whose snapshot host comes from an inherited HOSTNAME and whose storage names (`Primary-1`, `off-site 2`) Duplicacy needs sanitized, written by 0.11.0 and continued, restored and rolled back across versions. Remote storage URLs are pinned by `TestURL` (the same strings the bats tests pin), and the s3/sftp integration tests run against real servers.

Finding: 0.11.0 named its kit after the kernel hostname even with HOSTNAME set; later releases use the snapshot host. The old kit still opens; the test expects each version's name.
