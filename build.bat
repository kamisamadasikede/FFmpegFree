@echo off
setlocal

powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\build.ps1" %*
set "exit_code=%errorlevel%"

if not "%exit_code%"=="0" (
  echo.
  echo Build failed. Exit code: %exit_code%
  pause
  exit /b %exit_code%
)

echo.
echo Build completed successfully.
pause
