# #372 Release v0.11.9 from release/0.11 (first published build of the 0.11.8 changes)

- Kind: issue, state: closed, by cody, created 2026-11-20T17:47:37-08:00
- Labels: none; milestone: none

Cut v0.11.8 from release/0.11 (8fb87e1) carrying the Debian base refresh from #370 / #371, through Prepare Release and Publish Release from main. This is the first maintenance release on the new path, so it also validates hosted qualification before #356 retires the old writer.

Approved by Cody 2026-11-20 (SisyphusMD/project-rules#74).

## cody, 2026-11-20T18:16:13-08:00

Status: v0.11.8 tagged but NOT published. Prepare Release (run 849) qualified and opened #373; merged into release/0.11 at efe35ce (ancestry verified); Publish Release (run 851) created annotated tag v0.11.8, already on forgejo.lan and GitHub. The tag publish (run 41501) failed at the cluster skopeo push, before any image push: release/0.11 docker-publish.yml still uses the retired *.nas mirror names, and its skopeo v1.22.3 digest is gone upstream. Rerun failed the same way. ghcr has no 0.11.8; 0.11 and 0.11.7 still point at sha256:83723fb6. Fix: archiver#374 / PR #375 (.lan names + main's skopeo v1.24.1 pin). The tag is not moved (already mirrored). Proposed next: merge #375 and cut v0.11.9 with the same image contents; pending Cody. archiver#357 stays draft until a 0.11 publish succeeds.

## cody, 2026-11-20T18:52:22-08:00

Done: v0.11.9 published (digest sha256:5e6b551c on ghcr, cluster and NAS registries; release pages on all three hosts). v0.11.8 remains a tag without an image.
