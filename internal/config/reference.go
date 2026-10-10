package config

import (
	"sort"
	"strings"
)

// Var is one configuration variable, as the reference and the schema describe it
// (ADRs 43, 44). Reference is the one list: a test fails when a variable the code accepts is
// missing from it, and the generated docs/configuration.md and docs/archiver.schema.json are
// checked against it.
type Var struct {
	Name    string
	Group   string
	Secret  bool     // a file in the secrets directory (or <NAME>_FILE), never the environment
	Default string   // "" when unset means off or none
	Values  []string // the accepted values, when there is a fixed set
	Kind    string   // string, integer, boolean (true or false), flag (on when set), interval, cron, url, port, path, list
	Desc    string
	// CaseFold: the loader takes Values in any letter case.
	CaseFold bool
	// Pattern, when set, is what the schema checks instead of the kind's pattern.
	Pattern string
}

// Groups in reference order.
var Groups = []string{"Services", "Schedules", "Storage", "Maintenance", "Notifications", "Monitoring", "Restore drills", "Recovery kit", "Hooks", "Container", "Restore commands"}

// Reference lists every setting except the per-storage ones, which TargetReference builds
// from the storage types.
var Reference = []Var{
	{Name: "SERVICE_DIRECTORIES", Group: "Services", Kind: "list", Desc: "Directories to back up, separated by colons or newlines; each may be a bash glob such as `/srv/*/`. Each directory is one snapshot, `<hostname>-<directory name>`."},
	{Name: "BACKUP_PARALLELISM", Group: "Services", Kind: "integer", Default: "2", Desc: "How many services a backup processes at once, each with its hooks and backup. 1 backs them up one after another."},
	{Name: "DUPLICACY_THREADS", Group: "Services", Kind: "integer", Default: "4", Desc: "Threads each Duplicacy backup, copy, check and restore uses."},

	{Name: "BACKUP_SCHEDULE", Group: "Schedules", Kind: "cron", Desc: "When backups run, as a cron expression (`0 3 * * *`). Unset: manual mode, nothing is scheduled."},
	{Name: "MAINTENANCE_SCHEDULE", Group: "Schedules", Kind: "cron", Desc: "When storage check and prune run. Unset: they run only on `archiver maintenance`."},
	{Name: "RESTORE_DRILL_SCHEDULE", Group: "Schedules", Kind: "cron", Desc: "When restore drills run (ADR 28). Unset: no drills."},
	{Name: "TZ", Group: "Schedules", Kind: "string", Default: "UTC", Desc: "Time zone for schedules and log timestamps, such as `America/New_York`."},

	{Name: "STORAGE_PASSWORD", Group: "Storage", Secret: true, Kind: "string", Desc: "Password encrypting every storage (at least 8 characters, a Duplicacy rule)."},
	{Name: "RSA_PASSPHRASE", Group: "Storage", Secret: true, Kind: "string", Desc: "Passphrase of the RSA private key that encrypts the backups."},

	{Name: "CHECK_BACKUPS", Group: "Maintenance", Kind: "boolean", Default: "true", Desc: "Whether maintenance checks the storages. Turn off on a deployment sharing a storage another one maintains."},
	{Name: "PRUNE_BACKUPS", Group: "Maintenance", Kind: "boolean", Default: "true", Desc: "Whether maintenance prunes by PRUNE_KEEP. Turn off on a deployment sharing a storage another one maintains."},
	{Name: "ROTATE_BACKUPS", Group: "Maintenance", Kind: "boolean", Desc: "Old name of PRUNE_BACKUPS, still read when PRUNE_BACKUPS is unset."},
	{Name: "PRUNE_KEEP", Group: "Maintenance", Kind: "string", Default: "-keep 0:180 -keep 30:30 -keep 7:7 -keep 1:1", Desc: "Duplicacy retention: one revision a day for a week, a week for a month, a month for half a year, none older."},
	{Name: "PRUNE_EXHAUSTIVE_FREQUENCY", CaseFold: true, Group: "Maintenance", Kind: "string", Default: "monthly", Values: []string{"off", "daily", "weekly", "monthly"}, Desc: "How often the prune also removes chunks no revision references."},
	{Name: "CHECK_INTERVAL", Group: "Maintenance", Kind: "interval", Desc: "How often each secondary is checked (`12h`, `7d`); a target's own STORAGE_TARGET_N_CHECK_INTERVAL wins. Default: a day, a week for SFTP-like storages."},

	{Name: "NOTIFY_ON", CaseFold: true, Group: "Notifications", Kind: "string", Default: "failures", Values: []string{"failures", "problems", "everything"}, Desc: "Which notifications every destination receives (ADR 36)."},
	{Name: "ALERT_REPEAT_INTERVAL", Pattern: `^$|^0$|` + intervalPattern, Group: "Notifications", Kind: "interval", Default: "24h", Desc: "How often an ongoing failure is notified again (`6h`, `2d`); `0` never repeats (ADR 33)."},
	{Name: "NOTIFICATION_SERVICE", CaseFold: true, Group: "Notifications", Kind: "string", Values: []string{"Pushover", "None"}, Desc: "`Pushover` sends to Pushover with the secrets below (any letter case)."},
	{Name: "PUSHOVER_USER_KEY", Group: "Notifications", Secret: true, Kind: "string", Desc: "Pushover user key."},
	{Name: "PUSHOVER_API_TOKEN", Group: "Notifications", Secret: true, Kind: "string", Desc: "Pushover application token."},
	{Name: "PUSHOVER_NOTIFY_ON", CaseFold: true, Group: "Notifications", Kind: "string", Values: []string{"failures", "problems", "everything"}, Desc: "NOTIFY_ON for Pushover alone."},
	{Name: "APPRISE_URL", Group: "Notifications", Secret: true, Kind: "url", Desc: "An Apprise API notify URL (`http://apprise:8000/notify/<key>`, with `user:password@` for basic auth)."},
	{Name: "APPRISE_TAGS", Group: "Notifications", Kind: "string", Desc: "The Apprise tag for each kind, such as `failure=critical,problem=alerts,routine=quiet`; a kind left out goes to `all`."},
	{Name: "APPRISE_NOTIFY_ON", CaseFold: true, Group: "Notifications", Kind: "string", Values: []string{"failures", "problems", "everything"}, Desc: "NOTIFY_ON for Apprise alone."},
	{Name: "NTFY_URL", Group: "Notifications", Kind: "url", Desc: "An ntfy server and topic, such as `https://ntfy.sh/my-archiver`."},
	{Name: "NTFY_TOKEN", Group: "Notifications", Secret: true, Kind: "string", Desc: "An ntfy access token, if the topic needs one."},
	{Name: "NTFY_NOTIFY_ON", CaseFold: true, Group: "Notifications", Kind: "string", Values: []string{"failures", "problems", "everything"}, Desc: "NOTIFY_ON for ntfy alone."},

	{Name: "CHECKIN_URL", Group: "Monitoring", Kind: "url", Desc: "A dead man's switch pinged after each backup run (ADR 37): Uptime Kuma push URLs get `status=up`/`down`, others `/fail` on failure."},
	{Name: "METRICS_PORT", Group: "Monitoring", Kind: "port", Desc: "Serves Prometheus metrics on `/metrics` there (ADR 35); `logs/archiver.prom` is written either way."},
	{Name: "WEB_PORT", Group: "Monitoring", Kind: "port", Desc: "Serves the read-only status page there (ADR 38). It has no login: put it behind an authenticating proxy."},
	{Name: "LOG_FORMAT", Group: "Monitoring", Kind: "string", Default: "text", Values: []string{"text", "json"}, Desc: "`json` writes the container's output as one JSON object per line."},

	{Name: "RESTORE_DRILL_SERVICES", Pattern: `^$|^([1-9][0-9]*|all)$`, Group: "Restore drills", Kind: "string", Default: "1", Desc: "How many services each drill restores from each storage, rotating; `all` restores every one."},
	{Name: "RESTORE_DRILL_STORAGES", Group: "Restore drills", Kind: "string", Default: "all", Desc: "Which storages drills restore from: `all`, `primary`, or storage names separated by commas."},
	{Name: "RESTORE_DRILL_DIR", Group: "Restore drills", Kind: "path", Default: "/tmp/archiver-drill", Desc: "Scratch space a drill restores into and deletes afterwards; never a service directory."},
	{Name: "RESTORE_DRILL_EXCLUDE", Group: "Restore drills", Kind: "list", Desc: "Services drills leave out, separated by commas."},

	{Name: "RECOVERY_PASSWORD", Group: "Recovery kit", Secret: true, Kind: "string", Desc: "Encrypts the recovery kit uploaded beside the backups; without it there is no kit. Keep it in a password manager."},
	{Name: "RECOVERY_KIT_EXTRA_PATHS", Group: "Recovery kit", Kind: "list", Desc: "Files or directories also carried in the kit's `extra/`, separated by colons."},

	{Name: "HOOKS_DIR", Group: "Hooks", Kind: "path", Desc: "Hooks come from `<HOOKS_DIR>/<service>/` instead of the service directories (ADR 45)."},

	{Name: "RSA_PRIVATE_KEY_FILE", Group: "Container", Kind: "path", Default: "/run/secrets/rsa_private_key", Desc: "The RSA private key to place in the container's key directory at start."},
	{Name: "RSA_PUBLIC_KEY_FILE", Group: "Container", Kind: "path", Default: "/run/secrets/rsa_public_key", Desc: "The RSA public key to place at start."},
	{Name: "SSH_PRIVATE_KEY_FILE", Group: "Container", Kind: "path", Default: "/run/secrets/ssh_private_key", Desc: "SFTP storages: the SSH private key to place at start."},
	{Name: "SSH_PUBLIC_KEY_FILE", Group: "Container", Kind: "path", Default: "/run/secrets/ssh_public_key", Desc: "SFTP storages: the SSH public key to place at start."},
	{Name: "HOSTNAME", Group: "Container", Kind: "string", Desc: "Overrides the host part of snapshot IDs and the kit's name. Keep it stable: changing it starts new snapshots."},
	{Name: "SECRETS_DIR", Group: "Container", Kind: "path", Default: "/run/secrets", Desc: "Where secret files are read from. Any secret can instead be named by `<SECRET>_FILE`."},

	{Name: "SNAPSHOT_ID", Group: "Restore commands", Kind: "string", Desc: "auto-restore and snapshot-exists: the snapshot, `<hostname>-<service>`."},
	{Name: "LOCAL_DIR", Group: "Restore commands", Kind: "path", Desc: "auto-restore: where to restore."},
	{Name: "REVISION", Group: "Restore commands", Kind: "string", Default: "latest", Desc: "auto-restore: a revision number or `latest`."},
	{Name: "STORAGE_TARGET", Group: "Restore commands", Kind: "string", Desc: "auto-restore: restore from this storage (name or number) only."},
	{Name: "OVERWRITE", Group: "Restore commands", Kind: "flag", Desc: "auto-restore: replace files that differ."},
	{Name: "DELETE_EXTRA", Group: "Restore commands", Kind: "flag", Desc: "auto-restore: delete files the revision does not have."},
	{Name: "HASH_COMPARE", Group: "Restore commands", Kind: "flag", Desc: "auto-restore: compare files by content, not size and time."},
	{Name: "IGNORE_OWNERSHIP", Group: "Restore commands", Kind: "flag", Desc: "auto-restore: leave ownership alone (without CHOWN and FOWNER it is left anyway, with a warning)."},
	{Name: "RESTORE_PATHS", Group: "Restore commands", Kind: "list", Desc: "auto-restore: only these paths in the snapshot, separated by commas."},
	{Name: "DRY_RUN", Group: "Restore commands", Kind: "flag", Desc: "auto-restore: show what would be restored, replaced and deleted, and restore nothing."},
	{Name: "RUN_RESTORE_SERVICE", Group: "Restore commands", Kind: "flag", Desc: "auto-restore: run the service's post-restore hook after the files are restored."},
	{Name: "RESTORE_THREADS", Group: "Restore commands", Kind: "integer", Desc: "auto-restore: threads for this restore. Default: DUPLICACY_THREADS."},
	{Name: "NO_PREVIEW", Group: "Restore commands", Kind: "flag", Desc: "restore: skip the preview of what an interactive restore changes."},
	{Name: "RECOVERY_PASSWORD_FILE", Group: "Restore commands", Kind: "path", Desc: "recover: a file holding the kit's password, instead of the prompt."},
}

