# #210 docs: generated configuration reference and JSON schema (ADRs 43, 44)

- Kind: pull, state: closed, by cody, created 2026-10-10T01:23:08-07:00
- Labels: none; milestone: none
- refs/pull/210/head -> main, merged: True

Stacked on #209 (slim) on #208 on #207: merge those first.

- internal/config/reference.go: `Reference`, one table of every setting (group, secret, default, values, kind, description, case folding, schema pattern), plus `TargetReference()` built from the storage types' fields (with the breakglass secret variants and their types listed).
- reference_render.go: `ReferenceMarkdown()` -> docs/configuration.md; `ReferenceSchema()` -> docs/archiver.schema.json (JSON Schema 2020-12; empty always allowed as unset; enums, case-insensitive patterns only where the loader folds case; ECMA-262 patterns matching ParseInterval, port 1-65535, counts >= 1; a secret set as a variable is flagged `not: {}` with its `_FILE` alternative).
- Tests: every name the loader's settings/secrets patterns accept and every literal Getenv/getenv in internal/ and cmd/ is documented (allowlist: HOME, PATH, kit step markers, refused old names); the two files equal what the table generates (`UPDATE_REFERENCE=1 go test ./internal/config` rewrites them).
- Checked: schema validates under Draft 2020-12 (good env passes, bad env flagged); all 137 patterns compile as JS regexes; interval/port patterns behave as the parser does.
- README links both. Codex: 5 rounds, last clean.
