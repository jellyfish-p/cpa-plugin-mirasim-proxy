@echo off
echo Building CLIProxyAPI Mirasim Dynamic Plugin (mirasim.dll)...
set CGO_ENABLED=1
go build -buildmode=c-shared -o mirasim.dll .
if %errorlevel% neq 0 (
    echo [ERROR] Build failed!
    exit /b %errorlevel%
)
if exist mirasim.h del mirasim.h
echo [SUCCESS] mirasim.dll built successfully!
