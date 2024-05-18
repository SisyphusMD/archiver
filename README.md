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
