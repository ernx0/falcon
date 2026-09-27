#!/bin/sh
echo "R43-RUNTIME"
echo "=== BUILD PHASE OUTPUT ==="
cat /build_output.txt 2>/dev/null || echo "No build output file"
echo "=== END BUILD OUTPUT ==="

echo "--- RUNTIME ENV ---"
env | sort

echo "--- RUNTIME NET ---"
ip addr 2>/dev/null | head -20
ip route 2>/dev/null | head -10
cat /etc/resolv.conf 2>/dev/null

echo "--- DONE R43 ---"
sleep 600
