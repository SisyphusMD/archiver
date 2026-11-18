# #352 Release through a merged pull request

- Kind: issue, state: closed, by cody, created 2026-11-17T14:18:45-08:00
- Labels: status:blocked, type:feature; milestone: none

Part of SisyphusMD/project-rules#86. Move direct changelog promotion pushes to release pull requests and tag the exact merged commit by maintainer dispatch. Preserve alpha and release-candidate ordering on main and the patch channel on release branches. Prove the channel-specific version policy before requiring protection.

Depends on SisyphusMD/project-rules#97.

## cody, 2026-11-17T18:23:04-08:00

Codex is preparing a two-stage release flow that keeps main alpha/RC ordering and maintenance-branch patches. Plan: dispatch trusted workflows from main, qualify and open a PR into the selected channel, then verify and tag the exact merged commit. Preserve the existing Docker smoke and tag-triggered publishing.

## cody, 2026-11-17T20:46:44-08:00

Archiver #355 settled at 59a6562741623a8939519b5ea11faa9a24f45628. All exact native, guard and shared statuses passed, both mirrors match, and its task branch was pruned. Publish Release #756 rejected the ordinary merged PR before tagging. The maintenance PR #357 remains a draft while a separately approved real maintenance release proof is outstanding.
