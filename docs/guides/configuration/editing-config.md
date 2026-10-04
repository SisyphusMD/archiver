# Editing Configuration in Docker

This guide covers how to change your Archiver configuration when running in Docker.

## Overview

Archiver's configuration lives where your deployment keeps it:

- **Settings** are environment variables: the `archiver.env` file your `compose.yaml` loads with `env_file:`, or a Kubernetes ConfigMap.
- **Secrets and keys** are files mounted under `/run/secrets`: Compose `secrets:`, a Kubernetes Secret, or your secret store.

Nothing is stored inside the container, so there is nothing to edit there. Change the setting or secret where it lives, then recreate the container.

For every setting and its default, see `/opt/archiver/examples/archiver.env.example` (also in the repository at `docs/examples/archiver.env.example`) and the README's "Configuration" section.

---

## Editing Workflow

### Step 1: Change the setting or secret

- A setting: edit `archiver.env` (or the ConfigMap).
- A secret: replace the file under your secrets directory (or update the Secret). Each secret is one file holding only its value.

### Step 2: Recreate the container

```bash
docker compose up -d
```

Compose recreates the container when its environment changes. After changing only a secret file, force it: `docker compose up -d --force-recreate`. On Kubernetes, restart the pod.

### Step 3: Test your changes

```bash
docker exec archiver archiver backup
```

---

## Notes

- **Recovery kit**: when the recovery password is set (`init` sets it), Archiver keeps an encrypted copy of the whole configuration on every storage target and refreshes it after the next backup, so a change is captured automatically. See the README's "Automatic Recovery Kit" section.
- **Schedules**: `BACKUP_SCHEDULE` and `MAINTENANCE_SCHEDULE` are environment variables like the rest; an invalid one stops the container from starting.
- **Storage passwords and keys**: `STORAGE_PASSWORD`, `RSA_PASSPHRASE` and the RSA keypair belong to the existing storages. Changing them does not re-encrypt what is already stored, and the old values are still needed to restore it.
