# #182 fix(ci): verify TLS when release jobs send tokens

- Kind: pull, state: closed, by cody, created 2026-10-08T16:23:22-07:00
- Labels: none; milestone: none
- refs/pull/182/head -> main, merged: True

Early security pass (ADR 42), scan findings F1-F4: the release reconcile curl calls used -k and the skopeo cluster push / GHCR mirror used tls-verify=false while sending repo-write and registry push PATs. All three hosts (cluster Forgejo, NAS Forgejo, api.github.com) serve valid certificates; the NAS sync step already verified. With verification on, the GHCR mirror source is authenticated too, which closes the swapped-image path.
