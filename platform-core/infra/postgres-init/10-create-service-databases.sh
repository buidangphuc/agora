#!/bin/sh
# Runs once on first init of the SHARED postgres (docker-entrypoint-initdb.d).
# Creates one role + one database per service (logical DB-per-service on a single
# instance) — mirrors the kind gitops postgres seed. Idempotent.
set -e

SERVICES="identity listing search engagement order payment chat notification promotion audit referral sharing verification"

for svc in $SERVICES; do
  psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname postgres <<SQL
DO \$do\$ BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='${svc}_svc') THEN
    CREATE ROLE ${svc}_svc LOGIN PASSWORD '${svc}_pass';
  END IF;
END \$do\$;
SELECT 'CREATE DATABASE ${svc}_db OWNER ${svc}_svc'
  WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname='${svc}_db')\gexec
SQL
  echo "shared-postgres: ready ${svc}_db (owner ${svc}_svc)"
done
