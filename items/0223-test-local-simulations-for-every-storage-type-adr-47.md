# #223 test: local simulations for every storage type (ADR 47)

- Kind: pull, state: closed, by cody, created 2026-10-10T21:00:01-07:00
- Labels: none; milestone: none
- refs/pull/223/head -> main, merged: True

Every storage type is now tested against a local simulation, offline, with no accounts (ADR 47, superseding 25 and 27). The real-service job, its release gate and skip_real_backends are removed.

- tests/fakes: one stdlib Go server behind the services' own hostnames: B2 (b2, b2-custom), Dropbox, File Fabric, Google Drive (My Drive and a shared drive), OneDrive (one; odb on its own drive and a library), and Wasabi's MOVE carried out on Garage. A local Storj network from Storj's own images.
- Each fake checks what the real service would: B2's hosts per call, Dropbox's and OneDrive's exact app credentials, issued tokens only; duplicacy.com is held and refused.
- Fixes the simulations found: a Google Drive shared drive by ID (new Duplicacy patch, gcd-drive-id.patch, with its test), a misleading missing-directory diagnosis, the Storj satellite prompt.
- Full backends.sh run locally: all 19 types OK as primary and as copy.
