<#
.SYNOPSIS
  Zips the contents of a directory with forward-slash entry names, as the zip format requires.

.DESCRIPTION
  Used by `make release-windows` instead of Compress-Archive: under Windows PowerShell 5.1
  that cmdlet writes backslash-separated names, including directory entries such as
  "exiftool_files\lib\auto\". Expand-Archive copes, but lomoupg.exe's unzip (Go's archive/zip)
  read those directory entries as files, so the scheduled self-update could never unpack a
  real release. lomoupg now tolerates such zips, but the lomoupg that performs an update is
  the one already installed, so the archive itself has to be well-formed too.

  Directory entries are only written for empty directories; everything else is implied by the
  file paths.
#>
param(
    [Parameter(Mandatory)][string]$SourceDir,
    [Parameter(Mandatory)][string]$ZipPath
)

$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.IO.Compression
Add-Type -AssemblyName System.IO.Compression.FileSystem

$root = (Resolve-Path -LiteralPath $SourceDir).Path.TrimEnd('\', '/')
$ZipPath = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($ZipPath)
if (Test-Path -LiteralPath $ZipPath) { Remove-Item -LiteralPath $ZipPath -Force }

$zip = [System.IO.Compression.ZipFile]::Open($ZipPath, [System.IO.Compression.ZipArchiveMode]::Create)
try {
    Get-ChildItem -LiteralPath $root -Recurse -Force | ForEach-Object {
        $name = $_.FullName.Substring($root.Length + 1).Replace('\', '/')
        if ($_.PSIsContainer) {
            if (-not (Get-ChildItem -LiteralPath $_.FullName -Force | Select-Object -First 1)) {
                [void]$zip.CreateEntry("$name/")
            }
        } else {
            [void][System.IO.Compression.ZipFileExtensions]::CreateEntryFromFile(
                $zip, $_.FullName, $name, [System.IO.Compression.CompressionLevel]::Optimal)
        }
    }
} finally {
    $zip.Dispose()
}
