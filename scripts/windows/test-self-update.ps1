<#
.SYNOPSIS
  End-to-end test of the Windows scheduled self-update: the real lomorage-update.ps1,
  lomoupg.exe, lomorage-post-update.bat and lomorage-tray.ps1 from this checkout, run against
  a sandboxed install and a local release server.

.DESCRIPTION
  Nothing here touches a real install under %LOCALAPPDATA% or a running lomod.exe:
    - lomod.exe in the sandbox is a copy of lomoupg.exe stamped with a fake version (all the
      update flow needs from it is `lomod.exe --version`);
    - lomorage-stop.bat / lomorage-start.bat are replaced with stubs that only log that they
      ran, since the real ones act on every lomod.exe on the machine by image name;
    - the tray script runs with its singleton mutex renamed, so it coexists with a real tray.

  Scenarios:
    1. A process with the install dir as its working directory (what the tray was before it
       moved itself out) blocks the swap: the update must exit non-zero, leave the old
       release in place and still restart lomod.
    2. A check started while another one is running for the same install steps aside.
    3. The current tray, launched the way a pre-fix Startup shortcut launches it ("Start in"
       = install dir), neither blocks the swap nor needs the Scheduled Task: its own check at
       startup installs the new release -- lomod.args carried over, version.txt rewritten,
       old release kept as a backup, lomod restarted.
    4. Running again with nothing new is a no-op.
    5. The tray menu, driven in-process with lomod's running state faked: the status line,
       dimmed icon and Start/Stop enabling follow lomod, the Version line shows version.txt,
       and "Check for Updates..." installs a newer release and reports that, then reports
       "up to date", a failed check, and a check already in progress.
    6. A manifest pointing at an older release than the installed one (a machine switched
       from the nightly channel back to stable) doesn't downgrade it.

  Needs `go` on PATH (see windows-deps/go). Run: pwsh -File scripts/windows/test-self-update.ps1
#>
param(
    [string]$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path,
    [int]$Port = 8199,
    # Optional lomoupg.exe to put in the "installed" release instead of one built from this
    # checkout: the updater that performs an update is the one already on the user's machine,
    # so pass a released binary here to check that it can still install what this checkout builds.
    [string]$InstalledLomoupg = ""
)

$ErrorActionPreference = "Stop"

$OldVersion = "2026-01-01.00-00-00.0.aaaaaaa"
$NewVersion = "2026-02-02.00-00-00.0.bbbbbbb"
$NewerVersion = "2026-03-03.00-00-00.0.ccccccc"
$LomodArgs = "--version"

$sandbox = Join-Path $env:TEMP ("lomod-selfupdate-test-" + [guid]::NewGuid().ToString("N").Substring(0, 8))
$installDir = Join-Path $sandbox "lomod"
$backupRoot = Join-Path $sandbox "lomod-update-backup"
$www = Join-Path $sandbox "www"
$startLog = Join-Path $sandbox "start.log"
$stopLog = Join-Path $sandbox "stop.log"
$releaseUrl = "http://localhost:$Port/release.json"

$script:failures = 0
function Assert($cond, $msg) {
    if ($cond) { Write-Host "  ok   $msg" -ForegroundColor Green }
    else { Write-Host "  FAIL $msg" -ForegroundColor Red; $script:failures++ }
}

function Wait-Until([scriptblock]$cond, [int]$TimeoutSeconds = 15) {
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        if (& $cond) { return $true }
        Start-Sleep -Milliseconds 200
    }
    return $false
}

function Build-Lomoupg([string]$Version, [string]$OutFile) {
    $env:CGO_ENABLED = "0"
    $env:GOFLAGS = "-mod=vendor"
    Push-Location $RepoRoot
    try {
        & go build -ldflags "-X bitbucket.org/lomoware/lomo-backend/common/release.Version=$Version" -o $OutFile ./cmd/lomoupg
        if ($LASTEXITCODE -ne 0) { throw "go build failed" }
    } finally { Pop-Location }
}