// targetSettings are the per-target variables every storage type has.
var targetSettings = []Var{
	{Name: "NAME", Kind: "string", Desc: "The storage's name. Target 1 is the primary; the others are copies of it."},
	{Name: "TYPE", Kind: "string", Desc: "The storage type."},
	{Name: "CHECK_INTERVAL", Kind: "interval", Desc: "How often this secondary is checked; overrides CHECK_INTERVAL."},
	{Name: "CHECKIN_URL", Kind: "url", Desc: "Pinged when this storage is caught up or checked, its fail variant when copies or its check fail (ADR 37)."},
	{Name: "UPLOAD_LIMIT", Kind: "integer", Desc: "The most this storage is sent, in kilobytes per second: by a backup to it (the primary; services backing up at once share it, so it must be at least BACKUP_PARALLELISM) and by copies to it (ADR 44)."},
	{Name: "COPY_WINDOW", Kind: "string", Pattern: `^$|^([01][0-9]|2[0-3]):[0-5][0-9]-([01][0-9]|2[0-3]):[0-5][0-9]$`, Desc: "Secondaries: the local hours (TZ) copies and upkeep run, such as `01:00-06:00` or `22:00-06:00`; outside them the copy worker waits, and a copy still running when the window closes ends and resumes when it next opens (ADR 44). Backups to the primary are not held."},
	{Name: "BREAKGLASS_SFTP_USER", Kind: "string", Desc: "SFTP storages: the user the printed envelope names, for read-only access (ADR 23)."},
	{Name: "BREAKGLASS_SSH_KEY", Secret: true, Kind: "string", Desc: "SFTP storages: the private key of BREAKGLASS_SFTP_USER, printed on the envelope (ADR 23)."},
}

