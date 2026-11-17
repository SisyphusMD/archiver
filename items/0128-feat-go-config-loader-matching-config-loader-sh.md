# #128 feat: Go config loader matching config-loader.sh

- Kind: pull, state: closed, by cody, created 2026-10-01T06:54:08-07:00
- Labels: none; milestone: none
- refs/pull/128/head -> main, merged: True

Foundation for the Go backup pipeline (ADR 10 step 2). Not wired in yet; no behavior change. Storage names, env prefixes, URLs and glob expansion are pinned against bash: a shared tests/fixtures/storage-names.tsv checked by bats and Go, and glob cases verified against bash in the image.
