# #253 fix(duplicacy): File Fabric failed upload and OneDrive failed move retry report errors

- Kind: pull, state: closed, by cody, created 2026-10-17T14:33:06-07:00
- Labels: none; milestone: none
- refs/pull/253/head -> main, merged: True

Two Duplicacy 3.2.5 bugs, each as a patch in build/duplicacy/ with its own test run by the Dockerfile.

- filefabric-upload-error.patch: UploadFile did not check sendRequest's error, and sendRequest returned no error once retries ran out on an error status, so a refused or unreachable upload crashed on a nil body. TestFileFabricUploadFailsAfterRetries panicked on stock 3.2.5 and passes patched.
- onedrive-move-retry.patch: MoveFile, after creating a missing destination, returned success when its retried PATCH failed. TestOneDriveMoveFailsWhenTheRetryFails got nil on stock 3.2.5 and passes patched.

Checked: full series applies to fresh source and all 7 patch tests pass; docker build passes; tests/integration/backends.sh fabric one odb passes on the built image.
