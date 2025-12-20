@echo off
REM build.bat - Build executables for Windows (run from Windows or WSL)

set VERSION=%date:~-4,4%%date:~-10,2%%date:~-7,2%
set DIST_DIR=dist
if not exist "%DIST_DIR%" mkdir "%DIST_DIR%"

echo Building xentz-agent for all platforms...
echo Version: %VERSION%
echo.

echo Building for Windows (amd64)...
set GOOS=windows
set GOARCH=amd64
go build -ldflags="-s -w" -o "%DIST_DIR%\xentz-agent-windows-amd64.exe" ./cmd/xentz-agent

echo Building for Windows (arm64)...
set GOOS=windows
set GOARCH=arm64
go build -ldflags="-s -w" -o "%DIST_DIR%\xentz-agent-windows-arm64.exe" ./cmd/xentz-agent

echo Building for Linux (amd64)...
set GOOS=linux
set GOARCH=amd64
go build -ldflags="-s -w" -o "%DIST_DIR%\xentz-agent-linux-amd64" ./cmd/xentz-agent

echo Building for Linux (arm64)...
set GOOS=linux
set GOARCH=arm64
go build -ldflags="-s -w" -o "%DIST_DIR%\xentz-agent-linux-arm64" ./cmd/xentz-agent

echo Building for macOS (amd64)...
set GOOS=darwin
set GOARCH=amd64
go build -ldflags="-s -w" -o "%DIST_DIR%\xentz-agent-darwin-amd64" ./cmd/xentz-agent

echo Building for macOS (arm64)...
set GOOS=darwin
set GOARCH=arm64
go build -ldflags="-s -w" -o "%DIST_DIR%\xentz-agent-darwin-arm64" ./cmd/xentz-agent

echo.
echo Build complete! Executables are in .\%DIST_DIR%\
dir /b "%DIST_DIR%\xentz-agent*"

