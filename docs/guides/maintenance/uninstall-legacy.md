# Uninstalling Legacy Archiver Installation

This guide covers removing a legacy (non-Docker) Archiver installation from your Linux system.

## ⚠️ Prerequisites

**IMPORTANT:** Before uninstalling, ensure you have:

1. **Migrated to Docker** and verified backups work correctly
2. **Tested restores** from the Docker installation
3. **Run several successful backup cycles** from Docker
4. **Saved your bundle file** (`bundle.tar.enc` or `export-*.tar.enc`) in a safe location

**Do not proceed with uninstall until you're confident the Docker-based installation is working correctly.**

---

## Uninstall Steps

### Step 1: Disable Cron Schedule

Remove the automated backup schedule. The cron job may be in either root's crontab or your user's crontab.

**Check root crontab:**
```bash
# View root crontab
sudo crontab -l

# If archiver job is present, edit root crontab
sudo crontab -e

# Delete or comment out the archiver-related lines:
# PATH=/usr/local/bin:/usr/bin:/bin
# 0 3 * * * archiver start
```

**Check user crontab:**
```bash
# View your user crontab
crontab -l

# If archiver job is present, edit your user crontab
crontab -e

# Delete or comment out the archiver-related lines (PATH and job)
```

Save and exit the editor.

**Verify the cron job is removed:**
```bash
# Check root crontab
sudo crontab -l | grep archiver

# Check user crontab
crontab -l | grep archiver
```

Both should return no results.

### Step 2: Remove Archiver from PATH

Remove the symlink that makes the `archiver` command available system-wide:

```bash
# Check if symlink exists
ls -l /usr/local/bin/archiver

# Remove it
sudo rm /usr/local/bin/archiver

# Verify removal (may need to log out and back in for shell to recognize)
command -v archiver
```

Should return "archiver not found" or similar.

**Note:** If `command -v archiver` still shows the command after removal, log out and log back in to refresh your shell's PATH cache.

### Step 3: Remove Duplicacy Binary (Optional)

If Duplicacy was installed by the Archiver setup script and is not used by other applications:

```bash
# Check if Duplicacy is installed
command -v duplicacy

# If installed via Archiver setup, it will be at:
ls -l /usr/local/bin/duplicacy
ls -l /opt/duplicacy/

# Remove symlink
sudo rm /usr/local/bin/duplicacy

# Remove binary directory
sudo rm -rf /opt/duplicacy/

# Verify removal
command -v duplicacy
```

**Note:** If you installed Duplicacy separately or use it for other purposes, skip this step.

### Step 4: Remove Archiver Directory (Optional)

**WARNING:** This permanently deletes the Archiver repository from your system. Ensure you have:
- Your bundle file backed up elsewhere
- No need for the git history or old code

```bash
# Navigate to parent directory
cd /path/to/archiver/..

# Remove the archiver directory
sudo rm -rf archiver/
```

**Important:** Double-check you're in the correct directory before running `rm -rf`.

---

## Rollback (If Needed)

If you need to reinstall the legacy version:

1. Clone or restore the Archiver repository
2. Check out the appropriate version tag (e.g., `v0.6.5`)
3. Import your bundle file
4. Run the setup script (note: setup/uninstall commands were removed in v0.7.0, so you'd need v0.6.5 or earlier)

**Note:** With v0.7.0 being Docker-only, rollback would require using v0.6.5 or earlier.

---

## Getting Help

If you encounter issues:

- Report problems: https://forgejo.bryantserver.com/SisyphusMD/archiver/issues
- Check Docker installation guide: See README.md
