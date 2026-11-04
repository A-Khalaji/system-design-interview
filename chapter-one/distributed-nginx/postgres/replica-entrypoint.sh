#!/bin/bash
set -e

if [ ! -s "$PGDATA/PG_VERSION" ]; then
    echo "Initializing replica from primary..."

    until pg_isready -h postgres -p 5432; do
        sleep 2
    done

    PGPASSWORD="$REPLICATION_PASSWORD" pg_basebackup \
        -h postgres \
        -p 5432 \
        -D "$PGDATA" \
        -U "$REPLICATION_USER" \
        -Fp \
        -Xs \
        -P \
        -R \
        -C \
        -S "$REPLICATION_SLOT"
fi

exec docker-entrypoint.sh postgres
