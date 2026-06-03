#!/bin/bash
set -e

APP="tiler"
PIDFILE="${APP}.pid"

if [ ! -f "$PIDFILE" ]; then
    echo "tiler is not running (no pid file)"
    exit 0
fi

PID=$(cat "$PIDFILE")
if kill "$PID" 2>/dev/null; then
    echo "tiler stopped (pid $PID)"
else
    echo "tiler was not running (stale pid $PID)"
fi
rm -f "$PIDFILE"
