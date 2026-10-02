#!/usr/bin/env bash
#
# Scheduled self-update for a per-user lomod install (see install.sh). Registered as a daily
# per-user LaunchAgent by install.sh's register_autoupdate; safe to run by hand too. The macOS
# counterpart of installers/windows/lomorage-update.ps1.
#
# Thin wrapper around lomoupg, which already does the version check, download, SHA256
# verification, and directory swap (see cmd/lomoupg/main.go): running this with no update
# pending is a harmless no-op ("No new version, skip upgrade"). This script supplies the right
# arguments for this project's layout and handles what lomoupg's generic swap can't know about:
#
# - lomoupg's release-manifest lookup defaults to runtime.GOOS ("darwin"), which is LomoAgent's
#   own key in release.json, not this installer's -- --manifest-key is always passed explicitly.
# - lomoupg's --app-dir swap (rename the install dir aside, rename the freshly-extracted
#   download into place) only carries over what shipped in the tarball. lomod.args is written
#   by install.sh at install time and is NOT part of the tarball, so it's restored from the
#   install lomoupg just renamed away. version.txt is written by lomoupg itself
#   (--version-file) from the manifest's Version -- not regenerated from `lomod --version`,
#   which is stamped at build time and differs from the manifest's, so comparing against it
#   would re-download the same release every day.
# - Unlike the Windows wrapper, the post-swap work is done here after lomoupg returns, not in a
#   lomoupg --postcmd hook: that hook is fire-and-forget (cmd.Start(), never waited on), and
#   launchd kills whatever is still running in a job's process group once the job's own
#   process exits.
# - ~/Applications/Lomorage.app is a copy of the Lomorage.app template in the tarball (see
#   install.sh's install_app_bundle), so it's refreshed from the new release too.
# - A new release that can't even print its own version is rolled back to the old install.
# - --only-newer: lomoupg never installs a build older than the running one, e.g. when the
#   manifest key this install follows points at an older release than it has. lomoupg ships in
#   the same tarball as this script, so the one next to it always knows the flag.
# - lomod is only brought back up if it was running when the update started: one the user
#   deliberately stopped from the menu bar stays stopped.
# - The menu bar app's "Check for Updates…" doesn't run this script itself: a successful update
#   restarts the tray, and launchd would take down a script the tray had spawned along with
#   it. It drops an update-notify-requested file next to the install dir and kickstarts the
#   LaunchAgent instead; seeing that file, this run reports its outcome as a notification.
#
# Flags (all optional):
#   --install-dir <dir>    Default: this script's own directory (it's shipped next to lomod and
#                           lomoupg in the release tarball).
#   --release-url <url>    Default: https://lomorage.com/release.json
#   --manifest-key <key>   Default: macos-cli-arm64 or macos-cli-amd64, chosen from `uname -m`.
#   --keep-backups <n>     How many previous installs to keep under lomod-update-backup, next
#                           to the install dir. Default: 1
#   --wait-for-network     Give the network a few minutes to come up before checking. Passed by
#                           the LaunchAgent, which also runs at login -- typically before Wi-Fi
#                           has reconnected.
set -uo pipefail

LABEL="com.lomorage.lomod"
PLIST_PATH="${HOME}/Library/LaunchAgents/${LABEL}.plist"
APP_PATH="${HOME}/Applications/Lomorage.app"
LAUNCHER="${APP_PATH}/Contents/MacOS/lomorage-launcher"

