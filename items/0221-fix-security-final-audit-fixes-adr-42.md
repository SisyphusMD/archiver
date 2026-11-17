# #221 fix(security): final audit fixes (ADR 42)

- Kind: pull, state: closed, by cody, created 2026-10-10T09:26:23-07:00
- Labels: none; milestone: none
- refs/pull/221/head -> main, merged: True

The final security pass (ADR 42) at 0096a9d: Codex audit + manual review (the Claude Security scan was not run: its cost confirmation went unanswered). Cody delegated the fixes ("fix it securely without worsening the user experience or creating too many compromises").

**fcc23c8 logs-volume state is untrusted** (a less-trusted writer of the logs volume must not gain root):
- H1: an interrupted backup's catch-up acts only on configured service directories, snapshot IDs derived (validated) as a backup derives them; the record's `snapshots` field is gone (a `../` ID made it RemoveAll any directory; an unconfigured dir's post-backup ran as root).
- H5: drill leftover cleanup judged against `RESTORE_DRILL_DIR`, not the record's root (field gone).
- H4: `internal/atomicfile` (random `CreateTemp` name, sync, rename, dir sync) for run records, worker/backup/drill/maintenance state, envelope records, runlock, maintenance repo files; the Duplicacy resume patch's writer uses `ioutil.TempFile` likewise.
- H3: the status page opens logs through `os.OpenRoot(logs)` (the archiver.log -> prior_logs link still works; nothing outside), non-blocking, regular files only, 512 KiB cap.
- M7: a future-dated send claim no longer holds back alerts.
Tests: atomicfile planted-link test, web tail test (secret link, swapped prior_logs, FIFO, dir), claim test, fault-interrupt forged backup + drill records.

**4915b65 hooks around restores**
- H2: `hooks.Migrate` applies the ADR 45 rule to the settings file before sourcing it; automatic migration after a restore does nothing with HOOKS_DIR set (warns); a generated hook is refused when the settings file it sources (from the service dir, also under HOOKS_DIR) could be changed by others, and only hooks that source it are checked.
- 9: the restore hook gets Pdeathsig SIGKILL (thread-locked), so it dies with a killed Archiver instead of running on without the restore lock; detached children run on as before (why the lock is not inherited). Test: kill -9 mid-hook with a TERM-ignoring hook (fails without the fix; /proc check works on slim).

**e395c87 CI tokens off command lines** (M8 + same pattern found in trivy, curl -H, git -c): `.forgejo/scripts/registry-auth.sh` writes a skopeo auth file into a volume from env via builtins/stdin; skopeo uses --src/--dest-authfile, list-tags --authfile; trivy gets env names; reconcile curl uses `-H @file`; release git push uses GIT_CONFIG_* env. Verified locally: skopeo v1.22.3 parses the file (login --get-login), flags exist; shellcheck clean. scan-weekly gains a checkout.

**813319b docs**: CHANGELOG; docs/restore.md states the one deferred limit (H6: a concurrent writer to a restore destination can race the link check; full fix needs descriptor-based restore in Duplicacy).

Validation: unit -race, golangci-lint, integration (fault-interrupt, restore-commands full+slim, web, restore-drill, recover, roundtrip, migrate-hooks, hook-safety, incidents), Duplicacy patch tests, e2e passed. Codex: diff reviews clean; a cold verify of the fixes found the HOOKS_DIR legacy-source gap and TERM-ignoring hooks (both fixed).
