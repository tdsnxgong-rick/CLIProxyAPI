@echo off
title CLI Proxy API (Dev Server with Traffic Dump)
cd /d "%~dp0"
echo ============================================================
echo  Starting CLI Proxy API (with Traffic Dump)
echo  Config : C:\tools\others\CLIProxyAPI\cpa-core\config.yaml
echo  Port   : 18318 (Override via --port to avoid conflict)
echo ============================================================
go run ./cmd/server --config "C:\tools\others\CLIProxyAPI\cpa-core\config.yaml" --port 18318

REM pause