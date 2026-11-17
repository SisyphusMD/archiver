# #180 docs(adr): phase 7 gate (ADRs 39-41)

- Kind: pull, state: closed, by cody, created 2026-10-08T12:54:17-07:00
- Labels: none; milestone: none
- refs/pull/180/head -> main, merged: True

Cody's phase 7 decisions (2026-10-08), text approved: 39 lint enforced, coverage advisory, fuzz short-enforced and weekly-advisory; 40 SBOM and provenance always, release signing enforced, trivy advisory; 41 full image plus a -slim tag.

After approval, review corrected ADR 41's slim list to keep everything Archiver itself runs (tini, qrencode were missing). The decision is unchanged.