# One release directory, laid out like the published zip.
function New-Release([string]$Dir, [string]$Version) {
    New-Item -ItemType Directory -Force -Path $Dir | Out-Null
    Copy-Item (Join-Path $RepoRoot "installers\windows\*") $Dir
    Remove-Item (Join-Path $Dir "install.ps1")
    Build-Lomoupg -Version $Version -OutFile (Join-Path $Dir "lomoupg.exe")
    Copy-Item (Join-Path $Dir "lomoupg.exe") (Join-Path $Dir "lomod.exe")
    # Nested and empty directories, like the bundled exiftool_files tree in a real release.
    New-Item -ItemType Directory -Force -Path (Join-Path $Dir "exiftool_files\lib\auto\Compress"), (Join-Path $Dir "exiftool_files\empty") | Out-Null
    Set-Content (Join-Path $Dir "exiftool_files\lib\auto\Compress\Raw.dll") $Version -NoNewline
    Set-Content (Join-Path $Dir "exiftool_files\lib\autouse.pm") $Version -NoNewline

    Set-Content (Join-Path $Dir "lomorage-stop.bat") "@echo off`r`necho stop>>`"%~dp0..\stop.log`"`r`n" -NoNewline
    Set-Content (Join-Path $Dir "lomorage-start.bat") "@echo off`r`necho start>>`"%~dp0..\start.log`"`r`n" -NoNewline

    $tray = Join-Path $Dir "lomorage-tray.ps1"
    $src = Get-Content $tray -Raw
    if ($src -notmatch 'Local\\Lomorage\.TrayIcon') { throw "tray singleton mutex name not found -- update this test" }
    Set-Content $tray ($src -replace 'Local\\Lomorage\.TrayIcon', 'Local\Lomorage.TrayIcon.SelfUpdateTest') -NoNewline
}

# Zips a release directory into the local server's root and points release.json at it.
function Publish-Release([string]$Dir, [string]$Version) {
    New-Item -ItemType Directory -Force -Path $www | Out-Null
    $zip = Join-Path $www "release-$Version.zip"
    # Built exactly as `make release-windows` builds the real one.
    & powershell.exe -NoProfile -ExecutionPolicy Bypass -File (Join-Path $RepoRoot "scripts\windows\make-zip.ps1") -SourceDir $Dir -ZipPath $zip
    if ($LASTEXITCODE -ne 0) { throw "make-zip.ps1 failed" }
    $sha = (Get-FileHash $zip -Algorithm SHA256).Hash.ToLower()
    @{ "windows-cli" = @{ Version = $Version; URL = "http://localhost:$Port/release-$Version.zip"; SHA256 = $sha } } |
        ConvertTo-Json | Set-Content (Join-Path $www "release.json")
}

function Invoke-Update {
    $p = Start-Process -FilePath "powershell.exe" -PassThru -Wait -WindowStyle Hidden -WorkingDirectory $env:SystemRoot `
        -ArgumentList "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "`"$(Join-Path $installDir 'lomorage-update.ps1')`"", "-ReleaseUrl", $releaseUrl `
        -RedirectStandardOutput (Join-Path $sandbox "update.out")
    Get-Content (Join-Path $sandbox "update.out") | ForEach-Object { Write-Host "    | $_" }
    return $p.ExitCode
}

function Get-InstalledVersion { (Get-Content (Join-Path $installDir "version.txt") -Raw).Trim() }
function Get-Backups { @(Get-ChildItem $backupRoot -Directory -Filter "lomod-bak-*" -ErrorAction SilentlyContinue) }
function Get-LogLines([string]$Path) { if (Test-Path $Path) { @(Get-Content $Path).Count } else { 0 } }

