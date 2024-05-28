# Archiver

Archiver is a powerful, highly configurable backup tool, designed to remove barriers to following the 3-2-1 backup rule. It leverages the robust capabilities of [Duplicacy](https://github.com/gilbertchen/duplicacy) to create encrypted and de-duplicated backups, and automates the process of intiating, copying, pruning, and restoring Duplicacy repositories for any directory or service. It provides an easy way to run custom pre- and post-backup scripts for each directory or service, while offering scheduling via cron and notifications vs Pushover.

## Features

- **Efficient Deduplication**: Utilizes Duplicacy's block-level deduplication to minimize required storage space.
- **Secure Backups**: Ensures data integrity and confidentiality with encryption.
- **Flexible Configuration**: Offers easy setup and customization through a simple configuration file.
- **Automated Rotation**: Implements smart backup rotation policies to manage storage effectively.
- **Notifications**: Receive notifications via Pushover for successful backup completions, as well as any errors the script runs into. No more silent failures. Plan to support further notifcation services in the future.
- **Multiple Storage Backends Supported**: Currently support SFTP and B2 storage backends via duplicacy. Plan to add further backend support in the future.

## Getting Started

### Prerequisites

- **Supported OS**: Currently only support debian-based linux.
- **Supported Architecture**: Currently support ARM64 and AMD64.
- **Required Dependencies**: Requires git to clone the repository. All other required dependencies installed via setup script.
- **Configuration File**: Setup script can optionally aid in creating a config file. Otherwise, can manually copy and edit the example config file.
- **Notifications**: Pushover account required to receive notifications.
- **SFTP-Supporting Storage (i.e. Synology NAS) or BackBlaze B2 Required**: You should have available storage configured before installing.

### Storage Backend Preparation

#### BackBlaze
- **1**:

#### SFTP
- **1**:

### Installation

```bash
# Navigate to the desired parent directory for the project.
# For example, if installing in home dir:
cd ~

# Clone the repository
git clone https://github.com/SisyphusMD/archiver.git

# Run the setup script
sudo ./archiver/setup.sh
