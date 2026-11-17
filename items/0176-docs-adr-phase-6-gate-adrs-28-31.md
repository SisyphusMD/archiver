# #176 docs(adr): phase 6 gate (ADRs 28-31)

- Kind: pull, state: closed, by cody, created 2026-10-08T09:01:01-07:00
- Labels: none; milestone: none
- refs/pull/176/head -> main, merged: True

Cody's phase 6 decisions (2026-10-08), text approved: 28 opt-in rotating restore drills, 29 restore preview / other paths / single files / guided recovery, 30 archiver doctor, 31 fault injection in CI.

After Cody approved, two wording clarifications came from review, neither changing a decision. In ADR 28, drill freshness is checked on drill runs overall, since rotation drills each service less often. In ADR 31, a retry that completes a dropped transfer still succeeds.
