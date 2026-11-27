#!/bin/sh
# Runs cwmerch's Flyway migrations for DB_TYPE: cwmerch-db/mysql (MySQL,
# MariaDB) or cwmerch-db/postgresql (PostgreSQL). Same DB_* variables as the API.
set -eu

type=$(echo "${DB_TYPE:-mysql}" | tr '[:upper:]' '[:lower:]')
host=${DB_HOST:-db}
name=${DB_NAME:-wordpress}
case "$type" in
  postgres|postgresql|pgsql|pg|psql)
    dir=postgresql
    url="jdbc:postgresql://${host}:${DB_PORT:-5432}/${name}?sslmode=${DB_SSLMODE:-disable}"
    ;;
  mysql|mariadb|my)
    dir=mysql
    url="jdbc:mysql://${host}:${DB_PORT:-3306}/${name}?allowPublicKeyRetrieval=true&useSSL=false"
    ;;
  *)
    echo "unsupported DB_TYPE '$type' (mysql, mariadb, postgres)" >&2
    exit 1
    ;;
esac

echo "cwmerch migrations: $dir on ${host}/${name}"
exec flyway "-url=$url" "-user=${DB_USER:-wordpress}" "-password=${DB_PASSWORD:-}" \
  "-placeholders.table_prefix=${DB_TABLE_PREFIX:-wp_}" \
  "-locations=filesystem:/flyway/sql/$dir" -connectRetries=20 migrate
