# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.1] - 2024-06-01
### Fixed
- Fixed the Duplicacy Prune function. Backup rotations work now.

### Breaking Changes
- The `PRUNE_KEEP` variable in `config.sh` must now be an array instead of a string. Users must update their `config.sh` file accordingly.
  - **Old Format**:
    ```bash
    PRUNE_KEEP="-keep 0:180 -keep 30:30 -keep 7:7 -keep 1:1"
    ```
  - **New Format**:
    ```bash
    PRUNE_KEEP=(-keep 0:180 -keep 30:30 -keep 7:7 -keep 1:1)
    ```

## [0.1.0] - 2024-05-31
### Added
- Initial release of the project.
