@echo off
setlocal

powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\run.ps1" %*
set "exit_code=%errorlevel%"

if not "%exit_code%"=="0" (
  echo.
  echo Run failed. Exit code: %exit_code%
  pause
  exit /b %exit_code%
)