// TargetReference is every STORAGE_TARGET_<N>_ variable: the common ones, then each storage
// type's fields, merged by name with the types that use each listed.
func TargetReference() []Var {
	types := make([]string, 0, len(Types))
	for name := range Types {
		types = append(types, name)
	}
	sort.Strings(types)
	out := append([]Var(nil), targetSettings...)
	for i := range out {
		if out[i].Name == "TYPE" {
			out[i].Values = types
		}
	}
	byName := map[string]int{}
	usedBy := map[string][]string{}
	for _, tn := range types {
		for _, f := range Types[tn].Fields {
			usedBy[f.Name] = append(usedBy[f.Name], tn)
			if _, ok := byName[f.Name]; ok {
				continue
			}
			d := strings.TrimSpace(f.Prompt)
			if d != "" && !strings.HasSuffix(d, ".") {
				d += "."
			}
			if f.Optional {
				d += " Optional."
			}
			s := Var{Name: f.Name, Secret: f.Secret, Default: f.Default, Kind: "string", Desc: d}
			byName[f.Name] = len(out)
			out = append(out, s)
			if f.BreakGlass {
				byName["BREAKGLASS_"+f.Name] = len(out)
				out = append(out, Var{Name: "BREAKGLASS_" + f.Name, Secret: true, Kind: "string", Desc: "A narrower credential than " + f.Name + " (read-only), printed on the envelope instead of it (ADR 23). Optional."})
			}
		}
	}
	for name, i := range byName {
		out[i].Group = strings.Join(usedBy[strings.TrimPrefix(name, "BREAKGLASS_")], ", ")
	}
	return out
}
