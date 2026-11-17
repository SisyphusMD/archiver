# Contributing to Archiver

Open issues and pull requests on [Forgejo][forge]. Sign in with GitHub and wait for account
activation. After activation, clone the Forgejo repository and send a pull request with AGit:

```sh
git push origin HEAD:refs/for/main -o topic=<short-name>
```

Use a Forgejo access token as the HTTPS password. Push again with the same topic to update
the pull request. GitHub serves a read-only mirror.

Read the [SisyphusMD project rules][project-rules] before contributing. Every pull request closes
an issue. Rule 9 excludes AI attribution, including a `Co-Authored-By` trailer for an assistant.
Report vulnerabilities privately as described in [SECURITY.md](SECURITY.md).

## Tests

Run `go test -race ./cmd/... ./internal/...` and `go vet ./cmd/... ./internal/...`.
For the shell helpers, run `bats tests/next_version.bats`. The Docker CI workflow defines the
image build, installed smoke tests and cross-version backup and restore suites. Use disposable
storage and keys for testing.

Rule 13's safety-critical paths include `internal/pipeline`, `internal/restore`,
`internal/copier`, `internal/maintenance`, `internal/localprune` and `internal/kit`. Test
regressions against observed behavior, such as restoring an existing snapshot or verifying which
revisions a prune leaves available. Put the failing and passing evidence on the pull request.

## Required checks

The standard contexts are `CI / Issue link (pull_request_target)`,
`CI / Tests with code (pull_request_target)`, `CI / Diff size (pull_request_target)`,
`CI / Lint (pull_request_target)`, `CI / Secrets (pull_request_target)` and `AI review`.
Each new gate needs a passing and a failing run before it is added to branch protection.

The existing workflow reports these required contexts:

- `CI / Build / Publish / Test pipelines never end in grep -q (pull_request)`
- `CI / Build / Publish / Dockerfile download integrity (pull_request)`
- `CI / Build / Publish / Shellcheck (pull_request)`
- `CI / Build / Publish / Validate CHANGELOG structure (pull_request)`
- `CI / Build / Publish / Bats unit tests (pull_request)`
- `CI / Build / Publish / Go vet, format, and unit tests (pull_request)`
- `CI / Build / Publish / Fuzz the parsers (30 s each, ADR 39) (pull_request)`
- `CI / Build / Publish / PR build & smoke test (pull_request)`
- `CI / Build / Publish / Slim image (ADR 41) (pull_request)`
- `CI / Build / Publish / Round-trip + scheduler smoke (pull_request)`
- `CI / Build / Publish / Black-box suite (never-break across versions) (pull_request)`
- `CI / Build / Publish / Host-driven integration (entrypoint + storage sidecars) (pull_request)`

The workflow also runs `CI / Build / Publish / Check pull-request job trust gates (pull_request)`.

AGit and fork pull requests receive the standard gates, shellcheck and changelog validation.
A maintainer reviews the change before opening it from a same-repository branch for the full
suite.

## Break-glass

Rule 20 covers outages. For an urgent merge with Forgejo available, the maintainer can
remove the unavailable required context, restore it immediately after the merge, and record
the exception on the issue.

[forge]: https://forgejo.bryantserver.com/SisyphusMD/archiver
[project-rules]: https://forgejo.bryantserver.com/SisyphusMD/project-rules/src/branch/main/RULES.md
