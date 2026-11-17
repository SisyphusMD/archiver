# #228 fix(security): SFTP host keys pinned once for every repository and the kit (scan F12, F15)

- Kind: pull, state: closed, by cody, created 2026-10-11T13:46:49-07:00
- Labels: none; milestone: none
- refs/pull/228/head -> main, merged: True

Every Duplicacy repository (service, copy, maintenance, restore, drill, probe) links its known_hosts to one owner-only record under logs/.ssh, so a key seen once is checked by all of them and survives container recreation; the recovery kit's OpenSSH known_hosts moves there too.

- Container start seeds the record from every existing pin (service repositories, the old in-tree .duplicacy of each service directory, copy and maintenance repositories, the kit's $HOME/.ssh copy), reading only files Archiver owns that nobody else can write.
- A repository pinning a different key for a server the record knows is refused, left in place, and named.
- build/duplicacy/host-keys-append.patch opens known_hosts with O_APPEND, so concurrent processes never overwrite each other's keys (TestHostKeysAppendConcurrently fails 3/3 without it).
- sftp-runtime.sh: after the server's keys change, backup and restore are refused.

Not done: locking Duplicacy's whole check-and-append. Two processes making the very first contact with one server at the same instant can still each trust what answers; that is trust on first use either way.
