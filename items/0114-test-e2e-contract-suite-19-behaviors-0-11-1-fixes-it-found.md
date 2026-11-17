# #114 test(e2e): contract suite (19 behaviors) + 0.11.1 fixes it found

- Kind: pull, state: closed, by cody, created 2026-09-29T06:23:39-07:00
- Labels: none; milestone: none
- refs/pull/114/head -> main, merged: True

Phase 4, ADR 2: every contract behavior Cody marked is now a Go black-box e2e test (six groups, written and independently verified by a 12-agent workflow; each test proven by a sabotaged image). The tests found four real 0.11 defects, fixed here as the 0.11.1 patch (tests land first, per the patch-release rule):

- security: hooks inherited the DUPLICACY_* storage password / RSA passphrase (every post hook, every later pre hook)
- restore without CHOWN/FOWNER aborted at the first non-root file (the warn-not-refuse promise was false)
- a stopped backup could exit 0 (stop handler killed by its own signal); a stopped maintenance run exited 0
- the recovery kit was named by the kernel hostname, not the inherited HOSTNAME (a new kit per Job pod)

Dropped by Cody: making the healthcheck unhealthy after a failed finished run (deliberate old behavior; revisit in Phase 6b).

Also: pause-resume.sh polls instead of a fixed 3s sleep (flaked under load).

Local: 29/29 e2e on the fixed image (the four bug tests fail on 0.11.0), bats 91/91, old integration suite green, shellcheck clean, Codex review clean.
