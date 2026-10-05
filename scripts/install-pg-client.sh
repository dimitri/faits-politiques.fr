#!/usr/bin/env bash
# Installe les outils client PostgreSQL 17 (pg_dump, pg_restore, psql) via
# le dépôt PGDG — même source que docker/postgres.Dockerfile pour le
# serveur. Nécessaire en CI : le client par défaut d'un runner hébergé ne
# colle pas forcément à la version du serveur que fpctl provision db
# démarre, et pg_dump refuse d'emblée de dumper un serveur plus récent que
# lui-même (« aborting because of server version mismatch ») — pg_restore
# refuse de même un format d'archive plus récent que ce qu'il connaît.
set -euo pipefail

if command -v pg_dump >/dev/null && [[ "$(pg_dump --version)" == *") 17."* ]]; then
	echo "pg_dump 17 déjà présent, rien à faire."
	exit 0
fi

sudo install -d /usr/share/postgresql-common/pgdg
sudo curl -o /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc --fail \
	https://www.postgresql.org/media/keys/ACCC4CF8.asc
. /etc/os-release
echo "deb [signed-by=/usr/share/postgresql-common/pgdg/apt.postgresql.org.asc] https://apt.postgresql.org/pub/repos/apt ${VERSION_CODENAME}-pgdg main" \
	| sudo tee /etc/apt/sources.list.d/pgdg.list
sudo apt-get update
sudo apt-get install -y --no-install-recommends postgresql-client-17
