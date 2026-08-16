@echo off
title Stop CLI Proxy API (Port 18318)
echo ============================================================
echo  Stopping CLI Proxy API instance on port 18318
echo ============================================================

set FOUND=0
for /f "tokens=5" %%a in ('netstat -aon ^| findstr ":18318 " ^| findstr /i "LISTENING"') do (
    set PID=%%a
    set FOUND=1
    echo Found process with PID: %%a listening on port 18318
    taskkill /F /T /PID %%a
)

if "%FOUND%"=="1" (
    echo ============================================================
    echo  CLI Proxy API on port 18318 has been stopped.
    echo ============================================================
) else (
    echo ============================================================
    echo  No active process found listening on port 18318.
    echo ============================================================
)

REM pause
