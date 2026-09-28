#!/bin/sh
echo "=== R46 RUNTIME ==="
echo "--- BUILD OUTPUT ---"
cat /build_output.txt 2>/dev/null || echo "No build output"
echo "--- RUNTIME ENV ---"
env | sort
echo "--- DATABASE_URL ---"
echo "$DATABASE_URL"
echo "--- LOOPING ---"
i=0
while [ $i -lt 360 ]; do sleep 10; i=$((i+1)); done
