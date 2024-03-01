# Archiver

Archiver is a powerful, deduplication-based backup tool designed to streamline the process of securing and managing digital archives efficiently. Leveraging the robust capabilities of [Duplicacy](https://github.com/gilbertchen/duplicacy), Archiver provides an intuitive and reliable solution for handling backups.

## Features

- **Efficient Deduplication**: Utilizes Duplicacy's block-level deduplication to minimize storage space.
- **Secure Backups**: Ensures data integrity and confidentiality with encryption.
- **Flexible Configuration**: Offers easy setup and customization through a simple configuration file.
- **Automated Rotation**: Implements smart backup rotation policies to manage storage effectively.

## Getting Started

These instructions will get you a copy of the project up and running on your local machine for development and testing purposes.

### Prerequisites

What things you need to install the software and how to install them:

### Installing

A step-by-step series of examples that tell you how to get a development environment running:

```bash
# Clone the repository
git clone https://github.com/yourusername/archiver.git

# Navigate to the project directory
cd archiver

# Make the main script executable
chmod +x main.sh

# Run the setup script (if applicable)
./setup.sh

### Previously included instructions:
Copy link of most recent duplicacy_linux_arm64_X.X.X binary from:
https://github.com/gilbertchen/duplicacy/releases/latest

sudo wget -O /opt/duplicacy/duplicacy_linux_arm64_X.X.X https://github.com/gilbertchen/duplicacy/releases/download/vX.X.X/duplicacy_linux_arm64_X.X.X

sudo chmod 755 /opt/duplicacy/duplicacy_linux_arm64_X.X.X

sudo ln -s /opt/duplicacy/duplicacy_linux_arm64_X.X.X /usr/local/bin/duplicacy

#Key generation
openssl genrsa -aes256 -out private.pem -traditional 2048
openssl rsa -in private.pem --outform PEM -pubout -out public.pem
ssh-keygen

#To restore
#First you need to init with the same repository id
cd path/to/restore/dir
duplicacy init -e RPi4b8G-adguard sftp://archiver@192.168.2.6/srv/dev-disk-by-uuid-980406B804069980/duplicacy
#it will ask for "Enter the path of the private key file:"
#/home/cody/archiver/.keys/id_rsa
#it will then ask for "Enter storage password for sftp://archiver@192.168.2.6/srv/dev-disk-by-uuid-980406B804069980/duplicacy:"
#enter the storage password
duplicacy list -a #this should give you the info for revision number, needed below
#will ask for the same 2 as above
#next command needs sudo to be able to store the files owned by root
sudo duplicacy restore -r 1 -key /home/cody/archiver/.keys/private.pem
#Will be asked for storage password, then for key passphrase
