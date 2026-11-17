# #218 docs: guides under docs/, README as overview and quick start (ADR 43)

- Kind: pull, state: closed, by cody, created 2026-10-10T05:04:32-07:00
- Labels: none; milestone: none
- refs/pull/218/head -> main, merged: True

Stacked on #217: merge that first.

- README: what Archiver is, features, a mermaid flow diagram, a three-step quick start, and a guide index.
- docs/: installation, configuring, storage, hooks, restore, recovery (kit, envelope, recover), maintenance (check/prune, doctor, drills), monitoring, commands (incl. external schedulers), upgrading (new "Upgrading to 1.0" checklist from the CHANGELOG's removed/changed entries, plus the bundle and pre-0.7 paths). Content moved from the README nearly verbatim (scripted split by heading, levels renormalized, links rebased and anchors redirected).
- Dropped as superseded or wrong: the partial env-var table (the generated reference covers it), a duplicate image-tag list, the "Secrets" example that showed secrets as env vars. The settings/secrets enumerations in Configuration Sources now defer to the reference.
- References to README sections updated: compose.yaml and archiver.env.example comments, entrypoint refusal messages, the kit's README.txt pointer, older guides, unreleased CHANGELOG entries. A pre-existing broken link in local-storage-setup.md fixed.
- internal/docs/links_test.go: every relative link in README.md and docs/ must name an existing file, and every anchor a heading there (negative run checked).
- release.yml bumps the image tag in every docs/*.md but the ADRs, not a fixed list.

Codex review clean.
