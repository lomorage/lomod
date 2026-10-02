REM # win-build.bat
REM # how to build zlib for lomod
REM # first need to install gcc and set PATH as below
REM # make -f win32/Makefile.gcc BINARY_PATH=/bin INCLUDE_PATH=/c/usr/local/include LIBRARY_PATH=/c/usr/local/lib install



REM # set pkg-config into path



REM # new machine build
REM  1. download go language from go.dev, and install with default settings.
REM  2. download choco from chocolatey.org, and install the pkgconfig like: >choco install pkgconfiglite, if meet issue, need run PowerShell terminal as Administrator.
REM     -- then inpu the script:  choco install pkgconfiglite
REM  3. download vips-dev-8 lib from https://github.com/libvips/libvips/releases?after=v8.8.2
REM  4. download mingw64 from https://sourceforge.net/projects/mingw-w64/files/ make sure 64bit  
REM  5. Config as below, then run this bat file. 

REM !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!
REM  if you got some issues like gcc link issues, please clean go-build cache like on %user%/appdata/local/go-build maybe.
REM  and mv libpng16.pc to libpng.pc under %PKG_CONFIG_PATH%..\lib


set PATH=c:/ProgramData/chocolatey/lib/pkgconfiglite/tools/pkg-config-lite-0.28-1/bin;%PATH%
set PATH=c:/mingw64/bin;%PATH%
set PKG_CONFIG_PATH=h:\myproject\lomoware\lomo-win\dependencies\vips-dev-8.8\lib\pkgconfig
set GOROOT=c:/Go/
set GOARCH=amd64

REM # this is 64 bit exe
c:/Go/bin/go build -v -x --tags "sqlite_trace trace" -ldflags "-s -w"