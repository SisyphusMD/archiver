# SSH Key Management Guide

This guide covers how to create and manage SSH keys for SFTP storage targets in Archiver running in Docker.

## Overview

When using SFTP storage, Archiver requires an SSH private key for authentication. Archiver only supports Ed25519 key pairs with no passphrase for SFTP authentication.

During the initial setup (when running `docker run ... archiver:v0.7.0 init`), you are prompted:

```
Would you like to generate an SSH key pair for Duplicacy SFTP storage? (y|N):
```

- If you choose **Yes**, Archiver generates an Ed25519 key pair at `/opt/archiver/keys/id_ed25519`
- If you choose **No**, you need to provide your own Ed25519 key pair

This guide covers creating new keys after initial setup, replacing existing keys, and using existing keys from your host system.

---

## Creating New SSH Keys

### Step 1: Generate SSH Key Pair

Exec into the container and generate a new Ed25519 key pair:

```bash
docker exec -it archiver bash

# Generate Ed25519 key with no passphrase
ssh-keygen -t ed25519 -f /opt/archiver/keys/id_ed25519 -N "" -C "archiver"
```

This creates two files:
- `/opt/archiver/keys/id_ed25519` (private key)
- `/opt/archiver/keys/id_ed25519.pub` (public key)

### Step 2: View the Public Key

Display the public key to add to your SFTP server:

```bash
cat /opt/archiver/keys/id_ed25519.pub
```

Copy the entire output (starts with `ssh-ed25519`).

### Step 3: Add Public Key to SFTP Server

On your SFTP server, add the public key to the authorized keys file:

```bash
# On the SFTP server, logged in as the backup user
mkdir -p ~/.ssh
echo "ssh-ed25519 AAAAC3Nza..." >> ~/.ssh/authorized_keys
chmod 700 ~/.ssh
chmod 600 ~/.ssh/authorized_keys
```

### Step 4: Test the Connection

From inside the container, test the SSH connection:

```bash
ssh -i /opt/archiver/keys/id_ed25519 user@sftp-host

# If successful, you should connect. Type 'exit' to disconnect.
exit
```

If you see "Host key verification failed", this is expected on first connection. Accept the host key:

```bash
ssh -i /opt/archiver/keys/id_ed25519 -o StrictHostKeyChecking=accept-new user@sftp-host
exit
```

### Step 5: Update Configuration

Edit your configuration to use the new key:

```bash
nano /opt/archiver/config.sh
```

Update or add the SFTP storage configuration:

```bash
STORAGE_TARGET_X_NAME="nas"
STORAGE_TARGET_X_TYPE="sftp"
STORAGE_TARGET_X_SFTP_URL="192.168.1.100"
STORAGE_TARGET_X_SFTP_PORT="22"
STORAGE_TARGET_X_SFTP_USER="backup-user"
STORAGE_TARGET_X_SFTP_PATH="backups"
STORAGE_TARGET_X_SFTP_KEY_FILE="/opt/archiver/keys/id_ed25519"
```

Save and export the configuration:

```bash
archiver bundle export
exit
```

**IMPORTANT: Backup Your Bundle File**

Your SSH key is now encrypted in the bundle file. Copy the bundle to a safe location:

```bash
cp ~/archiver-bundle/bundle.tar.enc /path/to/safe/location/
```

Keep both the bundle file and your bundle password in a secure location.

### Step 6: Verify the Configuration

Run a test backup to confirm SFTP connectivity:

```bash
docker exec archiver archiver start
```

Monitor the logs to ensure SFTP connection succeeds:

```bash
docker exec -it archiver archiver logs
```

---

## Replacing Existing SSH Keys

If you need to replace an existing SSH key (e.g., for key rotation or after a security incident):

### Step 1: Generate New Key

Exec into the container and generate a new key:

```bash
docker exec -it archiver bash

# Backup existing key
mv /opt/archiver/keys/id_ed25519 /opt/archiver/keys/id_ed25519.old
mv /opt/archiver/keys/id_ed25519.pub /opt/archiver/keys/id_ed25519.pub.old

# Generate new key
ssh-keygen -t ed25519 -f /opt/archiver/keys/id_ed25519 -N "" -C "archiver"

# Display new public key
cat /opt/archiver/keys/id_ed25519.pub
```

Copy the public key output.

### Step 2: Add New Public Key to SFTP Server

On your SFTP server, add the new public key:

```bash
# On the SFTP server
echo "ssh-ed25519 AAAAC3Nza..." >> ~/.ssh/authorized_keys
```

### Step 3: Test New Key

From inside the container:

```bash
ssh -i /opt/archiver/keys/id_ed25519 user@sftp-host
exit
```

### Step 4: Export Configuration

Export the updated configuration with the new key:

```bash
archiver bundle export
exit
```

**IMPORTANT: Backup Your Bundle File**

```bash
cp ~/archiver-bundle/bundle.tar.enc /path/to/safe/location/
```

### Step 5: Verify and Clean Up

Test a backup with the new key:

```bash
docker exec archiver archiver start
```

After confirming the new key works, remove the old public key from your SFTP server's `~/.ssh/authorized_keys` file.

You can optionally remove the old key backup from the container:

```bash
docker exec archiver rm /opt/archiver/keys/id_ed25519.old /opt/archiver/keys/id_ed25519.pub.old
```

---

## Using Existing Keys from Host

If you already have an Ed25519 SSH key on your host that you want to use, you can copy it into the container:

```bash
# Copy from host to container
docker cp ~/.ssh/id_ed25519 archiver:/opt/archiver/keys/id_ed25519
docker cp ~/.ssh/id_ed25519.pub archiver:/opt/archiver/keys/id_ed25519.pub

# Set proper permissions
docker exec archiver chmod 600 /opt/archiver/keys/id_ed25519
docker exec archiver chmod 644 /opt/archiver/keys/id_ed25519.pub
```

Then follow steps 3-6 from "Creating New SSH Keys" to add the public key to your SFTP server, update configuration, and verify.

---

## Key Requirements

- **Key type**: Ed25519 only (RSA, ECDSA, etc. are not supported)
- **Passphrase**: Must have no passphrase (empty passphrase)
- **Permissions**: Private key must be `600` (`-rw-------`)
- **Location**: Default is `/opt/archiver/keys/id_ed25519`

---

## Getting Help

If you encounter issues:
- View logs: `docker exec -it archiver archiver logs`
- Report issues: https://github.com/sisyphusmd/archiver/issues
