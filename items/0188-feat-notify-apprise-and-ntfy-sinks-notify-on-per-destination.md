# #188 feat(notify): Apprise and ntfy sinks, NOTIFY_ON per destination

- Kind: pull, state: closed, by cody, created 2026-10-08T17:10:01-07:00
- Labels: none; milestone: none
- refs/pull/188/head -> main, merged: True

Phase 4 notifier interface + Cody Oct 4 requirement (Apprise and ntfy first-class beside Pushover) + ADR 36 NOTIFY_ON. A Sink interface (Pushover, Apprise API /notify with tags from APPRISE_TAGS and basic auth from the secret URL, ntfy with token and priority); KindOf classifies titles routine/problem/failure (unknown = failure); notify.FromConfig replaces the five hand-built notifiers. New secrets apprise_url, ntfy_token (carried in the kit). Breaking: default NOTIFY_ON=failures stops Backup Complete etc.; CHANGELOG Changed says how to keep them. Ran all 33 integration scripts and the e2e suite locally (e2e harness sets PUSHOVER_NOTIFY_ON=everything).
