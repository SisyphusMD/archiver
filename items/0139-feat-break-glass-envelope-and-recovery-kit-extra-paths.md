# #139 feat: break-glass envelope and recovery-kit extra paths

- Kind: pull, state: closed, by cody, created 2026-10-04T16:51:24-07:00
- Labels: none; milestone: none
- refs/pull/139/head -> main, merged: True

Per cody-3b DR audit and Cody decisions (2026-10-04): `archiver envelope [DIR]` writes a one-page PDF (drawn directly, qrencode only) and HTML with the recovery password, decrypt command, and per storage the kit location and a credential that reads it (STORAGE_TARGET_N_BREAKGLASS_* when set, else the backup credential marked FULL ACCESS; SFTP break-glass key + user). `archiver envelope confirm` records the written page; each kit run refreshes what the page would say, and status/healthcheck flag OUT OF DATE or a yearly check due, notifying once. RECOVERY_KIT_EXTRA_PATHS copies runbooks/helpers/git bundles into the kit under extra/ (modes and spaced names count for change detection). Backport to release/0.11 follows as 0.11.5.
