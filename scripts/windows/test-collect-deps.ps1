<#
.SYNOPSIS
  Tests collect-deps.ps1's download cache and retries against a local server, without
  touching SourceForge, GitHub, or the real windows-deps download cache and unpacked exiftool.

.DESCRIPTION
  The server stands in for SourceForge and BtbN. It serves small fake exiftool and ffmpeg
  zips (their hashes are passed in as the pinned ones), and can be told to answer the next N
  requests for a file with an HTML page instead, the way SourceForge does when it's having
  trouble. It counts requests per file, so each scenario can check what was downloaded.

  Scenarios:
    1. Nothing cached, first exiftool answer is an HTML page: retried, both zips cached,
       exiftool unpacked into -ExifToolDir, deps staged.
    2. Second run: exiftool comes from -ExifToolDir and ffmpeg from the cache, nothing is
       downloaded.
    3. Without -ExifToolDir, a corrupted cached zip is downloaded again and replaced.
    4. Without either, a server that only ever sends HTML: fails after -DownloadAttempts
       tries, and leaves no cached zip, partial download or -ExifToolDir behind.

  Run: pwsh -File scripts/windows/test-collect-deps.ps1
#>
param(
    [string]$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path,
    [int]$Port = 8198
)

$ErrorActionPreference = "Stop"

$sandbox = Join-Path $env:TEMP ("collect-deps-test-" + [guid]::NewGuid().ToString("N").Substring(0, 8))
$www = Join-Path $sandbox "www"
$cache = Join-Path $sandbox "cache"
$exiftoolDir = Join-Path $sandbox "exiftool-13.59"
$baseUrl = "http://localhost:$Port"

$script:failures = 0
function Assert($cond, $msg) {
    if ($cond) { Write-Host "  ok   $msg" -ForegroundColor Green }
    else { Write-Host "  FAIL $msg" -ForegroundColor Red; $script:failures++ }
}

function New-Zip([string]$Name, [hashtable]$Files) {
    $stage = Join-Path $sandbox "stage-$Name"
    foreach ($rel in $Files.Keys) {
        $path = Join-Path $stage $rel
        New-Item -ItemType Directory -Force -Path (Split-Path $path -Parent) | Out-Null
        Copy-Item $Files[$rel] $path
    }
    $zip = Join-Path $www $Name
    Compress-Archive -Path (Join-Path $stage "*") -DestinationPath $zip
    return (Get-FileHash $zip -Algorithm SHA256).Hash.ToLower()
}

# The next $Count requests for $Name get an HTML page instead of the file.
function Set-Failures([string]$Name, [int]$Count) { Set-Content (Join-Path $www "$Name.fail") $Count -NoNewline }
function Get-Requests([string]$Name) {
    $f = Join-Path $www "$Name.count"
    if (Test-Path $f) { [int](Get-Content $f -Raw) } else { 0 }
}

function Invoke-CollectDeps([string]$DestDir) {
    $out = Join-Path $sandbox "collect.out"
    $p = Start-Process -FilePath "powershell.exe" -PassThru -Wait -WindowStyle Hidden `
        -ArgumentList "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "`"$(Join-Path $RepoRoot 'scripts\windows\collect-deps.ps1')`"",
            "-DestDir", "`"$DestDir`"", "-VipsDir", "`"$(Join-Path $sandbox 'vips')`"", "-CacheDir", "`"$cache`"", "-ExifToolDir", "`"$exiftoolDir`"",
            "-ExifToolUrl", "$baseUrl/exiftool.zip", "-ExifToolSha256", $exiftoolSha,
            "-FfmpegReleaseUrl", $baseUrl, "-FfmpegZipName", "ffmpeg.zip", "-FfmpegSha256", $ffmpegSha,
            "-RetryPauseSeconds", "1" `
        -RedirectStandardOutput $out -RedirectStandardError "$out.err"
    @(Get-Content $out) + @(Get-Content "$out.err") | Where-Object { $_ } | ForEach-Object { Write-Host "    | $_" }
    return $p.ExitCode
}

