@echo off
setlocal
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0install.ps1" %*
if errorlevel 1 (
  echo Aetheris installation failed.
  exit /b 1
)
echo Aetheris installation completed.
endlocal

