REM go build -a -v -buildmode=c-shared

REM: download go1.12.8.windows-386.zip, unzip it. 

set PATH=c:/mingw/bin;%PATH%
set PATH=c:/Go32;%PATH%
set GOROOT=c:/Go32/
set GOARCH=386

c:/Go32/bin/go build -a -v -buildmode=c-archive main.go

c:/mingw/bin/gcc lomoupg.def main.a -shared -lwinmm -lWs2_32 -o lomoupg.dll -Wl,--out-implib,lomoupg.lib