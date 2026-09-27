#!/bin/sh
echo "R41-RUNTIME"
echo "=== BUILD PHASE OUTPUT ==="
cat /build_output.txt 2>/dev/null || echo "No build output file"
echo "=== END BUILD OUTPUT ==="

echo "--- RUNTIME ENV ---"
env | sort

echo "--- DONE R41 ---"
sleep 600