$server = $null
try {
    New-Item -ItemType Directory -Force -Path $www, (Join-Path $sandbox "vips\bin") | Out-Null
    Set-Content (Join-Path $sandbox "vips\bin\libvips-fake.dll") "dll" -NoNewline
    $fakeExe = Join-Path $env:SystemRoot "System32\hostname.exe"
    $perlDll = Join-Path $sandbox "perl.dll"
    Set-Content $perlDll "perl" -NoNewline
    $exiftoolSha = New-Zip "exiftool.zip" @{ "exiftool-13.59_64\exiftool(-k).exe" = $fakeExe; "exiftool-13.59_64\exiftool_files\perl.dll" = $perlDll }
    $ffmpegSha = New-Zip "ffmpeg.zip" @{ "ffmpeg\bin\ffmpeg.exe" = $fakeExe; "ffmpeg\bin\ffprobe.exe" = $fakeExe }

    $server = Start-Job -ArgumentList $www, $Port -ScriptBlock {
        param($root, $port)
        $listener = [System.Net.HttpListener]::new()
        $listener.Prefixes.Add("http://localhost:$port/")
        $listener.Start()
        while ($listener.IsListening) {
            $ctx = $listener.GetContext()
            $name = $ctx.Request.Url.AbsolutePath.TrimStart('/')
            $countFile = Join-Path $root "$name.count"
            $n = if (Test-Path $countFile) { [int](Get-Content $countFile -Raw) } else { 0 }
            Set-Content $countFile ($n + 1) -NoNewline
            $failFile = Join-Path $root "$name.fail"
            $fails = if (Test-Path $failFile) { [int](Get-Content $failFile -Raw) } else { 0 }
            if ($fails -gt 0) {
                Set-Content $failFile ($fails - 1) -NoNewline
                $bytes = [System.Text.Encoding]::ASCII.GetBytes("<html><head><title>SourceForge</title></head></html>")
            } else {
                $bytes = [System.IO.File]::ReadAllBytes((Join-Path $root $name))
            }
            $ctx.Response.ContentLength64 = $bytes.Length
            $ctx.Response.OutputStream.Write($bytes, 0, $bytes.Length)
            $ctx.Response.Close()
        }
    }
    $deadline = (Get-Date).AddSeconds(15)
    while ((Get-Date) -lt $deadline) {
        try { Invoke-WebRequest "$baseUrl/ffmpeg.zip" -UseBasicParsing | Out-Null; break } catch { Start-Sleep -Milliseconds 200 }
    }
    Remove-Item (Join-Path $www "ffmpeg.zip.count") -ErrorAction SilentlyContinue

    Write-Host "==> 1. Nothing cached, first exiftool answer is an HTML page"
    Set-Failures "exiftool.zip" 1
    $dest = Join-Path $sandbox "dist1"
    $code = Invoke-CollectDeps $dest
    Assert ($code -eq 0) "collect-deps succeeds (exit $code)"
    Assert ((Get-Requests "exiftool.zip") -eq 2) "exiftool downloaded twice: the HTML page, then the zip"
    Assert ((Get-Requests "ffmpeg.zip") -eq 1) "ffmpeg downloaded once"
    Assert ((Test-Path (Join-Path $cache "exiftool-13.59_64.zip")) -and (Test-Path (Join-Path $cache "ffmpeg.zip"))) "both zips cached"
    Assert ((Test-Path (Join-Path $exiftoolDir "exiftool.exe")) -and (Test-Path (Join-Path $exiftoolDir "exiftool_files\perl.dll"))) "exiftool unpacked into -ExifToolDir"
    Assert ((Test-Path (Join-Path $dest "exiftool.exe")) -and (Test-Path (Join-Path $dest "exiftool_files\perl.dll"))) "exiftool staged with exiftool_files"
    Assert ((Test-Path (Join-Path $dest "ffmpeg.exe")) -and (Test-Path (Join-Path $dest "ffprobe.exe"))) "ffmpeg and ffprobe staged"
    Assert (@(Get-ChildItem $cache -Filter "*.part").Count -eq 0) "no partial download left in the cache"

    Write-Host "==> 2. Second run uses -ExifToolDir and the cache"
    $code = Invoke-CollectDeps (Join-Path $sandbox "dist2")
    Assert ($code -eq 0) "collect-deps succeeds (exit $code)"
    Assert ((Get-Requests "exiftool.zip") -eq 2 -and (Get-Requests "ffmpeg.zip") -eq 1) "nothing downloaded"
    Assert (Test-Path (Join-Path $sandbox "dist2\exiftool_files\perl.dll")) "exiftool staged from -ExifToolDir"

    Write-Host "==> 3. Without -ExifToolDir, a corrupted cached zip is replaced"
    Remove-Item -Recurse -Force $exiftoolDir
    Set-Content (Join-Path $cache "exiftool-13.59_64.zip") "truncated" -NoNewline
    $code = Invoke-CollectDeps (Join-Path $sandbox "dist3")
    Assert ($code -eq 0) "collect-deps succeeds (exit $code)"
    Assert ((Get-Requests "exiftool.zip") -eq 3) "exiftool downloaded again"
    Assert ((Get-FileHash (Join-Path $cache "exiftool-13.59_64.zip") -Algorithm SHA256).Hash.ToLower() -eq $exiftoolSha) "cache holds the good zip again"

    Write-Host "==> 4. Only HTML pages: gives up after the retries"
    Remove-Item -Recurse -Force $exiftoolDir
    Remove-Item (Join-Path $cache "exiftool-13.59_64.zip")
    Set-Failures "exiftool.zip" 99
    $code = Invoke-CollectDeps (Join-Path $sandbox "dist4")
    Assert ($code -ne 0) "collect-deps fails (exit $code)"
    Assert ((Get-Requests "exiftool.zip") -eq 6) "tried 3 times"
    Assert (-not (Test-Path (Join-Path $cache "exiftool-13.59_64.zip"))) "nothing cached"
    Assert (@(Get-ChildItem $cache -Filter "*.part").Count -eq 0) "no partial download left in the cache"
    Assert (-not (Test-Path $exiftoolDir)) "no -ExifToolDir created"
} finally {
    if ($server) { Stop-Job $server -ErrorAction SilentlyContinue; Remove-Job $server -Force -ErrorAction SilentlyContinue }
    Remove-Item -Recurse -Force $sandbox -ErrorAction SilentlyContinue
}

if ($script:failures -gt 0) {
    Write-Host "$($script:failures) check(s) failed" -ForegroundColor Red
    exit 1
}
Write-Host "All checks passed" -ForegroundColor Green
