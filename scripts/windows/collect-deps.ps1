<#
.SYNOPSIS
  Stages runtime dependencies for a lomod.exe Windows release bundle into -DestDir:
    - the libvips runtime DLL tree (copied from an already-fetched vips-dev bundle -- run
      fetch-vips.ps1 first, or via `make build-lomod-windows` which depends on it)
    - a standalone exiftool.exe (bundles its own Perl, no system Perl required)
    - static ffmpeg.exe / ffprobe.exe builds

.DESCRIPTION
  exiftool and ffmpeg downloads are SHA256-verified so a corrupted or truncated download
  fails loudly instead of silently shipping a broken binary in the release zip.

  exiftool is pinned to a fixed version, so its hash is pinned here too. ffmpeg comes from
  BtbN's rolling "latest" release, which is rebuilt about daily under the same file name, so
  a pinned hash went stale before nearly every release. Instead its expected hash is read
  from the checksums.sha256 BtbN publishes with each build. That still catches a bad
  download, but trusts BtbN's release as the source of truth rather than a hash reviewed in
  this repo. To pin one build instead, pass -FfmpegRelease with a dated BtbN tag (e.g.
  autobuild-2026-08-31-13-27 -- as observed in 2026-09, not a documented BtbN policy, one
  build per month stays up long-term while the daily ones are deleted after about two
  weeks), the -FfmpegZipName published under it, and its
  -FfmpegSha256. The LGPL (not GPL) build is used deliberately to keep bundling obligations
  light.

  Which ffmpeg build went in is written to ffmpeg-build.txt in -DestDir, so each release
  (and its notes, see release-all.sh) records it.

  Downloads with a pinned hash (exiftool, and ffmpeg when -FfmpegSha256 is given) are kept in
  -CacheDir once verified and reused by later builds, so a release only depends on SourceForge
  being up the first time a version is used. A download that fails or doesn't match its hash
  is retried a few times first: SourceForge sometimes answers with an error status or an HTML
  page in place of the file.

  exiftool is also kept unpacked in -ExifToolDir (windows-deps\exiftool-<version>), like the
  vips-dev bundle fetch-vips.ps1 stages: once it's there, builds use it without downloading
  anything, so SourceForge only matters when -ExifToolVersion is bumped. Delete the directory
  to make the next build fetch and check the zip again.
#>
param(
    [Parameter(Mandatory = $true)][string]$DestDir,
    # Defaults to windows-deps\vips (set below: see the windows-deps defaults there).
    [string]$VipsDir = "",

    [string]$ExifToolVersion = "13.59",
    [string]$ExifToolSha256 = "44b512b25af500724ba579d0a53c8fc5851628b692dd5e5d94ae4a15c2cba9ec",
    [string]$ExifToolUrl = "https://sourceforge.net/projects/exiftool/files/exiftool-${ExifToolVersion}_64.zip/download",
    # Defaults to windows-deps\exiftool-<version>.
    [string]$ExifToolDir = "",

    [string]$FfmpegRelease = "latest",
    [string]$FfmpegZipName = "ffmpeg-n9.0-latest-win64-lgpl-9.0.zip",
    # Empty: take the hash from the checksums.sha256 BtbN publishes with -FfmpegRelease.
    [string]$FfmpegSha256 = "",
    [string]$FfmpegReleaseUrl = "https://github.com/BtbN/FFmpeg-Builds/releases/download/$FfmpegRelease",

    # Defaults to windows-deps\downloads.
    [string]$CacheDir = "",
    [int]$DownloadAttempts = 3,
    [int]$RetryPauseSeconds = 60
)

$ErrorActionPreference = "Stop"

# Here, not as parameter defaults: Windows PowerShell 5.1 (what `make` runs this with) leaves
# $PSScriptRoot empty while evaluating those, which turned them into paths under C:\.
$windowsDeps = Join-Path $PSScriptRoot "..\..\windows-deps"
if (-not $VipsDir) { $VipsDir = Join-Path $windowsDeps "vips" }
if (-not $ExifToolDir) { $ExifToolDir = Join-Path $windowsDeps "exiftool-$ExifToolVersion" }
if (-not $CacheDir) { $CacheDir = Join-Path $windowsDeps "downloads" }

function Get-Sha256Hex {
    # Not Get-FileHash: on some machines PSModulePath ordering resolves Microsoft.PowerShell.Utility
    # to an incompatible PowerShell-7-targeted copy from a WindowsApps package, silently breaking
    # Get-FileHash under Windows PowerShell 5.1. Raw .NET has no module-resolution dependency.
    param([string]$Path)
    $sha256 = [System.Security.Cryptography.SHA256]::Create()
    try {
        $stream = [System.IO.File]::OpenRead($Path)
        try { $hashBytes = $sha256.ComputeHash($stream) } finally { $stream.Close() }
    } finally {
        $sha256.Dispose()
    }
    return -join ($hashBytes | ForEach-Object { $_.ToString("x2") })
}

