# #351 Adopt the SisyphusMD project rules

- Kind: pull, state: closed, by cody, created 2026-11-17T14:09:35-08:00
- Labels: none; milestone: none
- ci/adopt-project-rules -> main, merged: True

Closes #350

Add the shared Forgejo checks, templates, issue archive and private security reporting. Keep the existing Docker CI and trust guard. Record only the verified historical scanner fingerprints.

Large diff: the shared workflows, scripts, templates and rules are copied verbatim from project-rules.

Validated locally with The AGit and fork trust guard, capped Go race tests and vet, Dockerfile and pipeline linters, and the pinned history scan.

GitHub mirror lockdown follows after checking the mirror token's permission to write workflow files. Branch protection is tracked on the project-rules adoption issue.
