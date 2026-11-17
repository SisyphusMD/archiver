# #257 fix(cli): --help prints usage; drill/kit-drill/deep-check refuse unknown options

- Kind: pull, state: closed, by cody, created 2026-10-18T08:05:26-07:00
- Labels: none; milestone: none
- refs/pull/257/head -> main, merged: True

`archiver drill --help` on the NAS ran a drill of a service named `--help`, failed, and raised a Restore Drill Failed incident (backup health FAILING). flagGuard answers --help/-h after any command with the usage, and refuses unknown options for drill, kit-drill and deep-check before they run. Unit test TestFlagGuard.
