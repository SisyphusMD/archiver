# SSH Key Management Guide

This guide covers how to create and manage SSH keys for SFTP storage targets in Archiver running in Docker.

## Overview

When using SFTP storage, Archiver requires an SSH private key for authentication. Archiver only supports Ed25519 key pairs with no passphrase for SFTP authentication.

The keypair is two secret files, mounted like every other secret:

- `/run/secrets/ssh_private_key` (override the path with `SSH_PRIVATE_KEY_FILE`)
- `/run/secrets/ssh_public_key` (override with `SSH_PUBLIC_KEY_FILE`; restore needs both halves)

At start the container copies them to `/opt/archiver/keys/id_ed25519` and `id_ed25519.pub`, which is where duplicacy reads them. Change the key by changing the secret files, never inside the container: the copies are replaced at the next start.

`archiver init` generates an Ed25519 keypair and writes it to `env-native/secrets/` with the rest of the configuration; the public key is shown at the end of init for copying to your SFTP server.

---

## Creating or Replacing SSH Keys

### Step 1: Generate the key pair

On the host, next to your other secret files:

```bash
ssh-keygen -t ed25519 -N "" -C "archiver" -f ./secrets/ssh_private_key
mv ./secrets/ssh_private_key.pub ./secrets/ssh_public_key
chmod 600 ./secrets/ssh_private_key
```

When replacing a key, keep the old files until the new key is verified.

### Step 2: Add the public key to the SFTP server

On your SFTP server, logged in as the backup user:

```bash
mkdir -p ~/.ssh
echo "ssh-ed25519 AAAAC3Nza..." >> ~/.ssh/authorized_keys
chmod 700 ~/.ssh
chmod 600 ~/.ssh/authorized_keys
```

### Step 3: Mount the keypair

In `compose.yaml`, list both files under the service's `secrets:` and the top-level `secrets:` (the template has them commented out):

```yaml
services:
  archiver:
    secrets:
      - ssh_private_key
      - ssh_public_key
secrets:
  ssh_private_key: { file: ./secrets/ssh_private_key }
  ssh_public_key:  { file: ./secrets/ssh_public_key }
```

On Kubernetes, add both files to the Secret mounted at `/run/secrets`.

Your SFTP storage target is configured with environment variables:

```bash
STORAGE_TARGET_X_NAME=nas
STORAGE_TARGET_X_TYPE=sftp
STORAGE_TARGET_X_SFTP_URL=192.168.1.100
STORAGE_TARGET_X_SFTP_PORT=22
STORAGE_TARGET_X_SFTP_USER=backup-user
STORAGE_TARGET_X_SFTP_PATH=backups
```

### Step 4: Recreate the container and test

```bash
docker compose up -d --force-recreate
docker exec -it archiver ssh -i /opt/archiver/keys/id_ed25519 -o StrictHostKeyChecking=accept-new backup-user@192.168.1.100
docker exec archiver archiver backup
```

After confirming the new key works, remove the old public key from your SFTP server's `~/.ssh/authorized_keys` file and delete the old key files.

---

## Using an Existing Key

To use an Ed25519 key you already have, copy it to `./secrets/ssh_private_key` and its public half to `./secrets/ssh_public_key`, then follow steps 2-4.

---

## Key Requirements

- **Key type**: Ed25519 only (RSA, ECDSA, etc. are not supported)
- **Passphrase**: Must have no passphrase (empty passphrase)
- **Permissions**: The private key file must be `600` (`-rw-------`)
- **Recovery kit**: the keypair is part of the configuration the recovery kit keeps on every storage target, so a replaced key is captured after the next backup
