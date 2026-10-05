#!/usr/bin/env bash
# Installe les outils client PostgreSQL 17 (pg_dump, pg_restore, psql) via
# le dépôt PGDG — même source que docker/postgres.Dockerfile pour le
# serveur. Nécessaire en CI : le client par défaut d'un runner hébergé ne
# colle pas forcément à la version du serveur que fpctl provision db
# démarre, et pg_dump refuse d'emblée de dumper un serveur plus récent que
# lui-même (« aborting because of server version mismatch ») — pg_restore
# refuse de même un format d'archive plus récent que ce qu'il connaît.
set -euo pipefail

bindir=/usr/lib/postgresql/17/bin

if [ ! -x "$bindir/pg_dump" ]; then
	sudo install -d /usr/share/postgresql-common/pgdg
	sudo curl -o /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc --fail \
		https://www.postgresql.org/media/keys/ACCC4CF8.asc
	. /etc/os-release
	echo "deb [signed-by=/usr/share/postgresql-common/pgdg/apt.postgresql.org.asc] https://apt.postgresql.org/pub/repos/apt ${VERSION_CODENAME}-pgdg main" \
		| sudo tee /etc/apt/sources.list.d/pgdg.list
	sudo apt-get update
	sudo apt-get install -y --no-install-recommends postgresql-client-17
fi

# Le wrapper de postgresql-client-common choisit lui-même « la » version
# active, et ce n'est pas forcément celle-ci — constaté sur un runner où
# /usr/bin/pg_restore continuait de résoudre une version différente après
# cette installation. $bindir en tête de PATH, explicitement, pour les
# étapes suivantes : jamais à la merci de ce choix.
echo "$bindir" >> "$GITHUB_PATH"
"$bindir/pg_dump" --version
