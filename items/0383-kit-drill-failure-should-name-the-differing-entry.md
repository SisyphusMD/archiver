# #383 Kit drill failure should name the differing entry

- Kind: issue, state: open, by cody, created 2026-11-21T22:14:11-08:00
- Labels: type:bug; milestone: none

ArchiverKitDrillFailed fired for backblaze and cowanserver from 2026-11-17 03:29Z until a manual `archiver kit-drill` passed on 2026-11-21 22:03 PT. The only evidence was "kit differs from the local kit": `compareDrill` in internal/kit/drill.go does not say which file differed, and container logs had rotated past the run, so the cause could not be found. Hypothesis: a drill between the 03:00 PT shelf upload (bootstrap/shelf/kit/k8s.bundle) and the ~04:10 PT kit placement compares a stale kit against a changed shelf; also unexplained is what triggered a 03:29Z run when KIT_DRILL_SCHEDULE is 0 6 15 * *.

Fix: report the differing entry names (and placement time vs shelf mtime) in the drill result and metrics/log line.
