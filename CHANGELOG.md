# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
