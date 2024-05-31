# Archiver
![Logo](logos/72x72.png) Archiver is a powerful, highly-configurable backup tool, designed to remove barriers to following the [3-2-1 Backup Strategy](https://www.backblaze.com/blog/the-3-2-1-backup-strategy/). It leverages the robust capabilities of [Duplicacy](https://github.com/gilbertchen/duplicacy) to create encrypted and de-duplicated backups, and automates the processes of initiating, copying, pruning, and restoring Duplicacy repositories for any directory or service to any number of storage backends. It provides an easy way to run custom pre-backup, post-backup, and restore scripts for each directory or service, while offering scheduling via Cron and notifications via [Pushover](https://pushover.net).

## TLDR
- Archiver will automate the backup of any configured directory to any number of SFTP and B2 storage backends.
- By default, it will back up all files within the configured directory.
- You can optionally:
  - Include and exclude files and subdirectories from the backups
  - Run custom bash scripts before and after each backup operation
  - Configure custom restore scripts to quickly get your services running again when the worst happens.

## Features

- **Efficient Deduplication**: Utilizes Duplicacy's block-level deduplication to minimize required storage space.
- **Secure Backups**: Ensures data integrity and confidentiality with encryption.
- **Flexible Configuration**: Offers easy setup and customization through a simple configuration file.
- **Automated Rotation**: Implements smart backup rotation policies to manage storage effectively.
- **Easy Restoration**: Restore script provided to get up and running again quickly after data loss.
- **Notifications**: Receive notifications via Pushover for successful backup completions, as well as any errors the script encounters. No more silent failures. Plan to support further notifcation services in the future.
- **Multiple Storage Backends Supported**: Currently support SFTP and B2 storage backends via duplicacy. Plan to add further backend support in the future.

## Getting Started

<details>
  <summary><h3>Prerequisites</h3></summary>

  - **Supported OS**: Currently only support debian-based linux.
  - **Supported Architecture**: Currently support ARM64 and AMD64.
  - **Required Dependencies**: Requires git to clone this GitHub repository. All other required dependencies installed via setup script.
  - **SFTP-Supporting Storage (i.e. Synology NAS) or BackBlaze B2 Required**: You should have at least one available supported storage target (SFTP or B2) configured before installing.
  - **Configuration File**: Setup script can optionally aid in creating a config file. Otherwise, you can manually copy and edit the example config file.
  - **Notifications**: Optional notifications via Pushover. Pushover account required to receive notifications.
</details>

<details>
  <summary><h3>Storage Backend Preparation</h3></summary>

  #### SFTP - [Synology](https://www.synology.com/en-us) NAS
  - **Enable SFTP**:
    - Login as an administrator to your Synology DiskStation Manager (DSM) Web UI (usually http://<ip.address.of.your.nas>:5000).
    - Open **Control Panel**.
    - Select **File Services** under **File Sharing**.
    - Select the **FTP** tab in the top.
    - Leave options under **FTP / FTPS** unselected. **SFTP** is not FTP or FTPS, even though the naming can be confusing.
    - Check the box to **Enable SFTP service** under **SFTP**.
    - Can change the **Port number**, or leave as the default **22**.
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
  - **Provide SSH Public Key File**:
    - If you already have an id_ed25519 SSH key (id_rsa not supported), you can complete this section now. Otherwise, the **Setup Script** below can create an SSH key for you, and you can come back to complete this section after the SSH key file is created.
    - From **Control Panel**, select **User & Group** under **File Sharing**.
    - Click **Advanced** at the top.
    - At the bottom, under **User Home**, select the checkbox to **Enable user home service**.
    - Click **Apply**.
    - From the DSM home screen, open **File Station**.
    - In the list of **Shared Folders** on the left, select **homes**.
      - ***Important***: If you select **home** instead of **homes**, you will only see the home directory of the logged in user. To add an SSH key for another user, you will need to open **homes** instead.
    - Open the folder for the user that will be used to access the share.
    - If there is already a folder named **.ssh**, double click that folder to open it. Otherwise, click **Create** at the top, then click **Create folder** in the drop down, and name the new folder **.ssh** (the leading period is required), and finally double click the newly created **.ssh** folder to open it.
      - ***Important***: Must click **Create folder** and not **Create shared folder**. The former does what we need, creating a directory within the currently open directory. The latter is to create a new higher-level shared network folder.
    - Name the new folder **.ssh**. The leading period is required.
    - Double click the newly created **.ssh** directory to open it.
    - If there is already a file named **authorized_keys**, do the following:
      - Double-click the **authorized_keys** file to download it.
      - Using a text editor, add a new line to the bottom of the document containing the contents of your public SSH key file, usually named id_ed25519.pub. The line should start with **ssh-ed25519 AAAA...**.
      - Save the document with the line added.
      - Back in **File Station**, right click **authorized_keys**, click **rename**, and rename the file to **authorized_keys.backup**.
      - Click **Upload** in the top, then click **Upload - Skip**, and browse to and select the edited **authorized_keys** file, and click **Open**.
      - Ensure the file uploads correctly and is named **authorized_keys**.
    - If there is not already a file named **authorized_keys**, do the following:
      - Using a text editor, create a new file, and copy the contents of your public SSH key file, usually named id_ed25519.pub, to this new file. The line should start with **ssh-ed25519 AAAA...**.  Save the new file as **authorized_keys**.
      - Back in **File Station**, click **Upload** at the top, then click **Upload - Skip**, and browse to and select the newly created **authorized_keys** file, and click **Open**.
      - Ensure the file uploads correctly and is named **authorized_keys**.

  #### B2 - [BackBlaze](https://www.backblaze.com/)
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
    - Make note of your **keyID** and **applicationKey**. The Application Key will only be displayed once.
</details>

<details>
  <summary><h3>Notification Set Up (optional)</h3></summary>

  #### [Pushover](https://pushover.net)
  - [Create an account](https://pushover.net/signup) or [Sign In](https://pushover.net/login) to **[Pushover](https://pushover.net)**.
  - Make note of **Your User Key**, located at the top-right corner of the Pushover Dashboard after logging in.
  - In order to receive notifications, you will need to **[Add a Phone, Tablet, or Desktop](https://pushover.net/clients)** to your account.
  - From the Pushover Dashboard, scroll to the bottom and select **[Create an Application/API Token](https://pushover.net/apps/build)**.
  - Give your application a **Name**, and optionally a **Description**, **URL**, and/or **Icon**.
  - Check the box to agree to the **Terms and Conditions**, and click **Create Application**.
  - Make note of the **API Token/Key**, located at the top of the page after creating the Application.
</details>

<details>
  <summary><h3>Installation</h3></summary>

  #### Git Installation

  - Check if git is already installed
  ```bash
  git --version
  ```

  - Install git if not installed
  ```bash
  sudo apt update
  ```
  ```bash
  sudo apt install git -y
  ```

  #### Archiver Script Installation

  - ##### Navigate to the desired parent directory for the project.
    - For example, if installing in home dir:
    ```bash
    cd ~
    ```

  - ##### Clone the GitHub repository
  ```bash
  git clone https://github.com/SisyphusMD/archiver.git
  ```

  - ##### Run the setup script
  ```bash
  sudo ./archiver/setup.sh
  ```

  - ##### Follow the instructions in the automated setup script to:
    - Install required package dependencies for the setup script.
      - If any missing dependencies are found, you will be prompted to allow their installation.
    - Download the appropriate Duplicacy binary for your system.
      - If the Duplicacy binary cannot be found, you will be prompted to allow its installation.
    - Generate the required RSA keypair for Duplicacy storage encryption.
      - If RSA key files are not found, you will be prompted to generate them.
      - See [here](https://forum.duplicacy.com/t/new-feature-rsa-encryption/2662) for manual generation details.
    - Generate SSH key files required for SFTP storage backends.
      - If SSH key files are not found, you will be prompted to generate them.
      - To generate manually, run this from the archiver directory:
        ```bash
        mkdir -p .keys && ssh-keygen -t ed25519 -f .keys/id_ed25519" -N "" -C "archiver"
        ```
        - Do **NOT** provide a passphrase when prompted.
    - Build your configuration file.
      - If your configuration file is not found, you will be prompted to build it.
      - An example configuration file is provided in the examples directory, if you prefer to configure manually.
    - Schedule Archiver to run automatically via cron.
      - See this [Cronitor Guide](https://cronitor.io/guides/cron-jobs) for details on scheduling via cron.

  - ##### Optionally, prepare custom service-backup-settings.sh and restore-service.sh files for any of your services and place in their respective service directories. Examples of these files can be found in the examples directory.

  - ##### ***IMPORTANT:*** You **MUST** keep backups of your .keys directory and all its contents, as well as your config.sh file. These will be needed to restore backups. If you can't restore your backups, why make them in the first place?
    - Don't attempt to just back up your keys and config file using this script. If you did that, and you lost them, you may not be able to get them back, given you don't have them anymore to access their backups.
</details>

<details>
  <summary><h3>Restoring</h3></summary>

  #### Restoring Archiver

  - Navigate to the desired parent directory for the project, and clone the GitHub repository as noted in the **Installation** steps.
  ```bash
  cd ~
  ```
  ```bash
  git clone https://github.com/SisyphusMD/archiver.git
  ```

  - Run the setup script to install dependencies and the Duplicacy binary, and restore cron scheduling, but otherwise you can skip the portions that create new SSH keys, RSA keys, and config file. You will need to provide your backed up .keys/ directory and config.sh file.
  ```bash
  sudo ./archiver/setup.sh
  ```

  - Copy your prior .keys directory (including your SSH and RSA key files) into the archiver directory. This should include **id_ed25519**, **id_ed25519.pub**, **private.pem**, and **public.pem**.

  - Copy your prior **config.sh** into the project directory.

  #### Restoring Services

  - Run the restore script once for each service you need to restore.
  ```bash
  sudo ./archiver/restore.sh
  ```
</details>

<details>
  <summary><h3>Usage</h3></summary>

  - If you completed the cron setup step while installing, Archiver will run automatically following the schedule you set.
    - Depending on the size of your directories and your network speeds, the first run may take a long time.
    - Archiver utilizes a LOCKFILE mechanism to ensure that multiple instances do not run concurrently.

  - To manually start a backup, run the following from your archiver directory:
    ```bash
    sudo ./archiver.sh
    ```
  - To manually start a backup with logs displaying, run the following from your archiver directory:
    ```bash
    sudo ./archiver.sh --view-logs
    ```
  - To check on the process of an actively running Archiver backup, run the following from your archiver directory:
    ```bash
    sudo ./view-logs.sh
    ```
  - To stop/cancel an actively running Archiver backup, run the following from your archiver directory:
    ```bash
    sudo ./stop-archiver.sh
    ```

  - All archiver scripts must be run with sudo or as root. This is in order to preserve ownership of files being backed up, and for access to files not owned by your user.
</details>