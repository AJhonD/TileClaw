@echo off
set APP=tileclaw.exe
if "%CONF%"=="" set CONF=conf\conf.toml

tasklist /FI "IMAGENAME eq %APP%" 2>NUL | find /I "%APP%" >NUL 2>&1
if %ERRORLEVEL%==0 (
    echo tileclaw is already running
    exit /b 1
)

start /B "" "%APP%" -c "%CONF%" > app.log 2>&1
echo tileclaw started
