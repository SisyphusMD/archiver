# #146 feat: recovery kit in Go (ADR 10)

- Kind: pull, state: closed, by cody, created 2026-10-05T14:08:49-07:00
- Labels: none; milestone: none
- refs/pull/146/head -> main, merged: True

ADR 10 step 3, recovery kit: `archiver recovery-kit [force]` and the backup's kit step (`archiver recovery-kit-step`, replacing the bash shim) run in Go (`internal/kit`, `config.Snapshot`).

- Same payload (archiver.env, secrets/, keys, RECREATE.txt, deployment/, extra/), same fingerprint and `.recovery-kit-state`: `tests/integration/kit-parity.sh` builds a kit with the bash code and then with Go, and requires the Go kit to find it current and, forced, to decrypt to identical files and modes. Deployments switching over re-upload nothing.
- Encryption stays `tar | openssl enc -aes-256-cbc -pbkdf2 -pass fd:3`, so kits decrypt with stock openssl (never-break).
- Local placement is native Go with the bash rules (stage, hash-verify, rename through a fresh inode, chmod only when the mode differs, unverified kept out of the state). SFTP, B2 and S3 go through `sftp` and `curl` with credentials on stdin; B2 now parses JSON properly and has a unit test against a fake API.
- The envelope check stays bash (`lib/scripts/envelope-check.sh`) until the envelope port.

Local: unit tests, kit-parity, recovery-kit, envelope, stop-go, copy-workers, s3-garage, sftp-runtime, full e2e suite.
