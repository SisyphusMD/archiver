# #334 fix(drills): scheduled drills whose times meet take turns

- Kind: pull, state: closed, by cody, created 2026-11-13T02:10:45-08:00
- Labels: none; milestone: none
- refs/pull/334/head -> main, merged: True

Kit, restore and named drills share one lock. A scheduled drill whose time came while another ran was refused, with no notification, and left for its next slot: a month for a monthly drill.

The daemon now has the drills it starts take turns: the second waits and starts when the first ends, and also waits for a drill started by hand. `archiver stop` drops a waiting drill along with the running one; a drill still waiting when the container stops runs at its next scheduled time. The first kit drill (ADR 54) never waits.

Proof: the new phases in tests/integration/restore-drill.sh fail on main with "A restore drill is already running" and pass here, including the stop phase.
