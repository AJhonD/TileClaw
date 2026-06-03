#!/bin/bash
set -e

APP="tiler"
CONF="${CONF:-conf.toml}"
PIDFILE="${APP}.pid"

if [ -f "$PIDFILE" ]; then
    PID=$(cat "$PIDFILE")
    if kill -0 "$PID" 2>/dev/null; then
        echo "tiler is already running (pid $PID)"
        exit 1
    fi
    rm -f "$PIDFILE"
fi

nohup ./"$APP" -c "$CONF" >> app.log 2>&1 &
echo $! > "$PIDFILE"
echo "tiler started (pid $!)"
