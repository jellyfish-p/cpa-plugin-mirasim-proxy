@echo off
echo Building CLIProxyAPI Mirasim Proxy Plugin...
set CGO_ENABLED=1
go build -buildmode=c-shared -o mirasim-proxy.dll .
if exist mirasim-proxy.h del mirasim-proxy.h
echo Build completed: mirasim-proxy.dll
