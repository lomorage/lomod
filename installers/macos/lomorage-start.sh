#!/usr/bin/env bash
# Starts lomod with the args install.sh wrote to lomod.args at install time.
# Shipped next to lomod in the release tarball; also invoked by the launchd agent
# install.sh registers for autostart.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ARGS_FILE="${SCRIPT_DIR}/lomod.args"

if [[ ! -f "${ARGS_FILE}" ]]; then
    echo "lomod.args not found next to lomorage-start.sh, cannot start lomod" >&2
    exit 1
fi

# lomod.args declares a LOMOD_ARGS bash array (see install.sh) so paths containing spaces
# (e.g. under "Application Support") survive intact, unlike plain word-splitting.
# shellcheck source=/dev/null
source "${ARGS_FILE}"

exec "${SCRIPT_DIR}/lomod" "${LOMOD_ARGS[@]}"
