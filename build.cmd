@echo off
set GO=go
set LDFLAGS="-s -w"
set TAGS="socks shadowsocks"
set BUILD=build
set PROGRAM=PProxy.exe
%GO% build -ldflags %LDFLAGS% -tags %TAGS% -v -o %BUILD%/%PROGRAM%