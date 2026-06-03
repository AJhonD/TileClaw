@echo off
set APP=tiler.exe

tasklist /FI "IMAGENAME eq %APP%" 2>NUL | find /I "%APP%" >NUL 2>&1
if %ERRORLEVEL%==1 (
    echo tiler is not running
    exit /b 0
)

taskkill /IM "%APP%" /F >NUL 2>&1
echo tiler stopped
