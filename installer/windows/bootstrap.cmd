@echo off
setlocal
title Aetheris Windows Core Installer
echo [Aetheris] Starting Windows Core installer...
echo [Aetheris] Progress and errors will remain visible in this window.
set "PAYLOAD=%~dp0payload.zip"
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "$ErrorActionPreference='Stop'; $temp = Join-Path $env:TEMP ('AetherisCore-' + [guid]::NewGuid().ToString()); try { Write-Host '[Aetheris] Extracting installation payload...'; Expand-Archive -LiteralPath '%PAYLOAD%' -DestinationPath $temp -Force; Write-Host '[Aetheris] Running automatic setup...'; & (Join-Path $temp 'install.ps1'); $code = 0 } catch { Write-Host ('[Aetheris][failed] ' + $_.Exception.Message) -ForegroundColor Red; $code = 1 } finally { if (Test-Path $temp) { Remove-Item -LiteralPath $temp -Recurse -Force } }; if ($code -eq 0) { Write-Host '[Aetheris][success] Installation complete.' -ForegroundColor Green } else { Write-Host '[Aetheris][failed] Check %LOCALAPPDATA%\Aetheris\logs\install.log' -ForegroundColor Red }; Read-Host 'Press Enter to close'; exit $code"
exit /b %ERRORLEVEL%
