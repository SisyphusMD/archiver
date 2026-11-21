# #374 Point the 0.11 publish workflow at the live mirror names

- Kind: issue, state: closed, by cody, created 2026-11-20T18:06:01-08:00
- Labels: none; milestone: none

The v0.11.8 tag publish (run 41501, job 204023) failed at "Push to cluster Forgejo via skopeo": release/0.11 docker-publish.yml still pulls skopeo from image-mirror.nas.bryantserver.com and mirrors to forgejo.nas.bryantserver.com, names retired on 2026-10-10 (main fixed this in ead5944). The pinned skopeo v1.22.3 digest is also gone from quay and the .lan mirror, so a plain rename is not enough.

Fix on release/0.11: use the .lan names and main's pinned skopeo v1.24.1 digest. No image is published for v0.11.8; the tag is already on both mirrors, so it is not moved.

## cody, 2026-11-20T18:21:05-08:00

Decision: Yes: merge archiver#375 and cut v0.11.9 (same image contents as the unpublished v0.11.8) through the trusted-main Prepare/Publish Release path. Why: the v0.11.8 tag is already mirrored and must not move, and its publish workflow cannot run.

Decided by: Cody, 2026-11-20.
