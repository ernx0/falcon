#!/bin/sh
echo "R42-RUNTIME"
echo "=== BUILD PHASE OUTPUT ==="
cat /build_output.txt 2>/dev/null || echo "No build output file"
echo "=== END BUILD OUTPUT ==="

echo "--- RUNTIME ENV ---"
env | sort

echo "--- DONE R42 ---"
sleep 600
