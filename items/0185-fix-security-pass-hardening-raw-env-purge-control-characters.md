# #185 fix: security-pass hardening (raw env purge, control characters, token copies, kit extras, envelope argv)

- Kind: pull, state: closed, by cody, created 2026-10-08T16:39:23-07:00
- Labels: none; milestone: none
- refs/pull/185/head -> main, merged: True

Early security pass (ADR 42), the five small items Cody approved, one commit each with a unit test: raw secret env vars unset at start (names carried in ARCHIVER_PURGED_SECRETS so child archivers still warn); storage settings with control characters refused (SFTP_PATH sftp-batch injection); token copies written as new 0600 files renamed into place; RECOVERY_KIT_EXTRA_PATHS copies regular files/dirs only, skips cycles; envelope fetch commands use RCLONE_CONFIG_KIT_* instead of credentials in argv (backends.sh updated to grep the new form).
