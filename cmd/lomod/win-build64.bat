REM # win-build64.bat
REM # set pkg-config into path
REM # new machine build
REM  1. download go language1.13.15 from go.dev, and install with default settings.
REM  2. download choco from chocolatey.org, and install the pkgconfig like: 
REM        > choco install pkgconfiglite, if meet issue, need run PowerShell terminal as Administrator.
REM     -- then input the script:  choco install pkgconfiglite
REM  3. download https://github.com/lomolomo2/download-vips and run download-vips.bat
REM  4. download mingw64 from https://sourceforge.net/projects/mingw-w64/files/ make sure 64bit, 
REM      or  https://github.com/niXman/mingw-builds-binaries/releases/download/12.2.0-rt_v10-rev2/x86_64-12.2.0-release-win32-seh-msvcrt-rt_v10-rev2.7z
REM  5. install rice.   go get github.com/GeertJohan/go.rice/rice@latest
REM  6. Config as below, then run this bat file. 

REM !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!
REM  if you got some issues like gcc link issues, please clean go-build cache like on %user%/appdata/local/go-build maybe.
REM  and mv libpng16.pc to libpng.pc under %PKG_CONFIG_PATH%..\lib


REM change if need to update vips 
echo off

set BUILD_TYPE="release"

set SELF_HOMEDIR=%cd%\
echo %SELF_HOMEDIR%

REM change below if need, actually you do not need!!!!!!!
set VIPS_VERSION=vips-dev-cf23591
set SELF_LOMO_ROOT=%SELF_HOMEDIR%\..\..\..\
set VIPS_PK_CONFIG_PATH=%SELF_LOMO_ROOT%\download-vips\vips-dev-w64-all\%VIPS_VERSION%\
set VIPS_BIN_PATH=%VIPS_PK_CONFIG_PATH%\bin\

if %BUILD_TYPE% == "release" (
	echo "release"
	set LOMOD_PACKAGE_PATH=%SELF_LOMO_ROOT%\lomo-win\output\x64\bin\Release\Lomoagent\lomod\
) else (
	echo "debug"
	set LOMOD_PACKAGE_PATH=%SELF_LOMO_ROOT%\lomo-win\output\x64\bin\Debug\Lomoagent\lomod\
)

set PATH=c:/ProgramData/chocolatey/lib/pkgconfiglite/tools/pkg-config-lite-0.28-1/bin;%PATH%
set PATH=c:/mingw64/bin;c:/Users/%username%/go/bin;%PATH%
set PKG_CONFIG_PATH=%VIPS_PK_CONFIG_PATH%\lib\pkgconfig
set GOROOT=c:/Go/
set GOARCH=amd64

cd %SELF_HOMEDIR%
REM # this is 64 bit exe
cd ../../handler
rice embed-go

cd %SELF_HOMEDIR%

c:/Go/bin/go build -v -x --tags "sqlite_trace trace" -ldflags "-s -w"

if %ERRORLEVEL% neq 0 (
    echo An error occurred during the build process.
    exit /b %ERRORLEVEL%
)

cd %SELF_HOMEDIR%\..\lomoc
call win-build64.bat

copy /Y lomoc.exe ..\lomod\lomoc.exe

cd %SELF_HOMEDIR%

REM copy vips dll to package folder
robocopy %VIPS_BIN_PATH% %LOMOD_PACKAGE_PATH% /E

copy /Y lomod.exe %LOMOD_PACKAGE_PATH%
copy /Y lomoc.exe %LOMOD_PACKAGE_PATH%