function Get-File {
    param([string]$Url, [string]$OutFile)

    Write-Host "Downloading $Url ..."
    # curl.exe (built into Windows 10 1803+), not Invoke-WebRequest: SourceForge's Cloudflare
    # bot-protection serves Invoke-WebRequest an HTML interstitial instead of the real file.
    & curl.exe -fsSL -o $OutFile $Url
    if ($LASTEXITCODE -ne 0) { throw "curl.exe failed downloading $Url (exit $LASTEXITCODE)" }
}

# Returns the path of $FileName in -CacheDir, downloading it from $Url first unless a copy with
# the expected hash is already there. Only a verified download is moved into the cache.
function Get-VerifiedFile {
    param([string]$Url, [string]$ExpectedSha256, [string]$FileName)

    New-Item -ItemType Directory -Force -Path $CacheDir | Out-Null
    $cached = Join-Path $CacheDir $FileName
    $expected = $ExpectedSha256.ToLower()
    if ((Test-Path $cached) -and (Get-Sha256Hex -Path $cached) -eq $expected) {
        Write-Host "Using cached $cached"
        return $cached
    }
    $partial = "$cached.part"
    for ($attempt = 1; ; $attempt++) {
        try {
            Get-File -Url $Url -OutFile $partial
            $actual = Get-Sha256Hex -Path $partial
            if ($actual -eq $expected) {
                Move-Item -Path $partial -Destination $cached -Force
                return $cached
            }
            $problem = "SHA256 mismatch for $Url`nexpected: $expected`nactual:   $actual"
        } catch {
            $problem = $_.Exception.Message
        }
        Remove-Item $partial -Force -ErrorAction SilentlyContinue
        if ($attempt -ge $DownloadAttempts) { throw $problem }
        Write-Host "Download attempt $attempt/$DownloadAttempts failed, retrying in $RetryPauseSeconds s:`n$problem"
        Start-Sleep -Seconds $RetryPauseSeconds
    }
}

function Get-FfmpegPublishedSha256 {
    # Looks up $FileName in the checksums.sha256 BtbN publishes with $FfmpegRelease
    # (sha256sum lines: "<sha256>  <file name>", or "<sha256> *<file name>" in binary mode).
    param([string]$FileName)
    $sumsFile = Join-Path $env:TEMP "ffmpeg-checksums.sha256"
    try {
        Get-File -Url "$FfmpegReleaseUrl/checksums.sha256" -OutFile $sumsFile
        $line = Get-Content $sumsFile |
            Where-Object { (($_ -split '\s+', 2)[1] -replace '^\*', '') -eq $FileName } |
            Select-Object -First 1
    } finally {
        Remove-Item $sumsFile -Force -ErrorAction SilentlyContinue
    }
    if (-not $line) { throw "$FileName is not listed in BtbN's checksums.sha256 -- has the file name changed upstream?" }
    return ($line -split '\s+', 2)[0]
}

function Get-FfmpegZip {
    # Downloads $FileName and checks it against BtbN's checksums.sha256. A rebuild of a rolling
    # tag like "latest" uploads checksums.sha256 about a minute before the zips it describes, so
    # mid-rebuild the checksums can be newer than the zip (or the zip briefly missing). Retry
    # after a pause, re-fetching both, before calling it a bad download.
    param([string]$FileName, [string]$OutFile, [int]$Attempts = 3, [int]$PauseSeconds = 120)
    for ($attempt = 1; ; $attempt++) {
        try {
            $expected = Get-FfmpegPublishedSha256 -FileName $FileName
            Get-File -Url "$FfmpegReleaseUrl/$FileName" -OutFile $OutFile
            $actual = Get-Sha256Hex -Path $OutFile
            if ($actual -eq $expected.ToLower()) {
                Write-Host "ffmpeg SHA256 $actual matches BtbN's checksums.sha256"
                return $actual
            }
            $problem = "SHA256 mismatch for $OutFile`nBtbN checksums.sha256: $expected`nactual:                $actual"
        } catch {
            $problem = $_.Exception.Message
        }
        if ($attempt -ge $Attempts) { throw $problem }
        Write-Host "ffmpeg download attempt $attempt/$Attempts failed (BtbN may be mid-rebuild), retrying in $PauseSeconds s:`n$problem"
        Start-Sleep -Seconds $PauseSeconds
    }
}

New-Item -ItemType Directory -Force -Path $DestDir | Out-Null

# 1. libvips runtime DLLs
$vipsBin = Join-Path $VipsDir "bin"
if (-not (Test-Path $vipsBin)) {
    throw "vips-dev not found at $VipsDir -- run scripts/windows/fetch-vips.ps1 first"
}
Copy-Item -Path (Join-Path $vipsBin "*.dll") -Destination $DestDir -Force

