#!/bin/bash
set -euo pipefail

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" <<SQL
CREATE ROLE replicator WITH REPLICATION LOGIN PASSWORD '${REPLICATION_PASSWORD}';
CREATE ROLE app WITH LOGIN PASSWORD '${APP_PASSWORD}';
SQL

for db in ${APP_DATABASES}; do
  echo "membuat database: ${db}"
  psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" -c "CREATE DATABASE \"${db}\" OWNER app;"
done

echo "host replication replicator all scram-sha-256" >> "$PGDATA/pg_hba.conf"