# #236 refactor(config): drop CheckMetricsPort (no callers since CheckPorts took over in ADR 38)

- Kind: pull, state: closed, by cody, created 2026-10-13T08:00:00-07:00
- Labels: none; milestone: none
- refs/pull/236/head -> main, merged: True

Dead exported function: no callers, tests included. CheckPorts validates METRICS_PORT through the same checkPort helper.
