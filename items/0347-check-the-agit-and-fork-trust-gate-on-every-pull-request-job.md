# #347 Check the AGit and fork trust gate on every pull_request job

- Kind: issue, state: closed, by cody, created 2026-11-17T13:11:51-08:00
- Labels: status:in-progress; milestone: none

Follow-up to the AGit gate (`github.event.pull_request.flow == 0` beside the head-repository check) and SisyphusMD/project-rules#79.

Nothing checks the gate. Add a check, run in CI, that walks every job of every `pull_request`-triggered workflow under `.forgejo/workflows/` and fails unless its `if:` carries both the head-repository clause and `flow == 0`, with named exemptions and a reason for each. A job that loses its whole `if:` must fail it.

This repo has no `CONTRIBUTING.md` yet; the note that AGit and fork pull requests get no CI arrives with the project-rules adoption (SisyphusMD/project-rules#74).

`docker-publish.yml` has five jobs without the gate: `shellcheck` and `validate-changelog` (they read files and never execute the PR's code) and `build-and-push`, `create-release` and `summary` (tag pushes only). Each becomes a named exemption with its reason.

## cody, 2026-11-17T13:24:53-08:00

Status: not started beyond the issue. Plan as written, with the five named exemptions. Handed off.

## cody, 2026-11-17T13:29:50-08:00

Taking over the guard check from the release follow-up handoff. The adoption PR remains separate.

## cody, 2026-11-17T14:01:18-08:00

Merged in #348 at 60909b6. All required checks and the new guard passed on the reviewed head. Mutation checks rejected missing or weakened PR trust conditions and changes to a tag-only exemption.
