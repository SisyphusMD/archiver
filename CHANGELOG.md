# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.3.1] - 2024-06-06
### Fixed
- Fixed pruning (backup rotations)

## [0.3.0] - 2024-06-05
## **!!!BREAKING CHANGE!!!**
- Calling the script with no argument will no longer initiate a backup
- Must use the full command with argument: 'archiver start'
- If you have cronjobs scheduled without the 'start' argument, they will no longer initiate a backup without editing to include 'start'
  - You can edit your cronjobs with the following command: 'sudo crontab -e'
  - i.e. '0 3 * * * archiver start'

### Improved
- Massive argument improvements:
  - Arguments are now single words, not prefaced by '--'
    - archiver start|stop|pause|resume|restart|logs|status|setup|uninstall|restore|help
  - 'archiver' command with no argument (or with 'help' argument) prints a guide to available arguments
  - 'start':
    - 'archiver start' is now required to initiate a backup
      - may need to edit 'sudo crontab -e' if it previously did not include the 'start' argument
    - 'archiver start logs' to initiate a backup and view logs
    - 'archiver start prune|retain' prune and retain will override the behavior to prune or retain backups for this run only
    - logs and prune|retain can be combined
  - 'stop|pause|resume':
    - 'archiver stop|pause|resume' manually stops, pauses, or resumes a running backup
    - 'archiver resume' can be combined with 'logs'
  - 'logs'
    - 'archiver logs' will display the logs of a running backup (but no longer starts a new backup)
  - 'status'
    - 'archiver status' prints whether or not there is a currently running backup process
  - 'restart'
    - 'archiver restart' will stop any running backup and start a new one from the beginning
    - similar to 'archiver start' can be used with logs|prune|retain
  - 'restore'
    - 'archiver restore' will run the restore script
  - 'help'
    - 'archiver help' will display information about available commands and arguments
  - 'setup|uninstall'
    - 'archiver setup|uninstall' will run the setup or uninstall scripts
    - although, on first run, setup will require './archiver.sh setup' from the archiver dir, given archiver will not be in the PATH yet
    - uninstall function coming soon

## [0.2.3] - 2024-06-04
### Fixed
- Fixed LOCKFILE being left by main.sh.

### Improved
- Setup script places archiver in PATH.
  - Please run './setup.sh' again from the archiver repo directory to make this change.
    - You can skip all sections of the setup script by typing 'n' when prompted. The script will make this change regardless.
  - You should now run Archiver backups with the command 'archiver'.
  - This is global, no more need to change to your archiver directory.
  - It accepts arguments, such as 'archiver --view-logs' and 'archiver --stop'. More arguments to come soon.
  - Cron can also call 'archiver' directly: (e.g. '0 3 * * * archiver').
    - To edit your prior cronjob, run 'sudo crontab -e', and replace the path to the archiver script with simply 'archiver'.

## [0.2.2] - 2024-06-03
### Improved
- Scripts will auto-escalate to sudo now. So README no longer recommends to run with sudo.
- Logs all go to a single file now for easier viewing.
- Reorganized directory structure.
- Stop is now an argument './archiver.sh --stop'.

## [0.2.1] - 2024-06-02
### Improved
- The LOCKFILE mechanism is much more robust now.
- Setting the stage for all commands to be through "sudo archiver --argument" rather than through calling various scripts.

## [0.2.0] - 2024-06-01
### Improved
- Major improvements to speed. Backup to primary storage for all repositories completes first, then each secondary storage copies sequentially.

## [0.1.1] - 2024-06-01
### Fixed
- Fixed the Duplicacy Prune function. Backup rotations work now.

## [0.1.0] - 2024-05-31
### Added
- Initial release of the project.
