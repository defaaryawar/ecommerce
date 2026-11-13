#!/bin/bash
set -euo pipefail

if [ ! -s "$PGDATA/PG_VERSION" ]; then
  echo "menunggu primary ${PRIMARY_HOST}..."
  until pg_isready -h "$PRIMARY_HOST" -U postgres >/dev/null 2>&1; do sleep 1; done

  echo "clone data dari ${PRIMARY_HOST}..."
  rm -rf "${PGDATA:?}"/*
  PGPASSWORD="$REPLICATION_PASSWORD" pg_basebackup \
    -h "$PRIMARY_HOST" -U replicator -D "$PGDATA" -Fp -Xs -P -R
  chmod 0700 "$PGDATA"
fi

exec docker-entrypoint.sh postgres -c hot_standby=on