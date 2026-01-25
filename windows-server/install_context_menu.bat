@echo off
set "EXE_PATH=%~dp0k-share.exe"

echo 🚀 Adding "Send to Phone" to Context Menu...
echo 📂 Executable: %EXE_PATH%

reg add "HKEY_CLASSES_ROOT\*\shell\KShareSend" /ve /d "Send to Phone (K-Share)" /f
reg add "HKEY_CLASSES_ROOT\*\shell\KShareSend\command" /ve /d "\"%EXE_PATH%\" -send \"%%1\"" /f

echo.
echo ✅ Done! Right-click any file to see "Send to Phone (K-Share)"
pause