$server = $null
$holder = $null
$trayProc = $null
try {
    Write-Host "==> Building sandbox in $sandbox"
    New-Release -Dir $installDir -Version $OldVersion
    Set-Content (Join-Path $installDir "lomod.args") $LomodArgs -NoNewline
    Set-Content (Join-Path $installDir "version.txt") $OldVersion -NoNewline
    if ($InstalledLomoupg) {
        Write-Host "==> Updating with $InstalledLomoupg ($(& $InstalledLomoupg --version))"
        Copy-Item $InstalledLomoupg (Join-Path $installDir "lomoupg.exe") -Force
        # The installed lomorage-update.ps1 comes from the same release as that lomoupg.exe.
        # Releases before -only-newer don't pass it, and their lomoupg.exe would reject it.
        $updateScript = Join-Path $installDir "lomorage-update.ps1"
        if (-not ((& $InstalledLomoupg --help) -match "only-newer")) {
            Set-Content $updateScript ((Get-Content $updateScript -Raw) -replace "(?m)^\s*-only-newer ``\r?\n", "") -NoNewline
        }
    }

    $newRelease = Join-Path $sandbox "new-release"
    New-Release -Dir $newRelease -Version $NewVersion
    Publish-Release -Dir $newRelease -Version $NewVersion

    $server = Start-Job -ArgumentList $www, $Port -ScriptBlock {
        param($root, $port)
        $listener = [System.Net.HttpListener]::new()
        $listener.Prefixes.Add("http://localhost:$port/")
        $listener.Start()
        while ($listener.IsListening) {
            $ctx = $listener.GetContext()
            $file = Join-Path $root $ctx.Request.Url.AbsolutePath.TrimStart('/')
            if (Test-Path $file -PathType Leaf) {
                $bytes = [System.IO.File]::ReadAllBytes($file)
                $ctx.Response.ContentLength64 = $bytes.Length
                $ctx.Response.OutputStream.Write($bytes, 0, $bytes.Length)
            } else {
                $ctx.Response.StatusCode = 404
            }
            $ctx.Response.Close()
        }
    }
    if (-not (Wait-Until { try { (Invoke-WebRequest $releaseUrl -UseBasicParsing).StatusCode -eq 200 } catch { $false } })) {
        throw "local release server did not come up on port $Port"
    }

    Write-Host "==> 1. A process with the install dir as its working directory blocks the swap"
    $holder = Start-Process -FilePath "powershell.exe" -PassThru -WindowStyle Hidden -WorkingDirectory $installDir `
        -ArgumentList "-NoProfile", "-Command", "Start-Sleep 300"
    Start-Sleep -Seconds 2
    $code = Invoke-Update
    Assert ($code -ne 0) "update exits non-zero (got $code)"
    Assert ((Get-InstalledVersion) -eq $OldVersion) "version.txt still says the old version"
    Assert ((& (Join-Path $installDir "lomod.exe") --version) -eq $OldVersion) "old lomod.exe still in place"
    Assert ((Get-Backups).Count -eq 0) "no backup of the old release was made"
    Assert (-not (Test-Path (Join-Path $backupRoot "uncompress"))) "unzipped release cleaned up"
    Assert (Wait-Until { (Get-LogLines $startLog) -eq 1 }) "lomod was restarted after the failed swap"
    Stop-Process -Id $holder.Id -Force
    $holder = $null
    Start-Sleep -Seconds 1

    Write-Host "==> 2. An update check already in progress makes a second one step aside"
    $mutexName = "Local\Lomorage.Update." + ($installDir.ToLowerInvariant() -replace '[^a-z0-9]', '_')
    $mutex = New-Object System.Threading.Mutex($false, $mutexName)
    [void]$mutex.WaitOne(0)
    try {
        $stopsBefore = Get-LogLines $stopLog
        $code = Invoke-Update
    } finally {
        $mutex.ReleaseMutex()
        $mutex.Dispose()
    }
    Assert ($code -eq 0) "second check exits 0 (got $code)"
    Assert ((Get-Content (Join-Path $sandbox "update.out") -Raw) -match "already running") "second check says why it skipped"
    Assert ((Get-LogLines $stopLog) -eq $stopsBefore) "lomod was not stopped"
    Assert ((Get-InstalledVersion) -eq $OldVersion) "still on the old version"

    Write-Host "==> 3. The tray, started with the install dir as its working directory, updates on its own"
    # No Invoke-Update here: the tray's own check at startup has to do it. LOMOD_RELEASE_URL is
    # inherited by the tray and the update script it starts.
    $env:LOMOD_RELEASE_URL = $releaseUrl
    $trayProc = Start-Process -FilePath "powershell.exe" -PassThru -WindowStyle Hidden -WorkingDirectory $installDir `
        -ArgumentList "-NoProfile", "-WindowStyle", "Hidden", "-ExecutionPolicy", "Bypass", "-File", "`"$(Join-Path $installDir 'lomorage-tray.ps1')`"", "-UpdateCheckDelaySeconds", "3"
    Assert (Wait-Until -TimeoutSeconds 60 { (Test-Path (Join-Path $installDir "version.txt")) -and (Get-InstalledVersion) -eq $NewVersion }) "version.txt rewritten to the new version"
    Assert (-not $trayProc.HasExited) "tray is running"
    Assert ((& (Join-Path $installDir "lomod.exe") --version) -eq $NewVersion) "new lomod.exe in place"
    $nested = Join-Path $installDir "exiftool_files\lib\auto\Compress\Raw.dll"
    Assert ((Test-Path $nested -PathType Leaf) -and (Get-Content $nested -Raw) -eq $NewVersion) "nested directories unpacked"
    Assert (Test-Path (Join-Path $installDir "exiftool_files\empty") -PathType Container) "empty directory unpacked"
    Assert ((Test-Path (Join-Path $installDir "lomod.args")) -and (Get-Content (Join-Path $installDir "lomod.args") -Raw) -eq $LomodArgs) "lomod.args carried over"
    $backups = Get-Backups
    Assert ($backups.Count -eq 1) "old release kept as one backup"
    if ($backups.Count -eq 1) {
        Assert ((Get-Content (Join-Path $backups[0].FullName "version.txt") -Raw).Trim() -eq $OldVersion) "backup is the old release"
    }
    Assert (Wait-Until { (Get-LogLines $startLog) -eq 2 }) "lomod was restarted after the update"
    Assert (-not $trayProc.HasExited) "tray survived the update"

    Write-Host "==> 4. Nothing new: no-op"
    $stopsBefore = Get-LogLines $stopLog
    $code = Invoke-Update
    Assert ($code -eq 0) "update exits 0 (got $code)"
    Assert ((Get-LogLines $stopLog) -eq $stopsBefore) "lomod was not stopped"
    Assert ((Get-Backups).Count -eq 1) "no new backup"

    Write-Host "==> 5. The tray menu: status line, dimmed icon, Version, Check for Updates"
    # The menu can't be clicked from outside, so a driver loads the tray script into its own
    # process (Windows PowerShell, like the real tray), minus the startup at the bottom that
    # starts lomod and the logon update check, and clicks items with PerformClick. lomod's
    # running state is faked: the real Test-LomodRunning sees any lomod.exe on the machine.
    Stop-Process -Id $trayProc.Id -Force
    $trayProc = $null
    $newerRelease = Join-Path $sandbox "newer-release"
    New-Release -Dir $newerRelease -Version $NewerVersion
    Publish-Release -Dir $newerRelease -Version $NewerVersion
    $driver = Join-Path $sandbox "tray-driver.ps1"
    Set-Content $driver -Value @'
param([string]$InstallDir, [string]$NewVersion, [string]$NewerVersion)
$failures = 0
function Check($cond, $msg) {
    if ($cond) { Write-Output "ok   $msg" } else { Write-Output "FAIL $msg"; $script:failures++ }
}
function Wait-For([scriptblock]$Cond, [int]$Seconds) {
    $deadline = (Get-Date).AddSeconds($Seconds)
    while ((Get-Date) -lt $deadline) {
        [System.Windows.Forms.Application]::DoEvents()
        if (& $Cond) { return $true }
        Start-Sleep -Milliseconds 100
    }
    return $false
}

$src = Get-Content (Join-Path $InstallDir "lomorage-tray.ps1") -Raw
$cut = $src.IndexOf("`nStart-Lomod`r`nStart-UpdateCheck")
if ($cut -lt 0) { Write-Output "FAIL tray startup block not found -- update this test"; exit 1 }
. ([scriptblock]::Create($src.Substring(0, $cut))) -InstallDir $InstallDir

$script:fakeRunning = $true
function Test-LomodRunning { $script:fakeRunning }
$script:notes = New-Object System.Collections.ArrayList
function Show-Notification([string]$Text) { [void]$script:notes.Add($Text) }
function Click-CheckForUpdates {
    $script:notes.Clear()
    $updateItem.PerformClick()
    # The first notification just says the check started; wait for the outcome.
    Wait-For { $script:notes.Count -ge 2 -or ($script:notes.Count -eq 1 -and $script:notes[0] -ne "Checking for updates...") } 120 | Out-Null
    return $script:notes[$script:notes.Count - 1]
}

Update-Status
$statusTimer.Start()
Check ($statusItem.Text -eq "Lomorage is running") "status line says running"
Check ($notifyIcon.Icon -eq $icon -and $notifyIcon.Text -eq "Lomorage (running)") "full-color icon while running"
Check ((-not $startItem.Enabled) -and $stopItem.Enabled) "only Stop enabled while running"

$script:fakeRunning = $false
Check (Wait-For { $statusItem.Text -eq "Lomorage is stopped" } 8) "poll picks up lomod stopping"
Check ($notifyIcon.Icon -eq $dimmedIcon -and $notifyIcon.Text -eq "Lomorage (stopped)") "dimmed icon while stopped"
Check ($startItem.Enabled -and -not $stopItem.Enabled) "only Start enabled while stopped"
$script:fakeRunning = $true
Request-StatusRefresh
Check (Wait-For { $statusItem.Text -eq "Lomorage is running" } 2) "requested refresh lands within a second or so"

# Pixels of the rendered status line in the running dot's own green (52, 199, 89). A disabled
# menu item would draw it washed out, and ClearType text fringes are paler, so neither counts.
function Get-StatusDotGreenPixels {
    $menu.Show(0, 0)
    [System.Windows.Forms.Application]::DoEvents()
    $bmp = New-Object System.Drawing.Bitmap($menu.Width, $menu.Height)
    $menu.DrawToBitmap($bmp, (New-Object System.Drawing.Rectangle(0, 0, $menu.Width, $menu.Height)))
    $menu.Close()
    $b = $statusItem.Bounds
    $count = 0
    for ($x = $b.Left; $x -lt [Math]::Min($b.Right, $bmp.Width); $x++) {
        for ($y = $b.Top; $y -lt [Math]::Min($b.Bottom, $bmp.Height); $y++) {
            $c = $bmp.GetPixel($x, $y)
            if ([Math]::Abs($c.R - 52) -le 12 -and [Math]::Abs($c.G - 199) -le 12 -and [Math]::Abs($c.B - 89) -le 12) { $count++ }
        }
    }
    $bmp.Dispose()
    return $count
}

$green = Get-StatusDotGreenPixels
Check ($green -ge 20) "status line shows a green dot while running ($green green pixels)"
Check ($versionItem.Text -eq "Version $NewVersion") "Version line shows version.txt (got '$($versionItem.Text)')"
$script:fakeRunning = $false
$green = Get-StatusDotGreenPixels
Check ($green -eq 0) "no green dot while stopped ($green green pixels)"
$script:fakeRunning = $true
Update-Status

$note = Click-CheckForUpdates
Check ($note -eq "Updated to version $NewerVersion.") "Check for Updates installs the newer release (got '$note')"
Check ((Get-Content (Join-Path $InstallDir "version.txt") -Raw).Trim() -eq $NewerVersion) "version.txt says the newer version"

$note = Click-CheckForUpdates
Check ($note -eq "Lomorage is up to date (version $NewerVersion).") "second check reports up to date (got '$note')"

$goodUrl = $env:LOMOD_RELEASE_URL
$env:LOMOD_RELEASE_URL = "http://localhost:1/release.json"
$note = Click-CheckForUpdates
$env:LOMOD_RELEASE_URL = $goodUrl
Check ($note -eq "Update check failed. Lomorage is still on version $NewerVersion.") "unreachable release server reports a failed check (got '$note')"

$mutexName = "Local\Lomorage.Update." + ($InstallDir.ToLowerInvariant() -replace '[^a-z0-9]', '_')
$held = New-Object System.Threading.Mutex($true, $mutexName)
$script:notes.Clear()
$updateItem.PerformClick()
$held.ReleaseMutex()
$held.Dispose()
Check ($script:notes.Count -eq 1 -and $script:notes[0] -eq "An update check is already running.") "a check already running is reported, not started again"

$statusTimer.Stop()
$notifyIcon.Dispose()
exit $failures
'@
    $p = Start-Process -FilePath "powershell.exe" -PassThru -Wait -WindowStyle Hidden -WorkingDirectory $env:SystemRoot `
        -ArgumentList "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "`"$driver`"", "-InstallDir", "`"$installDir`"", "-NewVersion", $NewVersion, "-NewerVersion", $NewerVersion `
        -RedirectStandardOutput (Join-Path $sandbox "driver.out") -RedirectStandardError (Join-Path $sandbox "driver.err")
    foreach ($line in @(Get-Content (Join-Path $sandbox "driver.out")) + @(Get-Content (Join-Path $sandbox "driver.err"))) {
        if ($line -match '^ok   (.*)') { Assert $true $Matches[1] }
        elseif ($line -match '^FAIL (.*)') { Assert $false $Matches[1] }
        elseif ($line) { Write-Host "    | $line" }
    }
    Assert ($p.ExitCode -eq 0) "tray driver exits 0 (got $($p.ExitCode))"

    Write-Host "==> 6. A manifest pointing at an older release than the installed one: no downgrade"
    # What a machine sees when it switches from the nightly channel back to stable.
    $olderZip = Join-Path $www "release-$NewVersion.zip"
    @{ "windows-cli" = @{ Version = $NewVersion; URL = "http://localhost:$Port/release-$NewVersion.zip"; SHA256 = (Get-FileHash $olderZip -Algorithm SHA256).Hash.ToLower() } } |
        ConvertTo-Json | Set-Content (Join-Path $www "release.json")
    $stopsBefore = Get-LogLines $stopLog
    $backupsBefore = (Get-Backups).Count
    $code = Invoke-Update
    Assert ($code -eq 0) "update exits 0 (got $code)"
    Assert ((Get-Content (Join-Path $sandbox "update.out") -Raw) -match "is not newer than") "update says why it skipped"
    Assert ((Get-InstalledVersion) -eq $NewerVersion) "still on the newer release"
    Assert ((& (Join-Path $installDir "lomod.exe") --version) -eq $NewerVersion) "lomod.exe not replaced"
    Assert ((Get-LogLines $stopLog) -eq $stopsBefore) "lomod was not stopped"
    Assert ((Get-Backups).Count -eq $backupsBefore) "no new backup"
} finally {
    foreach ($p in $holder, $trayProc) {
        if ($p -and -not $p.HasExited) { Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue }
    }
    if ($server) { Stop-Job $server -ErrorAction SilentlyContinue; Remove-Job $server -Force -ErrorAction SilentlyContinue }
    Start-Sleep -Seconds 1
    Remove-Item -Recurse -Force $sandbox -ErrorAction SilentlyContinue
}

if ($script:failures -gt 0) {
    Write-Host "$($script:failures) check(s) failed" -ForegroundColor Red
    exit 1
}
Write-Host "All checks passed" -ForegroundColor Green
