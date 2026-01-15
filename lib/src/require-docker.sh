#!/bin/bash

# Check if running in Docker
if [ ! -f "/.dockerenv" ]; then
  echo "ERROR: Only Docker deployment is supported" >&2
  echo "" >&2
  echo "Archiver must be run inside a Docker container." >&2
  echo "Direct installation on the host system is no longer supported." >&2
  echo "" >&2
  echo "See the README for Docker deployment instructions." >&2
  echo "" >&2
  exit 1
fi
