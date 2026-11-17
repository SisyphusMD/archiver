# #166 fix: recovery kit on Dropbox app-folder storages

- Kind: pull, state: closed, by cody, created 2026-10-06T16:40:33-07:00
- Labels: none; milestone: none
- refs/pull/166/head -> main, merged: True

Found by the first real-service run against the Dropbox test account: the backup and copy worked, but the kit failed with `path root is not supported for sandbox app`. rclone treats `KIT:/archiver/primary` as absolute and sends a path-root header, which Dropbox refuses for app-folder apps; `KIT:archiver/primary` works (checked against the real account with rclone 1.75.1). The Dropbox remote path is now relative, which also fixes the envelope fetch command. `TestDropboxRemotePathRelative` pins it.
