REM # win-build.bat
REM # how to build zlib for lomod
REM # first need to install gcc and set PATH as below
REM # make -f win32/Makefile.gcc BINARY_PATH=/bin INCLUDE_PATH=/c/usr/local/include LIBRARY_PATH=/c/usr/local/lib install



REM # set pkg-config into path

set PATH=c:/ProgramData/chocolatey/lib/pkgconfiglite/tools/pkg-config-lite-0.28-1/bin;%PATH%
set PATH=c:/mingw64/bin;%PATH%
set PKG_CONFIG_PATH=c:/xampp/htdocs/bitbucket/lomo-win/dependencies/vips-dev-8.8/lib/pkgconfig
set GOROOT=c:/Go/
set GOARCH=amd64

REM # this is 64 bit exe
c:/Go/bin/go build -v -x --tags "sqlite_trace trace"