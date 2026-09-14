DATABASE_URL ?= postgres://fp:fp@localhost:55432/fp?sslmode=disable
export DATABASE_URL

.PHONY: db-up db-down db-image db-dump db-restore migrate ingest build test reset site

db-up:            ## démarre Postgres local
	docker compose up -d --wait db

db-down:
	docker compose down

# --- L'image PostgreSQL du projet ------------------------------------------
#
# Quatre extensions qu'aucune image publiée ne réunit : postgis (les contours
# dessinés par la base), vector (recherche par voisinage), rum (recherche de
# phrase avec positions) et unaccent (la configuration de recherche `fr`).
# Voir docker/postgres.Dockerfile.
#
# --network=host : sur cette machine, le réseau bridge de Docker n'a pas de DNS
# sortant — apt-get update y reste bloqué — alors que le réseau de l'hôte
# fonctionne. C'est une particularité locale, pas une exigence du Dockerfile :
# ailleurs, un `docker build` nu suffit.
db-image:         ## construit l'image PostgreSQL du projet
	docker build --network=host -f docker/postgres.Dockerfile \
		-t faits-politiques/postgres:17 .

# --- Sauvegarde et restauration --------------------------------------------
#
# Le répertoire de données de PostgreSQL N'EST PAS PORTABLE d'une image à
# l'autre : musl et glibc ne classent pas les chaînes de la même façon, donc les
# index ne sont pas dans le même ordre. Le passage se fait par un flux logique,
# jamais par copie du volume.
#
# pg_dump est celui du CONTENEUR, pas celui de l'hôte : l'archive doit être
# lisible par le pg_restore de la version cible.
db-dump: db-up    ## sauvegarde la base (format custom) dans db/dump/
	mkdir -p db/dump
	time docker compose exec -T db pg_dump -Fc -U fp -d fp > db/dump/fp.dump
	@ls -lh db/dump/fp.dump

# -j : la restauration se parallélise, à la différence du dump en format
# custom. L'essentiel du temps part dans la reconstruction des index, en
# particulier les deux GIN plein texte du corpus du Journal officiel.
db-restore: db-up ## restaure db/dump/fp.dump dans la base courante
	docker compose exec -T db psql -U fp -d postgres -c "DROP DATABASE IF EXISTS fp WITH (FORCE)"
	docker compose exec -T db psql -U fp -d postgres -c "CREATE DATABASE fp OWNER fp"
	time docker compose exec -T db pg_restore -U fp -d fp -j 4 --no-owner /dump/fp.dump

migrate: db-up    ## applique les migrations
	go run ./cmd/ingest -only=migrate

ingest: db-up     ## télécharge, archive et charge les jeux de données
	go run ./cmd/ingest

build:            ## génère le site statique dans ./site
	go run ./cmd/build

verify:            ## contrôles de cohérence des données chargées
	go run ./cmd/verify

site: ingest verify build

test: db-up       ## rejoue les garanties structurelles sur la base chargée
	@for f in db/tests/*.sql; do \
		echo "--- $$f"; \
		docker compose exec -T db psql -v ON_ERROR_STOP=1 -U fp -d fp -f - < $$f | grep -E 'NOTICE|ERROR' || true; \
	done

reset: db-down    ## repart de zéro (détruit la base, garde l'archive brute)
	docker volume rm -f faits-politiquesfr_fp_pgdata 2>/dev/null || true
