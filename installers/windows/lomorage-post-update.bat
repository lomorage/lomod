@echo off
rem Runs as lomoupg's --postcmd hook after a scheduled self-update swaps in a new lomod
rem release: lomoupg's --app-dir rename-swap only carries over what was in the freshly
rem downloaded zip, so lomod.args and version.txt -- written into InstallDir by install.ps1
rem at install time, not part of the zip -- are missing from the new directory. This restores
rem lomod.args from the just-renamed-away old install (the newest lomod-bak-* folder under
rem %1, lomoupg's --backup-dir) before starting lomod, and regenerates version.txt from the
rem new lomod.exe itself rather than copying the old one.
setlocal enabledelayedexpansion
set "SCRIPT_DIR=%~dp0"
set "BACKUP_ROOT=%~1"

set "LATEST_BAK="
if exist "%BACKUP_ROOT%" (
    for /f "delims=" %%D in ('dir /b /ad /o-d "%BACKUP_ROOT%\lomod-bak-*" 2^>nul') do (
        if not defined LATEST_BAK set "LATEST_BAK=%%D"
    )
)

if defined LATEST_BAK (
    if exist "%BACKUP_ROOT%\%LATEST_BAK%\lomod.args" (
        copy /y "%BACKUP_ROOT%\%LATEST_BAK%\lomod.args" "%SCRIPT_DIR%lomod.args" >nul
    )
)

if not exist "%SCRIPT_DIR%lomod.args" (
    echo lomod.args missing after update and no backup found to restore it from -- lomod.exe won't start until this is fixed manually 1>&2
    exit /b 1
)

for /f "usebackq delims=" %%V in (`"%SCRIPT_DIR%lomod.exe" --version 2^>nul`) do set "NEWVER=%%V"
if defined NEWVER (
    >"%SCRIPT_DIR%version.txt" (set /p "=%NEWVER%" <nul)
)

call "%SCRIPT_DIR%lomorage-start.bat"
endlocal
