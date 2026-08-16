@echo off
title CLI Proxy API (Dev Server with Traffic Dump)
cd /d "%~dp0"

set "CONFIG_PATH=%~dp0..\..\config.yaml"
if not exist "%CONFIG_PATH%" set "CONFIG_PATH=%~dp0config.yaml"

echo ============================================================
echo  Starting CLI Proxy API (with Traffic Dump)
echo  Config : %CONFIG_PATH%
echo  Port   : 18318 (Override via --port to avoid conflict)
echo ============================================================
go run ./cmd/server --config "%CONFIG_PATH%" --port 18318

REM pause