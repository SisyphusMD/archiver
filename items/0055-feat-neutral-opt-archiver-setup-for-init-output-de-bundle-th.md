# #55 feat: neutral /opt/archiver/setup for init output (de-bundle the primary flow)

- Kind: pull, state: closed, by cody, created 2026-07-10T14:09:31-07:00
- Labels: none; milestone: none
- refs/pull/55/head -> main, merged: True

Owner feedback: the init flow was still branded by the bundle — the documented mount was `/opt/archiver/bundle` even though the bundle is now the transitional second-class artifact. init (and the entrypoint init mode) now write all output — `env-native/` materials plus the escrow `bundle.tar.enc` — to a neutral `/opt/archiver/setup` directory; `/opt/archiver/bundle` remains only the RUNTIME mount for bundle-mode deployments. init also stops claiming "you need this bundle file and its password to run Archiver" (false in the env-native model; it is the DR escrow). Full self-directed sweep of every user-facing surface included: compose "(required)" comment scoped to bundle mode, local-storage-setup guide paths made consistent, and every remaining bundle reference verified to be either the breaking-change/upgrade doc, layered-config semantics, migrate/export/import feature docs, bundle-mode runtime messages, or a labeled alternative.

Note: main's README now documents the new init mount, which the published 0.9.0 image does not honor — this should be followed promptly by a 0.9.1 patch release (also carrying the download-retry hardening and init wording fixes already on main).

Verified: real-entrypoint init drops both artifacts on the setup mount; init-envnative + entrypoint-secrets + bundle-secret + migrate + env-native + roundtrip + both smokes pass; bats 68/68.
