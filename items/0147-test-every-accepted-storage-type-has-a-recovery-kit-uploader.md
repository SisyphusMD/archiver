# #147 test: every accepted storage type has a recovery-kit uploader

- Kind: pull, state: closed, by cody, created 2026-10-05T14:53:53-07:00
- Labels: none; milestone: none
- refs/pull/147/head -> main, merged: True

`config.StorageTypes` is the list `Validate` accepts; the kit dispatches through an `uploaders` map, and `TestEveryStorageTypeHasAnUploader` fails when the two disagree (checked by adding a type without an uploader). No behavior change.