log() { printf '%s %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$1"; }

notify() {
    /usr/bin/osascript -e 'on run argv' \
        -e 'display notification (item 1 of argv) with title "Lomorage"' \
        -e 'end run' "$1" >/dev/null 2>&1 || true
}

wait_for_network() {
    local url="$1" i
    for i in 1 2 3 4 5 6 7 8 9 10; do
        if curl -fsS --max-time 10 -o /dev/null "${url}" 2>/dev/null; then
            return 0
        fi
        sleep 30
    done
    return 1
}

lomod_running() {
    pgrep -f "${INSTALL_DIR}/lomod" >/dev/null 2>&1
}

latest_backup() {
    ls -1 "${BACKUP_ROOT}" 2>/dev/null | grep '^lomod-bak-' | sort | tail -1
}

# Whether ~/Applications/Lomorage.app is the menu bar app for THIS install dir (it reads the
# install dir it controls from tray-install-dir.txt -- see install.sh's install_app_bundle).
is_tray_install() {
    local config="${HOME}/Library/Application Support/Lomorage/tray-install-dir.txt"
    [[ -f "${config}" && "$(cat "${config}")" == "${INSTALL_DIR}" ]]
}

# Replacing the bundle on disk doesn't disturb an already-running tray; it picks up the new
# binary whenever it's next launched (see restart_lomod).
refresh_app_bundle() {
    local template="${INSTALL_DIR}/Lomorage.app"
    if ! is_tray_install || [[ ! -d "${template}" ]]; then
        return 0
    fi
    if diff -rq "${template}" "${APP_PATH}" >/dev/null 2>&1; then
        return 0
    fi
    log "Refreshing ${APP_PATH}"
    rm -rf "${APP_PATH}"
    mkdir -p "$(dirname "${APP_PATH}")"
    cp -R "${template}" "${APP_PATH}"
}

# Post-swap fixups for the new install. Non-zero means it isn't usable and should be rolled back.
finish_update() {
    local backup="$1"
    [[ -x "${INSTALL_DIR}/lomod" ]] || { log "new release has no lomod binary"; return 1; }
    cp -p "${backup}/lomod.args" "${INSTALL_DIR}/lomod.args" || { log "could not restore lomod.args"; return 1; }
    [[ -s "${INSTALL_DIR}/version.txt" ]] || { log "lomoupg did not write version.txt"; return 1; }
    "${INSTALL_DIR}/lomod" --version >/dev/null 2>&1 || { log "new lomod does not run"; return 1; }
    refresh_app_bundle
}

rollback() {
    local backup="$1"
    log "Rolling back to the previous install"
    [[ -d "${backup}" ]] || return 1
    rm -rf "${INSTALL_DIR}"
    mv "${backup}" "${INSTALL_DIR}"
}

# Same bootout/bootstrap install.sh's register_autostart does. launchd, not this script, has to
# be what starts the tray (or a headless lomod): anything started from here belongs to the
# update job, not to the autostart agent that's supposed to be minding it.
relaunch_agent() {
    local domain i
    domain="gui/$(id -u)"
    launchctl bootout "${domain}/${LABEL}" >/dev/null 2>&1 || true
    # A tray reopened by hand (Spotlight/Launchpad) isn't launchd's to stop.
    pkill -f "${LAUNCHER}" 2>/dev/null || true
    # bootout can return before launchd has finished tearing the job down, and bootstrap is
    # refused until it has.
    for i in 1 2 3 4 5 6 7 8 9 10; do
        if launchctl bootstrap "${domain}" "${PLIST_PATH}" >/dev/null 2>&1; then
            return 0
        fi
        sleep 1
    done
    return 1
}

restart_lomod() {
    "${INSTALL_DIR}/lomorage-stop.sh" || true
    # Only go through the autostart agent if it's this install's: either it launches the tray
    # that controls this install dir, or it's the headless fallback running this install's
    # lomorage-start.sh directly.
    if [[ -f "${PLIST_PATH}" ]] && { is_tray_install || grep -qF "${INSTALL_DIR}/lomorage-start.sh" "${PLIST_PATH}"; }; then
        log "Restarting lomod via the ${LABEL} LaunchAgent"
        relaunch_agent && return 0
        log "could not relaunch the LaunchAgent, starting lomod directly"
    fi
    nohup "${INSTALL_DIR}/lomorage-start.sh" >/dev/null 2>&1 &
}

prune_backups() {
    ls -1 "${BACKUP_ROOT}" 2>/dev/null | grep '^lomod-bak-' | sort -r | tail -n "+$((KEEP_BACKUPS + 1))" |
        while IFS= read -r name; do
            rm -rf "${BACKUP_ROOT:?}/${name}"
        done
}

# Everything runs from main, called on the last line: bash reads a script as it executes it,
# and this file lives in the install dir that gets swapped out from under it mid-run.
main() {
    INSTALL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    local release_url="https://lomorage.com/release.json"
    local manifest_key=""
    local wait_network=""
    KEEP_BACKUPS=1

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --install-dir) INSTALL_DIR="$2"; shift 2 ;;
            --release-url) release_url="$2"; shift 2 ;;
            --manifest-key) manifest_key="$2"; shift 2 ;;
            --keep-backups) KEEP_BACKUPS="$2"; shift 2 ;;
            --wait-for-network) wait_network=1; shift ;;
            *) echo "unknown argument: $1" >&2; exit 1 ;;
        esac
    done

    if [[ -z "${manifest_key}" ]]; then
        case "$(uname -m)" in
            arm64) manifest_key="macos-cli-arm64" ;;
            *) manifest_key="macos-cli-amd64" ;;
        esac
    fi

    local notify_request notify_requested=""
    notify_request="$(dirname "${INSTALL_DIR}")/update-notify-requested"
    if [[ -f "${notify_request}" ]]; then
        rm -f "${notify_request}"
        notify_requested=1
    fi

    local version_file="${INSTALL_DIR}/version.txt"
    if [[ ! -f "${version_file}" ]]; then
        log "version.txt not found at ${version_file} -- is this a valid lomod install directory?"
        exit 1
    fi
    local curr_version
    curr_version="$(cat "${version_file}")"

    local lomoupg="${INSTALL_DIR}/lomoupg"
    if [[ ! -x "${lomoupg}" ]]; then
        log "lomoupg not found at ${lomoupg}"
        exit 1
    fi

    # Sibling of the install dir, not inside it: it has to survive lomoupg renaming the install
    # dir itself aside, and a rename (not a copy) needs it on the same volume.
    BACKUP_ROOT="$(dirname "${INSTALL_DIR}")/lomod-update-backup"
    mkdir -p "${BACKUP_ROOT}"

    # Someone waiting on a "Check for Updates…" click should hear back now, not in five minutes.
    if [[ -n "${wait_network}" && -z "${notify_requested}" ]] && ! wait_for_network "${release_url}"; then
        log "${release_url} is unreachable, skipping this update check"
        exit 1
    fi

    local was_running=""
    if lomod_running; then
        was_running=1
    fi
    local prev_backup
    prev_backup="$(latest_backup)"

    log "Checking for updates (current: ${curr_version})"
    "${lomoupg}" \
        -a "${INSTALL_DIR}" \
        -b "${BACKUP_ROOT}" \
        -c "${curr_version}" \
        -u "${release_url}" \
        -k "${manifest_key}" \
        -prc "${INSTALL_DIR}/lomorage-stop.sh" \
        -psc "" \
        --version-file "${version_file}" \
        --only-newer
    local rc=$?

    # lomoupg reports a failed swap on stdout, not in its exit code; a new lomod-bak-* is what
    # says the old install was actually moved aside.
    local new_backup swapped=""
    new_backup="$(latest_backup)"
    if [[ -n "${new_backup}" && "${new_backup}" != "${prev_backup}" ]]; then
        swapped=1
        if finish_update "${BACKUP_ROOT}/${new_backup}"; then
            log "Updated to $(cat "${version_file}")"
        else
            rollback "${BACKUP_ROOT}/${new_backup}" || log "rollback failed, re-run the installer to repair: curl -fsSL https://lomorage.com/mac/install.sh | bash"
            rc=1
        fi
    fi

    # Also covers lomoupg having stopped lomod (its --precmd) and then failed to swap.
    if [[ -n "${was_running}" ]] && { [[ -n "${swapped}" ]] || ! lomod_running; }; then
        restart_lomod
    fi

    prune_backups

    if [[ -n "${notify_requested}" ]]; then
        if [[ "${rc}" -ne 0 ]]; then
            notify "Update check failed. Details are in ~/Library/Logs/Lomorage/update.log"
        elif [[ -n "${swapped}" ]]; then
            notify "Updated to version $(cat "${version_file}")"
        else
            notify "Lomorage is up to date (version ${curr_version})"
        fi
    fi
    exit "${rc}"
}

main "$@"
