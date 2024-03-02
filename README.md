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

# Run the setup script (if applicable)
./setup.sh

Copy secrets.sh.example: In the .keys directory of the cloned repository, locate the secrets.sh.example file. This file contains example values for sensitive variables used in the project. Copy this file and rename the copy to secrets.sh:

Edit secrets.sh: Open the secrets.sh file in a text editor of your choice. Replace the example values with your actual sensitive information. Be sure to fill in all required variables.



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

### GitHub setup
sudo apt update
sudo apt install git
git config --global user.name "casabryant"
git config --global user.email "cody+github@casabryant.com"
cd "${HOME}"
git clone git@github.com:casabryant/archiver.git # Make sure to add ssh rsa_id.pub key to GitHub authentication key first
