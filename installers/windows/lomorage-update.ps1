<#
.SYNOPSIS
  Scheduled self-update for a per-user lomod CLI install (see install.ps1). Registered as a
  daily, non-elevated Scheduled Task by install.ps1's Register-Autoupdate.

.DESCRIPTION
  Thin wrapper around lomoupg.exe, which already does the version check, download, SHA256
  verification, and directory swap (see cmd/lomoupg/main.go). This script just supplies the
  right arguments for this project's layout and fixes up what lomoupg's generic swap can't
  know about:

  - lomoupg's release-manifest lookup defaults to runtime.GOOS ("windows"), which is
    LomoAgent's own key in release.json, not this CLI's -- -platform-key/-k must be passed
    explicitly (see install.ps1's -ManifestKey doc comment for why the two never collide).
  - lomoupg's --app-dir swap (rename InstallDir aside, rename the freshly-unzipped download
    into place) only carries over what shipped in the zip. lomod.args and version.txt are
    written into InstallDir by install.ps1 at install time and are NOT part of the zip, so
    they'd be silently lost on every update unless something restores them post-swap.
    lomorage-post-update.bat (the --postcmd hook below) does that, using --backup-dir to find
    the old install lomoupg just renamed away.
  - --backup-dir is deliberately a fixed, persistent path (a sibling of InstallDir), not
    lomoupg's own default of an auto-created-and-deleted temp dir: the postcmd hook runs via
    cmd.Start() (fire-and-forget, not waited on), so if the backup were in a temp dir it could
    already be gone (lomoupg's deferred cleanup) by the time the hook reads it.

  This script does not itself need to check the current version first: lomoupg.exe already
  does that (via -curr-version) and exits with "No new version, skip upgrade" if there's
  nothing to do, so running this daily with no update pending is a harmless no-op.

.PARAMETER InstallDir
  Defaults to this script's own directory (it's shipped next to lomod.exe/lomoupg.exe in the
  release zip, same as the other installers/windows/*.ps1 scripts).
#>
param(
    [string]$InstallDir = $PSScriptRoot,
    # $env:LOMOD_RELEASE_URL: lets a test (or a staging manifest) redirect the check made by
    # the tray icon at logon, which starts this script without arguments.
    [string]$ReleaseUrl = $(if ($env:LOMOD_RELEASE_URL) { $env:LOMOD_RELEASE_URL } else { "https://lomorage.com/release.json" }),
    [string]$ManifestKey = "windows-cli",
    [int]$KeepBackups = 3,
    # Wait before checking. The tray passes this at logon, when the network is often not up yet.
    [int]$DelaySeconds = 0
)

$ErrorActionPreference = "Stop"

function Write-Step($msg) { Write-Host "==> $msg" }

# Before taking the mutex below, so a logon check still waiting here doesn't make the tray's
# "Check for Updates..." (which looks for that mutex first) report a check already running.
if ($DelaySeconds -gt 0) { Start-Sleep -Seconds $DelaySeconds }

# One update per install at a time: the daily Scheduled Task (which also catches up a missed
# run shortly after logon) and the tray's own logon check can land together, and two lomoupg
# runs renaming the same directory would trip over each other. Per InstallDir, so a second
# install elsewhere isn't held up.
$mutexName = "Local\Lomorage.Update." + ($InstallDir.ToLowerInvariant() -replace '[^a-z0-9]', '_')
$updateMutex = New-Object System.Threading.Mutex($false, $mutexName)
try {
    $haveMutex = $updateMutex.WaitOne(0)
} catch {
    # The previous holder died without releasing it; the mutex is ours now.
    if ($_.Exception.GetBaseException() -isnot [System.Threading.AbandonedMutexException]) { throw }
    $haveMutex = $true
}
if (-not $haveMutex) {
    Write-Host "Another update check is already running for $InstallDir, skipping"
    exit 0
}

try {
    $versionFile = Join-Path $InstallDir "version.txt"
    if (-not (Test-Path $versionFile)) {
        throw "version.txt not found at $versionFile -- is this a valid lomod install directory?"
    }
    $currVersion = (Get-Content $versionFile -Raw).Trim()

    # Sibling of InstallDir, not inside it: it has to survive lomoupg renaming InstallDir
    # itself aside mid-update, and stick around long enough for the postcmd hook to read it.
    $backupRoot = Join-Path (Split-Path $InstallDir -Parent) "lomod-update-backup"
    New-Item -ItemType Directory -Force -Path $backupRoot | Out-Null

    $lomoupg = Join-Path $InstallDir "lomoupg.exe"
    if (-not (Test-Path $lomoupg)) {
        throw "lomoupg.exe not found at $lomoupg"
    }

    Write-Step "Checking for updates (current: $currVersion)"
    # -only-newer: never install a build older than the running one, e.g. when the manifest
    # key this install follows points at an older release than it has. lomoupg.exe ships in
    # the same zip as this script, so the one here always knows the flag.
    & $lomoupg `
        -only-newer `
        -a $InstallDir `
        -b $backupRoot `
        -c $currVersion `
        -u $ReleaseUrl `
        -k $ManifestKey `
        -prc (Join-Path $InstallDir "lomorage-stop.bat") `
        -psc (Join-Path $InstallDir "lomorage-post-update.bat") `
        -psca $backupRoot
    $exitCode = $LASTEXITCODE

    # Prune old backups regardless of whether this run updated anything -- lomoupg only ever
    # adds a new lomod-bak-* folder here, never removes one.
    $backups = Get-ChildItem -Path $backupRoot -Directory -Filter "lomod-bak-*" -ErrorAction SilentlyContinue |
        Sort-Object Name -Descending
    if ($backups.Count -gt $KeepBackups) {
        $backups | Select-Object -Skip $KeepBackups | ForEach-Object {
            Remove-Item -Recurse -Force $_.FullName -ErrorAction SilentlyContinue
        }
    }

    # lomoupg exits non-zero when it found a new release but couldn't swap it in (it still
    # restarts the old one), which shows up as this task's Last Run Result.
    if ($exitCode -ne 0) {
        Write-Host "Update failed (lomoupg exit code $exitCode); still on $currVersion" -ForegroundColor Red
    }
    exit $exitCode
} catch {
    Write-Host "Update check failed: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}