# 2. exiftool (standalone Windows package: exiftool(-k).exe + supporting perl lib files)
# exiftool.exe is only a launcher since 12.88: it needs the bundled perl in exiftool_files/
# next to it, or every call fails with "Could not find ...perl5*.dll"
if ((Test-Path (Join-Path $ExifToolDir "exiftool.exe")) -and (Test-Path (Join-Path $ExifToolDir "exiftool_files"))) {
    Write-Host "exiftool $ExifToolVersion already present at $ExifToolDir, skipping download."
} else {
    $exiftoolZip = Get-VerifiedFile -Url $ExifToolUrl -ExpectedSha256 $ExifToolSha256 -FileName "exiftool-${ExifToolVersion}_64.zip"
    $exiftoolStage = Join-Path $env:TEMP "exiftool-stage"
    Remove-Item -Recurse -Force $exiftoolStage -ErrorAction SilentlyContinue
    Expand-Archive -Path $exiftoolZip -DestinationPath $exiftoolStage -Force
    $exiftoolExe = Get-ChildItem -Path $exiftoolStage -Filter "exiftool*.exe" -Recurse | Select-Object -First 1
    if (-not $exiftoolExe) { throw "exiftool.exe not found after extracting $exiftoolZip" }
    $exiftoolFiles = Join-Path $exiftoolExe.DirectoryName "exiftool_files"
    if (-not (Test-Path $exiftoolFiles)) { throw "exiftool_files not found next to $($exiftoolExe.FullName)" }
    # Staged next to -ExifToolDir and renamed into place, so an interrupted build never
    # leaves a half-copied directory that the next one would take as complete.
    $exiftoolNew = "$ExifToolDir.new"
    Remove-Item -Recurse -Force $exiftoolNew -ErrorAction SilentlyContinue
    New-Item -ItemType Directory -Force -Path $exiftoolNew | Out-Null
    Copy-Item -Path $exiftoolExe.FullName -Destination (Join-Path $exiftoolNew "exiftool.exe")
    Copy-Item -Path $exiftoolFiles -Destination (Join-Path $exiftoolNew "exiftool_files") -Recurse
    Remove-Item -Recurse -Force $ExifToolDir -ErrorAction SilentlyContinue
    Move-Item -Path $exiftoolNew -Destination $ExifToolDir
    Remove-Item -Recurse -Force $exiftoolStage
}
Copy-Item -Path (Join-Path $ExifToolDir "exiftool.exe") -Destination (Join-Path $DestDir "exiftool.exe") -Force
Remove-Item -Recurse -Force (Join-Path $DestDir "exiftool_files") -ErrorAction SilentlyContinue
Copy-Item -Path (Join-Path $ExifToolDir "exiftool_files") -Destination (Join-Path $DestDir "exiftool_files") -Recurse -Force

# 3. ffmpeg / ffprobe
# Only a pinned build is cached: a rolling tag like "latest" reuses one file name for each new
# build, so the cache could never tell a stale copy from the current one.
if ($FfmpegSha256) {
    $ffmpegZip = Get-VerifiedFile -Url "$FfmpegReleaseUrl/$FfmpegZipName" -ExpectedSha256 $FfmpegSha256 -FileName $FfmpegZipName
    $ffmpegSha = $FfmpegSha256.ToLower()
    $ffmpegZipIsCached = $true
} else {
    $ffmpegZip = Join-Path $env:TEMP $FfmpegZipName
    $ffmpegSha = Get-FfmpegZip -FileName $FfmpegZipName -OutFile $ffmpegZip
    $ffmpegZipIsCached = $false
}
$ffmpegStage = Join-Path $env:TEMP "ffmpeg-stage"
Remove-Item -Recurse -Force $ffmpegStage -ErrorAction SilentlyContinue
Expand-Archive -Path $ffmpegZip -DestinationPath $ffmpegStage -Force
$ffmpegExe = Get-ChildItem -Path $ffmpegStage -Filter "ffmpeg.exe" -Recurse | Select-Object -First 1
$ffprobeExe = Get-ChildItem -Path $ffmpegStage -Filter "ffprobe.exe" -Recurse | Select-Object -First 1
if (-not $ffmpegExe -or -not $ffprobeExe) { throw "ffmpeg.exe/ffprobe.exe not found after extracting $ffmpegZip" }
Copy-Item -Path $ffmpegExe.FullName -Destination (Join-Path $DestDir "ffmpeg.exe") -Force
Copy-Item -Path $ffprobeExe.FullName -Destination (Join-Path $DestDir "ffprobe.exe") -Force
if (-not $ffmpegZipIsCached) { Remove-Item $ffmpegZip -Force }
Remove-Item -Recurse -Force $ffmpegStage

# Record which ffmpeg build went in: with a rolling tag, two releases from the same commit can
# bundle different builds. Shipped in the zip, and quoted in the release notes by release-all.sh.
$ffmpegVersion = & (Join-Path $DestDir "ffmpeg.exe") -version | Select-Object -First 1
@(
    $ffmpegVersion,
    "BtbN/FFmpeg-Builds $FfmpegRelease $FfmpegZipName",
    "sha256 $ffmpegSha"
) | Set-Content -Path (Join-Path $DestDir "ffmpeg-build.txt") -Encoding ASCII
Write-Host "Bundled $ffmpegVersion"

Write-Host "Staged vips DLLs, exiftool.exe, ffmpeg.exe, ffprobe.exe into $DestDir"
