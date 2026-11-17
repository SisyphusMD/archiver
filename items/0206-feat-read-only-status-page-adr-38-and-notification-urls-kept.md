# #206 feat: read-only status page (ADR 38) and notification URLs kept out of logs

- Kind: pull, state: closed, by cody, created 2026-10-09T20:21:14-07:00
- Labels: none; milestone: none
- refs/pull/206/head -> main, merged: True

Stacked on #205 (check-ins): merge that first.

1. fix(notify): a failed notification send logged Go's url.Error, which carries the request URL; an Apprise URL holds its key, so the key reached archiver.log (on disk). The error is now reported without the URL; check-in URL validation errors no longer repeat the URL. (Apprise is unreleased, so no released version leaked.)
2. feat (ADR 38): WEB_PORT serves one read-only page (html/template, auto-refresh 30 s): backup health and reasons, runs (paused shown), each service's last backup, copy workers, storage maintenance, drills, open incidents, the last 200 lines of a chosen log (archiver/maintenance/copies/drill, whitelisted), plus /status.json. GET/HEAD only (405 otherwise), CSP default-src 'none', no secrets; log lines redacted (Apprise keys, Kuma push tokens, URL credentials) as a second layer for older logs. No login: startup warning, README and compose (bound to 127.0.0.1) say to put it behind an authenticating proxy. METRICS_PORT and WEB_PORT validated as ports and not the same (numerically) at start, in both modes.

tests/integration/web.sh; unit tests (escaping, redaction, POST refused). 52 integration scripts + e2e pass locally. 8 Codex rounds, last clean.
