# #204 feat: Prometheus metrics textfile and METRICS_PORT (ADR 35)

- Kind: pull, state: closed, by cody, created 2026-10-09T18:24:14-07:00
- Labels: none; milestone: none
- refs/pull/204/head -> main, merged: True

Stacked on #203 (preflight): merge that first.

ADR 35. internal/metrics renders status.Take's snapshot as Prometheus gauges: backup health, runs in progress, each service's last backup (success/attempt time, duration, ok, revision, bytes uploaded parsed from duplicacy -stats), copy workers (behind, failing, last caught up, last check), maintenance (last check/prune per storage), drills, open incidents. logs/archiver.prom written atomically after every backup run and every minute by the daemon (or by the entrypoint in manual mode). METRICS_PORT serves the same on /metrics; validated at container start (both modes) and in config. compose.yaml: commented METRICS_PORT + ports. README "Metrics" table. tests/integration/metrics.sh; 50 integration scripts + e2e pass locally. 2 Codex rounds, last clean.
