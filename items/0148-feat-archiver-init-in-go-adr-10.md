# #148 feat: archiver init in Go (ADR 10)

- Kind: pull, state: closed, by cody, created 2026-10-05T15:15:18-07:00
- Labels: none; milestone: none
- refs/pull/148/head -> main, merged: True

ADR 10 step 3, init: `archiver init` runs in Go (`internal/setup`); `lib/scripts/init.sh` and the dispatcher's init routing are gone.

- Same questions and env-native materials (archiver.env via `config.NewSettings`, the serializer the kit uses; secrets/ with keys). The storage-type question is built from `config.StorageTypes`.
- The RSA key is still `openssl genrsa -aes256 -traditional` (PKCS#1, encrypted; never-break), with the passphrase on fd 3 instead of typed through `expect`, which leaves the image.
- B2 key and S3 secret prompts hide input on a terminal and restore echo on Ctrl-C (checked under a pty).
- Input ending mid-setup writes nothing; a key that cannot be set aside is never generated over.
- y/N answers are read as lines.

Local: unit tests (also as root in the golang image), init-envnative.
