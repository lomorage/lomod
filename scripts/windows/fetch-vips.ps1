<#
.SYNOPSIS
  Downloads and extracts the prebuilt libvips Windows "dev" bundle (headers, pkgconfig files,
  import libs, and runtime DLLs) needed to CGO-compile lomod.exe and to stage its runtime DLLs.

.DESCRIPTION
  Source: https://github.com/libvips/build-win64-mxe releases -- the prebuilt Windows toolchain
  linked from https://www.libvips.org/install.html. Idempotent: skips the download if the
  requested version is already extracted at -VipsDir.

  Keep -VipsVersion in sync with whatever libvips version the Linux build links against
  (see the PREFIX_LOMO_VIPS docker image and recent "update vips to x.y" commits) -- govips talks
  to libvips through its stable C API, but mismatched major versions across platforms is still
  worth avoiding.

  No SHA256 pin is baked in here (unlike collect-deps.ps1's exiftool/ffmpeg downloads) because
  -VipsVersion is expected to move over time. Downloads are over HTTPS from GitHub; add a
  per-version hash file yourself if you want stronger supply-chain pinning.

  This bundle is built via MXE (cross-compiled from Linux), and its glib-2.0.pc declares a
  direct (non-private) dependency on -lintl, but ships no libintl import library or DLL of its
  own -- linking lomod.exe fails with "cannot find -lintl" otherwise. mingw-w64 toolchains do
  ship a working libintl-*.dll (just no import lib), so Ensure-LibIntl below generates one with
  gendef/dlltool (both part of a standard mingw-w64 install) and stages it into $VipsDir.
#>
param(
    [string]$VipsVersion = "8.18.5",
    [string]$VipsDir = "$PSScriptRoot/../../windows-deps/vips"
)

$ErrorActionPreference = "Stop"

function Ensure-LibIntl {
    param([string]$VipsDir)

    if (Test-Path (Join-Path $VipsDir "lib/libintl.a")) {
        return
    }

    $gcc = Get-Command gcc.exe -ErrorAction SilentlyContinue
    if (-not $gcc) {
        throw "gcc.exe not found on PATH -- put your mingw-w64 bin dir on PATH before running this script (Ensure-LibIntl needs its gendef/dlltool/libintl-*.dll)"
    }
    $mingwBin = Split-Path -Parent $gcc.Source
    $intlDll = Get-ChildItem -Path $mingwBin -Filter "libintl-*.dll" | Select-Object -First 1
    if (-not $intlDll) {
        throw "no libintl-*.dll found next to gcc.exe at $mingwBin -- this mingw-w64 build may not have NLS/gettext support"
    }

    Write-Host "Generating a libintl import library from $($intlDll.Name) (vips-dev doesn't ship one)"
    $work = Join-Path $env:TEMP "libintl-build"
    Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
    New-Item -ItemType Directory -Force -Path $work | Out-Null
    Copy-Item $intlDll.FullName (Join-Path $work $intlDll.Name)

    Push-Location $work
    try {
        & gendef.exe $intlDll.Name
        if ($LASTEXITCODE -ne 0) { throw "gendef.exe failed on $($intlDll.Name)" }
        $defFile = [IO.Path]::ChangeExtension($intlDll.Name, ".def")
        & dlltool.exe -d $defFile -D $intlDll.Name -l "libintl.a"
        if ($LASTEXITCODE -ne 0) { throw "dlltool.exe failed generating libintl.a" }
    } finally {
        Pop-Location
    }

    Copy-Item (Join-Path $work "libintl.a") (Join-Path $VipsDir "lib/libintl.a") -Force
    Copy-Item $intlDll.FullName (Join-Path (Join-Path $VipsDir "bin") $intlDll.Name) -Force
    Remove-Item -Recurse -Force $work
}

$pkgconfigMarker = Join-Path $VipsDir "lib/pkgconfig/vips.pc"
if (Test-Path $pkgconfigMarker) {
    Write-Host "vips-dev $VipsVersion already present at $VipsDir, skipping download."
    Ensure-LibIntl -VipsDir $VipsDir
    exit 0
}

$zipName = "vips-dev-x64-all-$VipsVersion.zip"
$url = "https://github.com/libvips/build-win64-mxe/releases/download/v$VipsVersion/$zipName"
$tmpZip = Join-Path $env:TEMP $zipName

Write-Host "Downloading $url ..."
# curl.exe (built into Windows 10 1803+), not Invoke-WebRequest -- see collect-deps.ps1 for why.
& curl.exe -fsSL -o $tmpZip $url
if ($LASTEXITCODE -ne 0) { throw "curl.exe failed downloading $url (exit $LASTEXITCODE)" }

# The archive wraps everything in a single top-level "vips-dev-<major.minor>" folder (not the
# full patch version), so extract to a scratch dir first and flatten that folder's contents into
# $VipsDir, rather than assuming the zip is already flat.
$extractTmp = Join-Path $env:TEMP "vips-dev-extract-$VipsVersion"
Remove-Item -Recurse -Force $extractTmp -ErrorAction SilentlyContinue
Expand-Archive -Path $tmpZip -DestinationPath $extractTmp -Force
Remove-Item $tmpZip -Force

New-Item -ItemType Directory -Force -Path $VipsDir | Out-Null
$inner = Get-ChildItem -Path $extractTmp -Directory | Select-Object -First 1
if (-not $inner) {
    throw "unexpected archive layout: no top-level folder found after extracting $zipName"
}
Get-ChildItem -Path $inner.FullName | Move-Item -Destination $VipsDir -Force
Remove-Item -Recurse -Force $extractTmp

if (-not (Test-Path $pkgconfigMarker)) {
    throw "vips.pc not found after extracting $zipName -- unexpected archive layout, check $VipsDir"
}

Ensure-LibIntl -VipsDir $VipsDir

Write-Host "vips-dev $VipsVersion staged at $VipsDir"
