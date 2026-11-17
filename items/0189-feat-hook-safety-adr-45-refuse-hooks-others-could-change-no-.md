# #189 feat: hook safety (ADR 45): refuse hooks others could change, -no-script, HOOKS_DIR

- Kind: pull, state: closed, by cody, created 2026-10-08T17:52:46-07:00
- Labels: none; milestone: none
- refs/pull/189/head -> main, merged: True

ADR 45 (Cody approved 2026-10-08) and its implementation. Every duplicacy call goes through proc.NoScript. hooks.Safe resolves the hook path component by component as the kernel does and refuses it when the file, or any directory an entry on the way is read from, is writable by group/others (sticky dirs excepted); owner not checked; filters not checked. HOOKS_DIR (absolute, validated on backup and restore paths) holds <service>/<hook>, incl. post-restore and legacy restore-service.sh. Integration wrappers that matched $1 now match $2 (the subcommand after -no-script) and pgrep patterns include -no-script. New tests/integration/hook-safety.sh; unit tests for link chains, .. after a symlinked dir, sticky ancestors. All integration scripts and e2e pass locally.
