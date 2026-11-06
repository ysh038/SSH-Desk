#!/bin/sh
# Undo the last tuidesk install (@VERSION@ package): put back the version it
# replaced, or remove tuidesk if there was none before. Settings in
# ~/.config/tuidesk are never touched. Use the same option as the install:
#
#   ./rollback.sh                after ./install.sh
#   sudo ./rollback.sh --system  after sudo ./install.sh --system
#
# Only the last install can be undone (one step back).
exec "$(cd "$(dirname "$0")" && pwd)/install.sh" --rollback "$@"
