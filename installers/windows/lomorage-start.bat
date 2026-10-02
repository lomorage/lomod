@echo off
rem Starts lomod.exe with the flags install.ps1 wrote to lomod.args at install time.
rem Shipped next to lomod.exe in the release zip; also used as lomoupg's --postcmd hook
rem after a self-update swaps in a new version, and as the target of the per-user
rem Startup-folder autostart shortcut.
setlocal
set "SCRIPT_DIR=%~dp0"

if not exist "%SCRIPT_DIR%lomod.args" (
    echo lomod.args not found next to lomorage-start.bat, cannot start lomod.exe 1>&2
    exit /b 1
)

rem Delegates to lomorage-start-hidden.vbs, which launches lomod.exe with window style 0 (no
rem console/taskbar entry at all). Plain `start /min lomod.exe` still creates a visible,
rem minimized window that lingers in the taskbar for as long as lomod.exe keeps running.
wscript.exe "%SCRIPT_DIR%lomorage-start-hidden.vbs"
endlocal
