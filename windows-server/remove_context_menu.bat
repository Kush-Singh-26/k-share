@echo off
echo 🗑️ Removing "Send to Phone" from Context Menu...

reg delete "HKEY_CLASSES_ROOT\*\shell\KShareSend" /f

echo.
echo ✅ Context menu item removed.
pause
