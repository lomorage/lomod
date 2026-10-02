<#
.SYNOPSIS
  Puts a Lomorage icon in the Windows notification area (system tray) with a right-click menu
  to open the web UI, start/stop/restart lomod.exe and check for updates. Mirrors the macOS
  menu bar app (installers/macos/Lomorage.app/Contents/MacOS/lomorage-tray.swift): same menu,
  same "running/stopped" line at the top, and the icon is dimmed while lomod is stopped.

.DESCRIPTION
  Runs entirely at the current user's permission level -- no admin rights, no Windows Service
  (see lomorage-start.bat / install.ps1 for why this project deliberately avoids a Service:
  it'd need elevation to install and would need extra logon-as-user config to reach a
  per-user DataDir like Pictures\Lomorage). This script is just a persistent, hidden
  PowerShell process hosting a WinForms NotifyIcon + message loop -- no new Go dependency,
  no separate compiled binary, consistent with the other scripts/windows/*.ps1 tooling already
  in this repo.

  Intended to be the target of the per-user Startup-folder autostart shortcut (see
  install.ps1's Register-Autostart) and of the shortcut install.ps1 launches right after a
  fresh install, both via:
      powershell.exe -WindowStyle Hidden -ExecutionPolicy Bypass -File lomorage-tray.ps1

  Starts lomod.exe itself on launch if it isn't already running, so a single autostart entry
  (this script) is enough -- no separate lomorage-start.bat entry needed for normal use.
  lomorage-start.bat / lomorage-stop.bat still exist unchanged for lomoupg's --precmd/--postcmd
  self-update hooks, which run at a point where nothing is listening for tray menu clicks
  anyway.

  Quit also stops lomod.exe -- if the tray isn't running, nothing is left to start/stop/open
  it from, so leaving lomod running headless with no way back to the tray short of re-running
  this script by hand isn't useful. Autostart brings both back together next login.

.PARAMETER InstallDir
  Where lomod.exe, lomod.args, and lomorage.ico live. Defaults to this script's own directory
  (it's shipped next to them in the release zip).
#>
param(
    [string]$InstallDir = $PSScriptRoot,
    # How long after tray start (logon) to wait before checking for an update.
    [int]$UpdateCheckDelaySeconds = 120
)

# Get this process's working directory out of InstallDir. The scheduled self-update
# (lomorage-update.ps1 -> lomoupg.exe) swaps in a new release by renaming InstallDir itself,
# and Windows refuses to rename a directory that any live process has as its working
# directory -- this tray runs for the whole login session, so it would block every update.
# [Environment]::CurrentDirectory, not Set-Location: only the former moves the process's
# actual working directory, which is what holds the directory handle.
[Environment]::CurrentDirectory = [Environment]::GetFolderPath("UserProfile")

# Singleton guard: exit quietly if another tray instance is already running, rather than
# putting up a second (or third...) icon. Without this, anything that launches this script
# again while a prior instance is still alive -- a manual re-run, a crash-and-restart, a dev
# workflow that redeploys lomod.exe and relaunches the tray without first stopping the old
# one -- leaves a pile of stale icons in the notification area, each still perfectly willing
# to start/stop/restart lomod out from under the others.
$mutexCreatedNew = $false
$singletonMutex = New-Object System.Threading.Mutex($true, "Local\Lomorage.TrayIcon", [ref]$mutexCreatedNew)
if (-not $mutexCreatedNew) {
    exit
}

Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

function Get-LomodArgString {
    $argsFile = Join-Path $InstallDir "lomod.args"
    if (-not (Test-Path $argsFile)) { throw "lomod.args not found at $argsFile" }
    return (Get-Content $argsFile -Raw).Trim()
}

function Get-LomodPort {
    try {
        $argStr = Get-LomodArgString
        if ($argStr -match '--port\s+(\d+)') { return [int]$Matches[1] }
    } catch { }
    return 8000
}

function Test-LomodRunning {
    return @(Get-Process -Name "lomod" -ErrorAction SilentlyContinue).Count -gt 0
}

function Start-Lomod {
    if (Test-LomodRunning) { return }
    $exe = Join-Path $InstallDir "lomod.exe"
    $argStr = Get-LomodArgString
    Start-Process -FilePath $exe -ArgumentList $argStr -WorkingDirectory $InstallDir -WindowStyle Hidden
}

function Stop-Lomod {
    Get-Process -Name "lomod" -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
}

function Restart-Lomod {
    Stop-Lomod
    Start-Sleep -Milliseconds 500
    Start-Lomod
}

# Checks for a new release once per tray start, i.e. at every logon. The daily Scheduled Task
# (install.ps1's Register-Autoupdate) stays the regular check for machines that are left on;
# this covers the ones that are off at that hour without depending on the task's own catch-up,
# and installs where registering the task failed. lomorage-update.ps1 makes sure the two never
# run at the same time.
function Start-UpdateCheck {
    $updateScript = Join-Path $InstallDir "lomorage-update.ps1"
    if (-not (Test-Path $updateScript)) { return }
    # WorkingDirectory outside InstallDir, for the same reason as this script's own (see top).
    Start-Process -FilePath "powershell.exe" -WindowStyle Hidden `
        -WorkingDirectory ([Environment]::GetFolderPath("UserProfile")) `
        -ArgumentList "-WindowStyle", "Hidden", "-ExecutionPolicy", "Bypass", "-File", "`"$updateScript`"", "-DelaySeconds", $UpdateCheckDelaySeconds
}

function Get-InstalledVersion {
    $versionFile = Join-Path $InstallDir "version.txt"
    $version = if (Test-Path $versionFile) { (Get-Content $versionFile -Raw).Trim() } else { "" }
    if ($version) { return $version }
    return "unknown"
}

# Newest lomod-bak-* under lomorage-update.ps1's backup dir. lomoupg moves the old install there
# only when it actually swaps in a new release, so a new name here is what says an update happened.
function Get-LatestUpdateBackup {
    $backupRoot = Join-Path (Split-Path $InstallDir -Parent) "lomod-update-backup"
    $latest = Get-ChildItem -Path $backupRoot -Directory -Filter "lomod-bak-*" -ErrorAction SilentlyContinue |
        Sort-Object Name -Descending | Select-Object -First 1
    if ($latest) { return $latest.Name }
    return ""
}

# lomorage-update.ps1 holds this mutex for the whole check (same name computation).
function Test-UpdateRunning {
    $mutexName = "Local\Lomorage.Update." + ($InstallDir.ToLowerInvariant() -replace '[^a-z0-9]', '_')
    $existing = $null
    if ([System.Threading.Mutex]::TryOpenExisting($mutexName, [ref]$existing)) {
        $existing.Dispose()
        return $true
    }
    return $false
}

function Show-Notification([string]$Text) {
    $notifyIcon.ShowBalloonTip(5000, "Lomorage", $Text, [System.Windows.Forms.ToolTipIcon]::Info)
}

# Runs lomorage-update.ps1 as its own process, like the logon check, and reports how it went
# once it exits (see Watch-ManualUpdate). Unlike on macOS, an update never restarts this tray:
# lomoupg only stops and restarts lomod.exe, so the tray is still here to report.
$script:updateProc = $null
function Start-ManualUpdateCheck {
    if (($script:updateProc -and -not $script:updateProc.HasExited) -or (Test-UpdateRunning)) {
        Show-Notification "An update check is already running."
        return
    }
    $updateScript = Join-Path $InstallDir "lomorage-update.ps1"
    if (-not (Test-Path $updateScript)) {
        [System.Windows.Forms.MessageBox]::Show(
            "Updates are not set up for this install. Re-run the installer to update and enable them.",
            "Lomorage", [System.Windows.Forms.MessageBoxButtons]::OK, [System.Windows.Forms.MessageBoxIcon]::Error) | Out-Null
        return
    }
    $script:updateFromVersion = Get-InstalledVersion
    $script:updatePrevBackup = Get-LatestUpdateBackup
    $script:updateSwappedAt = $null
    # WorkingDirectory outside InstallDir, for the same reason as this script's own (see top).
    $script:updateProc = Start-Process -FilePath "powershell.exe" -WindowStyle Hidden -PassThru `
        -WorkingDirectory ([Environment]::GetFolderPath("UserProfile")) `
        -ArgumentList "-WindowStyle", "Hidden", "-ExecutionPolicy", "Bypass", "-File", "`"$updateScript`""
    # Without holding the process handle from the start, ExitCode reads as empty once it exits.
    $null = $script:updateProc.Handle
    Show-Notification "Checking for updates..."
}

# Called every second from the timer below while a manual check is outstanding.
function Watch-ManualUpdate {
    $proc = $script:updateProc
    if (-not $proc -or -not $proc.HasExited) { return }
    if ($proc.ExitCode -ne 0) {
        $script:updateProc = $null
        Show-Notification "Update check failed. Lomorage is still on version $($script:updateFromVersion)."
        return
    }
    if ((Get-LatestUpdateBackup) -eq $script:updatePrevBackup) {
        $script:updateProc = $null
        Show-Notification "Lomorage is up to date (version $($script:updateFromVersion))."
        return
    }
    # Swapped. version.txt is rewritten by lomorage-post-update.bat, which lomoupg starts without
    # waiting for it, so give it a moment to land.
    if (-not $script:updateSwappedAt) { $script:updateSwappedAt = Get-Date }
    $version = Get-InstalledVersion
    $settled = $version -ne "unknown" -and $version -ne $script:updateFromVersion
    if ($settled -or ((Get-Date) - $script:updateSwappedAt).TotalSeconds -ge 30) {
        $script:updateProc = $null
        Show-Notification $(if ($settled) { "Updated to version $version." } else { "Lomorage was updated." })
        Request-StatusRefresh
    }
}

function Open-LomodWebUI {
    Start-Process ("http://localhost:{0}" -f (Get-LomodPort))
}

function Get-LomodBaseDir {
    $argStr = Get-LomodArgString
    if ($argStr -match '--base\s+"([^"]+)"') { return $Matches[1] }
    if ($argStr -match '--base\s+(\S+)') { return $Matches[1] }
    throw "--base not found in lomod.args"
}

# Wipes lomod's account/catalog state (assets.db, tokens, shares, admin.json) so it comes
# back up on the first-run setup page, WITHOUT touching any actual photos: those live under
# <BaseDir>\<username>\..., which is a sibling of var\/etc\, not inside either. Re-adding a
# user won't show pre-existing files as already-synced until something rescans that folder --
# the bytes survive, the "already synced" bookkeeping in assets.db does not.
function Reset-Lomod {
    $confirm = [System.Windows.Forms.MessageBox]::Show(
        "This will remove all accounts, tokens, and sharing/sync history, then restart lomod to the first-run setup page.`n`nYour photo files are NOT touched -- they live in a separate folder from this state. Continue?",
        "Reset Lomorage",
        [System.Windows.Forms.MessageBoxButtons]::YesNo,
        [System.Windows.Forms.MessageBoxIcon]::Warning)
    if ($confirm -ne [System.Windows.Forms.DialogResult]::Yes) { return }

    try {
        $baseDir = Get-LomodBaseDir
        Stop-Lomod
        Start-Sleep -Milliseconds 500
        foreach ($sub in "var", "etc") {
            $path = Join-Path $baseDir $sub
            if (Test-Path $path) { Remove-Item -Recurse -Force $path }
        }
        Start-Lomod
        $notifyIcon.ShowBalloonTip(3000, "Lomorage", "Reset complete. Open Lomorage to set it up again.", [System.Windows.Forms.ToolTipIcon]::Info)
    } catch {
        [System.Windows.Forms.MessageBox]::Show("Reset failed: $($_.Exception.Message)", "Reset Lomorage", [System.Windows.Forms.MessageBoxButtons]::OK, [System.Windows.Forms.MessageBoxIcon]::Error) | Out-Null
    }
}

# Greyed-out, half-transparent copy of the tray icon, shown while lomod is stopped (the macOS
# menu bar app's appearsDisabled).
function New-DimmedIcon([System.Drawing.Icon]$Source) {
    $src = $Source.ToBitmap()
    $dst = New-Object System.Drawing.Bitmap($src.Width, $src.Height)
    $matrix = New-Object System.Drawing.Imaging.ColorMatrix
    $matrix.Matrix00 = 0.30; $matrix.Matrix01 = 0.30; $matrix.Matrix02 = 0.30
    $matrix.Matrix10 = 0.59; $matrix.Matrix11 = 0.59; $matrix.Matrix12 = 0.59
    $matrix.Matrix20 = 0.11; $matrix.Matrix21 = 0.11; $matrix.Matrix22 = 0.11
    $matrix.Matrix33 = 0.5
    $attrs = New-Object System.Drawing.Imaging.ImageAttributes
    $attrs.SetColorMatrix($matrix)
    $g = [System.Drawing.Graphics]::FromImage($dst)
    $g.DrawImage($src, (New-Object System.Drawing.Rectangle(0, 0, $src.Width, $src.Height)),
        0, 0, $src.Width, $src.Height, [System.Drawing.GraphicsUnit]::Pixel, $attrs)
    $g.Dispose()
    return [System.Drawing.Icon]::FromHandle($dst.GetHicon())
}

# Small filled circle for the status line, like macOS's statusAvailable/statusNone images.
function New-StatusDot([System.Drawing.Color]$Color) {
    $bmp = New-Object System.Drawing.Bitmap(16, 16)
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $g.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
    $brush = New-Object System.Drawing.SolidBrush($Color)
    $g.FillEllipse($brush, 3, 3, 10, 10)
    $brush.Dispose()
    $g.Dispose()
    return $bmp
}

$icoPath = Join-Path $InstallDir "lomorage.ico"
$icon = if (Test-Path $icoPath) {
    New-Object System.Drawing.Icon($icoPath)
} else {
    [System.Drawing.SystemIcons]::Application
}
$dimmedIcon = New-DimmedIcon $icon
$runningDot = New-StatusDot ([System.Drawing.Color]::FromArgb(52, 199, 89))
$stoppedDot = New-StatusDot ([System.Drawing.Color]::Gray)

$notifyIcon = New-Object System.Windows.Forms.NotifyIcon
$notifyIcon.Icon = $icon
$notifyIcon.Text = "Lomorage"
$notifyIcon.Visible = $true

$menu = New-Object System.Windows.Forms.ContextMenuStrip

# No action: a label saying whether lomod is running, filled in by Update-Status. A label, not a
# disabled menu item: WinForms draws a disabled item's image greyed out, which would turn the
# green "running" dot grey.
$statusItem = New-Object System.Windows.Forms.ToolStripLabel
$statusItem.ForeColor = [System.Drawing.SystemColors]::GrayText
$menu.Items.Add($statusItem) | Out-Null

$menu.Items.Add("-") | Out-Null

$openItem = $menu.Items.Add("Open Lomorage")
$openItem.Add_Click({ Open-LomodWebUI })

$menu.Items.Add("-") | Out-Null

$startItem = $menu.Items.Add("Start")
$startItem.Add_Click({ Start-Lomod; Request-StatusRefresh })
$stopItem = $menu.Items.Add("Stop")
$stopItem.Add_Click({ Stop-Lomod; Request-StatusRefresh })
$restartItem = $menu.Items.Add("Restart")
$restartItem.Add_Click({ Restart-Lomod; Request-StatusRefresh })

$menu.Items.Add("-") | Out-Null

$resetItem = $menu.Items.Add("Reset to initial state...")
$resetItem.Add_Click({ Reset-Lomod })

$menu.Items.Add("-") | Out-Null

# No action: just a greyed-out label, filled in when the menu opens.
$versionItem = $menu.Items.Add("Version")
$versionItem.Enabled = $false
$updateItem = $menu.Items.Add("Check for Updates...")
$updateItem.Add_Click({ Start-ManualUpdateCheck })

$menu.Items.Add("-") | Out-Null

$quitItem = $menu.Items.Add("Quit")
$quitItem.Add_Click({
    $statusTimer.Stop()
    Stop-Lomod
    $notifyIcon.Visible = $false
    $notifyIcon.Dispose()
    [System.Windows.Forms.Application]::Exit()
})

function Update-Status {
    $running = Test-LomodRunning
    $startItem.Enabled = -not $running
    $stopItem.Enabled = $running
    $statusItem.Text = if ($running) { "Lomorage is running" } else { "Lomorage is stopped" }
    $statusItem.Image = if ($running) { $runningDot } else { $stoppedDot }
    # Only on a change: reassigning the tray icon every poll can make it flicker.
    if ($script:shownRunning -ne $running) {
        $script:shownRunning = $running
        $notifyIcon.Icon = if ($running) { $icon } else { $dimmedIcon }
        $notifyIcon.Text = if ($running) { "Lomorage (running)" } else { "Lomorage (stopped)" }
    }
}

# lomod is started in the background and takes a moment to exit on stop, so the state right
# after a click isn't the one it settles into: refresh on the next timer tick instead.
function Request-StatusRefresh {
    $script:statusRefreshRequested = $true
}

# Refresh right before the menu is shown, so it reflects whatever actually happened last (e.g.
# lomod crashed, or was killed outside the tray) without waiting for the next poll.
$menu.add_Opening({
    Update-Status
    $versionItem.Text = "Version " + (Get-InstalledVersion)
})

# The tray icon is dimmed while lomod is stopped, so its state shows without opening the menu.
# Polled every 5 seconds: lomod can also die or be started outside the tray (a crash, the
# updater, lomorage-stop.bat). The timer ticks every second for Request-StatusRefresh and a
# manual update check; Get-Process only runs every fifth tick otherwise.
$script:shownRunning = $null
$script:statusRefreshRequested = $false
$script:statusTicks = 0
$statusTimer = New-Object System.Windows.Forms.Timer
$statusTimer.Interval = 1000
$statusTimer.Add_Tick({
    $script:statusTicks++
    if ($script:statusRefreshRequested -or ($script:statusTicks % 5 -eq 0)) {
        $script:statusRefreshRequested = $false
        Update-Status
    }
    Watch-ManualUpdate
})

$notifyIcon.ContextMenuStrip = $menu
$notifyIcon.Add_DoubleClick({ Open-LomodWebUI })

Start-Lomod
Start-UpdateCheck
Update-Status
$statusTimer.Start()

[System.Windows.Forms.Application]::Run()
