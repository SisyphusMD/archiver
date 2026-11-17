# #53 docs: env-native-first operational examples

- Kind: pull, state: closed, by cody, created 2026-07-09T21:57:45-07:00
- Labels: none; milestone: none
- refs/pull/53/head -> main, merged: True

Follow-up to the env-native-first-class work: the Installation section led with env-native, but every OPERATIONAL example still taught bundle mode — the external-scheduler one-shot, the Kubernetes CronJob (which mounted the bundle tar as a Secret instead of showcasing ConfigMap + Secret, the whole point of env-native), the one-off restore, and both run-mode examples. All now lead env-native (`--env-file archiver.env` + `secrets/` mounted at `/run/secrets`, exactly what `init`/`migrate` emit; the k8s example shows `kubectl create configmap --from-env-file` / `create secret --from-file`), with bundle mode as a one-line labeled alternative. Verified `docker --env-file` passes the space-containing `PRUNE_KEEP` value verbatim.
