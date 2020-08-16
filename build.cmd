@echo off
set GO=go
set LDFLAGS="-s -w"
set TAGS="socks shadowsocks"
%GO% build -ldflags %LDFLAGS% -tags %TAGS% -v