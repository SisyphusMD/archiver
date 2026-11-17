# #254 fix(restore): a killed restore keeps the restore lock until its hook's foreground processes are gone

- Kind: pull, state: closed, by cody, created 2026-10-17T15:00:20-07:00
- Labels: none; milestone: none
- refs/pull/254/head -> main, merged: True

A restore into a service directory holds the restore lock, which keeps backups out. Its post-restore hook died with Archiver (Pdeathsig), but what the hook ran in the foreground (a shell's long command) ran on, writing to the service directory with the lock already released.

- The hook now runs under a hidden `archiver hook-guard`: the guard inherits the restore lock (fd 3, never passed on to the hook), runs the hook in its own process group, and if the restore dies kills that group and holds the lock until none of it runs. A daemon the hook starts in its own session is left alone, as before.
- The hook gets the terminal (/dev/tty) while it runs, also when the restore's answers are piped; Ctrl-Z stops the whole restore job and fg resumes the hook.
- Tests: tests/integration/restore-commands.sh case (fails on main: the lock was free while the hook's child still wrote), internal/hooks/guard_test.go (lock held until the foreground is gone, daemon survives, lock not inherited, PTY with piped stdin, Ctrl-Z continue). Job control also checked by hand with an interactive bash: jobs showed Stopped, fg resumed the hook.
- A hook killed by a signal now reports 128+signal (was -1).
