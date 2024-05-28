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
- **Required Dependencies**: Requires git to clone this GitHub repository. All other required dependencies installed via setup script.
- **Configuration File**: Setup script can optionally aid in creating a config file. Otherwise, can manually copy and edit the example config file.
- **Notifications**: Pushover account required to receive notifications.
- **SFTP-Supporting Storage (i.e. Synology NAS) or BackBlaze B2 Required**: You should have available storage configured before installing.

### Storage Backend Preparation

#### [BackBlaze](https://www.backblaze.com/)
- **Account**:
  - [Create an account](https://www.backblaze.com/sign-up/cloud-storage) or [Sign In](https://secure.backblaze.com/user_signin.htm) to **[BackBlaze](https://www.backblaze.com/)**.
  - Select **My Settings** under **Account** in the left-hand menu.
  - Check the box for **B2 Cloud Storage** under **Enabled Products**.
  - Click **OK**.
- **Bucket**:
  - Select **Buckets** under **B2 Cloud Storage** in the left-hand menu.
  - Select **Create a Bucket**.
  - Give your bucket a **Bucket Unique Name**.
  - Files in Bucket are: **Private**.
  - Default Encryption: **Enable**.
  - Object Lock: **Disable**.
  - Select **Create a Bucket** at the bottom when ready.
  - Lifecycle Settings should be default: **Keep all versions of the file (default)**
- **Application Key**:
  - Select **Application Keys** under **Account** in the left-hand menu.
  - Select **Add a New Application Key**.
  - Give your key a **Name of Key**.
  - For **Allow access to Bucket(s)**, select the bucket you created above.
  - For **Type of Access**, select **Read and Write**.
  - Check the box to **Allow List All Bucket Names**.
  - Leave **File name prefix** and **Duration (seconds)** blank.
  - Select **Create New Key** at the bottom when ready.
  - Make note of your **keyID** and **applicationKey** for use later. The Application Key will only be displayed once.

#### SFTP *via [Synology](https://www.synology.com/en-us) NAS*
- **Enable SFTP**:
  - Login to your Synology DiskStation Manager (DSM) Web UI (usually http://<ip.address.of.your.nas>:5000).
  - Open **Control Panel**.
  - Select **File Services** under **File Sharing**.
  - Select the **FTP** tab in the top.
  - Leave options under **FTP / FTPS** unselected. **SFTP** is not FTP or FTPS, even though the naming can be confusing.
  - Check the box to **Enable SFTP service** under **SFTP**.
  - Leave the **Port number** at the default **22**.
  - Click **Apply** in the bottom right corner.
- **Create User (if needed)**:
  - From **Control Panel**, select **User & Group** under **File Sharing**.
  - Under **User** in the top, click **Create**.
  - Give your user a **Name** and **Password**.
  - Click **Next**.
  - Select the checkboxes for the **Groups** this user should join.
  - Click **Next**.
  - **Assign shared folder permissions** if desired.
  - Click **Next**.
  - **Assign user quota** if desired.
  - Click **Next**.
  - Select the checkbox for **Allow** for **SFTP**, and set other **Application Permissions** as desired.
  - Click **Next**.
  - **Set user speed limit** if desired.
  - Click **Next**.
  - Confirm your selections and click **Done**.
- **Create Shared Folder**:
  - From **Control Panel**, select **Shared Folder** under **File Sharing**.
  - Click **Create** and then **Create Shared Folder** in the top.
  - Give your new shared folder a **Name**, and either leave all settings on the page at their default, or adjust as you see fit.
  - Click **Next**.
  - On the next page, select **Skip** or **Protect this shared folder by encrypting it**.
    - Best practice is to encrypt at the *Volume* level, rather than at the *Shared Folder* level.
    - Do not select **Protect this shared folder with WriteOnce**.
  - Click **Next**.
  - Configure advanced settings to your preference.
    - If your underlying file system is BTRFS, recommend selecting **Enable data checksum for advanced data integrity**.
  - Click **Next**.
  - Confirm your selections and click **Next**.
  - Select a user to give **Read/Write** access.
  - Click **Apply**.

### Installation

```bash
# Navigate to the desired parent directory for the project.
# For example, if installing in home dir:
cd ~

# Clone the repository
git clone https://github.com/SisyphusMD/archiver.git

# Run the setup script
sudo ./archiver/setup.sh
