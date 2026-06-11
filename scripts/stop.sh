#!/bin/bash
set -e

APP="tileclaw"
PIDFILE="${APP}.pid"

if [ ! -f "$PIDFILE" ]; then
    echo "tileclaw is not running (no pid file)"
    exit 0
fi

PID=$(cat "$PIDFILE")
if kill "$PID" 2>/dev/null; then
    echo "tileclaw stopped (pid $PID)"
else
    echo "tileclaw was not running (stale pid $PID)"
fi
rm -f "$PIDFILE"
