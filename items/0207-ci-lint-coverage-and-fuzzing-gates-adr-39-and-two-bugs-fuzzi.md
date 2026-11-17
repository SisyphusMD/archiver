# #207 ci: lint, coverage and fuzzing gates (ADR 39), and two bugs fuzzing found

- Kind: pull, state: closed, by cody, created 2026-10-09T20:51:12-07:00
- Labels: none; milestone: none
- refs/pull/207/head -> main, merged: True

Stacked on #206 (status page), itself on #205: merge those first.

1. fix(config), found by the new fuzz targets on their first run: a SERVICE_DIRECTORIES class `[[:]` panicked (overlapping [: and :]); a day interval past ~106,000 days wrapped negative and was accepted. Both are seeds now (regression tests).
2. ci (ADR 39):
   - golangci-lint v2.14.0 (pinned, Renovate-tracked) with staticcheck, errcheck, ineffassign, unused, gosec; enforced. .golangci.yml: std-error-handling and common-false-positives presets; tests excluded from errcheck/gosec; gosec G104 (dup of errcheck) and G204 (argv, no shell, inherent to a wrapper) off; staticcheck ST1005 off (errors are user-facing sentences) and doc-comment style checks off.
   - Existing findings fixed once: ~100 errcheck sites made explicit (`_ =` where best-effort is deliberate), real handling where it mattered (init now fails if it cannot make the private keys and archiver.env owner-only; maintenance warns when its state cannot be written), 52 gosec sites annotated per line with their reason, unused code removed, receivers made consistent.
   - go test -cover (per-package coverage, no floor).
   - Fuzz targets: config (split, interval, glob match, Sanitize stability, whole Load+Validate+URL), maintenance preferences parsing, recovery archiver.env reading. tests/fuzz.sh discovers targets, 30 s each per change (job `fuzz`, now required by build-and-push), 10 min each weekly (fuzz-weekly.yml, advisory); a crash prints its input.
   - .renovaterc.json includePaths gains real-backends.yml (was missing) and fuzz-weekly.yml.

0 lint issues; unit tests pass. 5 Codex rounds, last clean.
