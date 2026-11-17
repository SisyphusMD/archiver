# #272 fix(duplicacy): retry chunk downloads the network cut off

- Kind: pull, state: closed, by cody, created 2026-10-21T11:19:53-07:00
- Labels: none; milestone: none
- refs/pull/272/head -> main, merged: True

GARAGE drill run 5 failed 22h and ~98% into its B2 restore on one `connection reset by peer`: Duplicacy 3.2.5 retries a chunk download only on io.ErrUnexpectedEOF. New patch `build/duplicacy/download-retry-network.patch` retries reset/aborted/broken-pipe/timeout up to 8 times with a 2s doubling backoff capped at 1m.

Test-first: `TestChunkDownloadRetriesNetworkErrors` drives a real download through a storage that cuts off N times; it fails on the image without the retry hunk (reset and timeout not retried) and passes with it. A non-network error is still not retried; a link that never comes back gives up after 9 downloads. Codex review: no findings.
