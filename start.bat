@echo off
set APP=tiler.exe
set CONF=conf.toml

tasklist /FI "IMAGENAME eq %APP%" 2>NUL | find /I "%APP%" >NUL 2>&1
if %ERRORLEVEL%==0 (
    echo tiler is already running
    exit /b 1
)

start /B "" "%APP%" -c "%CONF%" > app.log 2>&1
echo tiler started
