# #301 fix(recover): recreate service directories with their recorded owner and mode

- Kind: pull, state: closed, by cody, created 2026-11-01T06:49:42-08:00
- Labels: none; milestone: none
- refs/pull/301/head -> main, merged: True

Found by the vps kit-only recovery drill (2026-10-29): `archiver recover` created each service directory as root, while vps needs /srv dirs owned by uid 1000. A Duplicacy snapshot holds a directory's contents, not the directory, so the kit now records each existing service directory's uid, gid and mode (with setuid/setgid/sticky) in services.json, and recover applies them when it creates the directory, before the restore and its hook. Honors DRY_RUN and IGNORE_OWNERSHIP; without CHOWN+FOWNER it sets only the mode, like the restore. Warns when the kernel drops setgid (no CAP_FSETID). Adding the file re-uploads each kit once; older kits recover as before.
