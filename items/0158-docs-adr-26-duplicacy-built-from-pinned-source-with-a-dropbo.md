# #158 docs(adr): 26 Duplicacy built from pinned source with a Dropbox patch

- Kind: pull, state: closed, by cody, created 2026-10-06T07:20:33-07:00
- Labels: none; milestone: none
- refs/pull/158/head -> main, merged: True

Cody's decision (2026-10-06): build the pinned Duplicacy 3.2.5 source with a one-line patch so Dropbox refreshes directly against Dropbox with the user's own app, and offer it upstream. Supersedes ADR 7's released binary; the CLI still runs as a child process. Duplicacy's license permits modification and redistribution with the same terms.
