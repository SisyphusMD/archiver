# #195 feat: archiver doctor (ADR 30)

- Kind: pull, state: closed, by cody, created 2026-10-09T11:43:05-07:00
- Labels: none; milestone: none
- refs/pull/195/head -> main, merged: True

archiver doctor [--notify]: one read-only pass. Configuration and secrets (Validate, schedules via daemon.Jobs, RSA private key decrypts with the passphrase over fd 3 and matches public.pem, kit password, notifier destinations; --notify sends a test to each). Storages: kit.HasConfig through rclone (never creates), then a scratch repository attached by writing its preferences (no duplicacy init, so a vanished storage is never created), list -a under the in-use gate with the newest revisions held, and the RSA key proven by duplicacy cat of the smallest non-empty file up to 64 MB (KeyError / KeyUnchecked). Container: capabilities, logs on a mount (from mountinfo), free space, service directories, invalid snapshot IDs, legacy settings files, hook safety incl. restore-service.sh. Freshness: newest revision per service per storage vs 2x BACKUP_SCHEDULE, secondaries behind the primary, copy workers, the kit (recorded and actually present; vanished -> recovery-kit force), envelope vs the current configuration, drills (health rules). tests/integration/doctor.sh. 11 Codex review rounds; full integration suite + e2e pass locally.
