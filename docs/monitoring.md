# Monitoring

How to know the backups are good without reading logs: backup health, the status page, check-ins and metrics.

## Backup Health

`archiver healthcheck` is the container's liveness check: it fails only for what a restart or a person must fix (a dead scheduler, a crashed run, no space for logs), so a failed backup never makes the container restart. Whether the backups are good is a separate answer, `archiver health --backups`:

| State | Meaning | Exit |
|---|---|---|
| `OK` | Every service has a good primary backup, nothing is wrong | 0 |
| `DEGRADED` | Backups are good, but a secondary is behind or down or a copy to it failed, a check or mirror failed, maintenance failed, or a service has not been backed up yet | 1 |
| `FAILING` | Two scheduled backups (`BACKUP_SCHEDULE`) have come since a service's last good primary backup, or its last backup failed, a backup was refused, the recovery kit or a restore drill is failing, or a log cannot be written | 2 |

It lists why, and `archiver status` shows it first. For a monitor such as Uptime Kuma, run `docker exec archiver archiver health --backups` and alert on a non-zero exit, or read `backup_health` from `archiver status --json`, which also carries each service's last backup, the copy workers, drills, storage maintenance and the open incidents. Each service's last backup is kept in `logs/.backup-state.json`.

## Status Page

With `WEB_PORT` set (for example `8470`), Archiver serves a read-only page: backup health and why, each service's last backup, the copy workers, storage maintenance, restore drills, open incidents and the last 200 lines of a log (backup, maintenance, copies or drill). It refreshes every 30 seconds, and `/status.json` serves the same as `archiver status --json`. Nothing on it changes anything (anything but GET is refused), and no secret appears on it.

**It has no login of its own.** Put it behind a reverse proxy that authenticates (Caddy, Traefik or nginx with basic auth or forward auth such as Authelia), or publish the port only on a trusted network, for example `127.0.0.1:8470:8470` for a proxy on the same host. A startup line warns while it is on. Without `WEB_PORT` nothing listens.

## Check-ins

A check-in is a dead man's switch: an outside monitor (an Uptime Kuma push monitor, healthchecks.io) expects to hear from Archiver on a schedule and alerts when it does not, so a container that is down or a backup that never ran is noticed as surely as one that failed.

- `CHECKIN_URL` is called after each backup run that succeeds. After one that fails, its fail variant is called, so the alert comes at once rather than at the monitor's timeout: an Uptime Kuma push URL (`/api/push/…`) gets `status=down`, any other gets `/fail` added to its path (healthchecks.io and its kind).
- `STORAGE_TARGET_N_CHECKIN_URL` is called when that secondary is caught up (a copy done or a check passed), and its fail variant when copies to it or its check fail.

Set the monitor's expected interval a little longer than the backup schedule's. A check-in that cannot be delivered is logged as a warning; the monitor alerts on the silence anyway. The URLs are settings, not secrets: whoever has one can only send check-ins.

## Metrics

Archiver keeps Prometheus metrics in `logs/archiver.prom`, rewritten after every backup and every minute while the container runs. Point node-exporter's textfile collector at the logs volume (`--collector.textfile.directory`) and nothing else is needed. With `METRICS_PORT` set (for example `9469`, published in `compose.yaml`), the daemon also serves the same on `http://<host>:<port>/metrics` for Prometheus to scrape; without it nothing listens.

| Metric | Labels | Meaning |
|---|---|---|
| `archiver_backup_health` | | 0 OK, 1 DEGRADED, 2 FAILING ([Backup Health](#backup-health)) |
| `archiver_running` | `run` | Whether a backup, maintenance or drill is running |
| `archiver_service_last_success_timestamp_seconds` | `service`, `directory` | When the service last backed up to the primary |
| `archiver_service_last_attempt_timestamp_seconds`, `_last_duration_seconds`, `_last_ok` | `service`, `directory` | Its last backup: when it ended, how long it took, whether it succeeded |
| `archiver_service_last_revision`, `_last_uploaded_bytes` | `service`, `directory` | The revision and the bytes uploaded by its last successful backup |
| `archiver_copy_behind_revisions`, `_failing`, `_last_success_timestamp_seconds`, `_last_check_timestamp_seconds` | `target` | Each secondary's copy worker |
| `archiver_storage_last_check_timestamp_seconds`, `_last_prune_timestamp_seconds` | `storage` | Maintenance's last successful check and prune |
| `archiver_drill_last_ok`, `_last_timestamp_seconds` | `storage`, `service` | The last restore drill |
| `archiver_incidents_open`, `archiver_incident_open` | `key`, `title` | Open incidents |

For example, alert when `time() - archiver_service_last_success_timestamp_seconds > 2 * 86400` or `archiver_backup_health > 0`.
