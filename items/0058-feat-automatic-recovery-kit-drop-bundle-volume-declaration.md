# #58 feat: automatic recovery kit + drop bundle VOLUME declaration

- Kind: pull, state: closed, by cody, created 2026-07-10T20:15:53-07:00
- Labels: none; milestone: none
- refs/pull/58/head -> main, merged: True

Adds the automatic recovery kit: with the recovery_password secret present, each backup serializes the full effective configuration (settings, every secret, keys), generated RECREATE.txt recreation notes, and any deployment manifests mounted at /opt/archiver/deployment (compose bind / k8s ConfigMap / NixOS bind, captured verbatim), encrypts it (AES-256 pbkdf2), and uploads it as a plain file beside the duplicacy data on every storage target. Recovery = any one storage location + the single password, decrypted with stock openssl on any machine. init generates the password and displays it once; the compose template mounts the secret and its own compose.yaml, and consumes archiver.env via env_file. Re-uploaded only on content change; per-target state with automatic retry of failed targets; failures notify but never fail the backup.

Also removes /opt/archiver/bundle from the image VOLUME declaration: Compose carries the previous container mount over for image-declared volume paths on recreate, so converting a bundle-mode deployment to env-native with --force-recreate inherited the old bundle mount and crash-looped on the bundle-without-password fail-fast (hit live during the NAS conversion).

Testing: 74 bats (incl. B2 JSON client vs realistic API responses, fingerprint runtime-mutation immunity, ConfigMap-style capture); new recovery-kit integration suite (produce/decrypt/self-contained/manifest+notes/no-rewrite/update/force/prune-exhaustive-safe/multi-target partial-failure retry/same-password refusal); sftp + s3 wire suites extended with real kit upload/retrieve/decrypt; init-envnative extended (kit on by default, decryptable with the generated password); full gate run twice back-to-back, 18/18 green. The B2 native-API upload leg has no CI double (unit-tested parser only) and gets live verification on the NAS right after release.
