@echo off
rem Stops a running lomod.exe. Shipped next to lomod.exe in the release zip; also used as
rem lomoupg's --precmd hook before a self-update swaps in a new version.
rem /T: also take down anything lomod.exe spawned (ffmpeg, exiftool, ...). Left behind, a child
rem keeps the install directory as its working directory, which blocks lomoupg's rename of it.
taskkill /F /T /IM lomod.exe >nul 2>&1
exit /b 0
