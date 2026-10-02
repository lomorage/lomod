#!/usr/bin/env bash
# Stops a running lomod. Shipped next to lomod in the release tarball; also invoked as
# lomoupg's --precmd hook before a self-update swaps in a new version.
#
# Matches on the full path to this install's lomod binary (via pkill -f), not just the
# process name, so this can't kill an unrelated lomod install elsewhere on the machine.
# The launchd agent install.sh registers uses RunAtLoad only (no KeepAlive), so killing it
# here does not cause launchd to immediately respawn it -- same "stopped until next login
# or manual start" behavior as the Windows Startup-folder shortcut.
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

pkill -f "${SCRIPT_DIR}/lomod" >/dev/null 2>&1
exit 0
