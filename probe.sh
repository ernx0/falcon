#!/bin/sh
echo "R45-START"
i=0
while [ $i -lt 60 ]; do
  echo "=== CHECK $i ==="
  env | grep -i "database\|postgres\|pg\|dsn\|url\|conn" | sort
  i=$((i+1))
  sleep 10
done
echo "R45-DONE"
