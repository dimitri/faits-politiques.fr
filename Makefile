DATABASE_URL ?= postgres://fp:fp@localhost:55432/fp?sslmode=disable
export DATABASE_URL

.PHONY: db-up db-down db-image db-dump db-restore migrate ingest build test reset site man fpctl

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
	@# Pas de `time` : c'est un mot-clé de bash, et make lance ses recettes avec
	@# /bin/sh — dash sous Debian — où il n'existe pas. La durée est mesurée à la
	@# main, sur une seule ligne, chaque ligne de recette ayant son propre shell.
	@debut=$$(date +%s); \
	docker compose exec -T db pg_dump -Fc -U fp -d fp > db/dump/fp.dump && \
	echo "dump : $$(( $$(date +%s) - debut )) s"
	@ls -lh db/dump/fp.dump

# -j : la restauration se parallélise, à la différence du dump en format
# custom. L'essentiel du temps part dans la reconstruction des index, en
# particulier les deux GIN plein texte du corpus du Journal officiel.
#
# Le script d'extensions est REJOUÉ après la restauration. Le dump ne porte que
# les extensions de la base SOURCE : restaurer une base de l'ancienne image
# (sans vector ni rum) dans la nouvelle laissait ces deux extensions absentes,
# et DROP/CREATE DATABASE saute le script d'initialisation de l'image. Ses
# CREATE EXTENSION IF NOT EXISTS ignorent ce que le dump a déjà recréé et
# ajoutent le reste. Le rejouer AVANT pg_restore ferait échouer ses propres
# CREATE EXTENSION, qui n'ont pas de IF NOT EXISTS.
db-restore: db-up ## restaure db/dump/fp.dump dans la base courante
	docker compose exec -T db psql -U fp -d postgres -c "DROP DATABASE IF EXISTS fp WITH (FORCE)"
	docker compose exec -T db psql -U fp -d postgres -c "CREATE DATABASE fp OWNER fp"
	@debut=$$(date +%s); \
	docker compose exec -T db pg_restore -U fp -d fp -j 4 --no-owner /dump/fp.dump; \
	rc=$$?; echo "restauration : $$(( $$(date +%s) - debut )) s (code $$rc)"; exit $$rc
	docker compose exec -T db psql -U fp -d fp -v ON_ERROR_STOP=1 \
		-f /docker-entrypoint-initdb.d/01-extensions.sql

fpctl:            ## compile le point d'entrée unique du projet (bin/fpctl)
	go build -o bin/fpctl ./cmd/fpctl

# Régénère les pages de manuel de fpctl depuis leurs sources Markdown
# (cmd/fpctl/man/*.md). Nécessite pandoc — un outil de développement, jamais
# une dépendance d'exécution : les .1 générés sont gravés dans le binaire
# (go:embed) et lus au vol par `man`, présent sur toute machine Unix.
man:              ## régénère les pages de manuel de fpctl (nécessite pandoc)
	@for f in cmd/fpctl/man/*.md; do \
		pandoc -s -t man "$$f" -o "$${f%.md}.1"; \
	done

migrate: db-up fpctl    ## applique les migrations
	./bin/fpctl ingest -only=migrate

ingest: db-up fpctl     ## télécharge, archive et charge les jeux de données
	./bin/fpctl ingest

build: fpctl            ## génère le site statique dans ./site
	./bin/fpctl build

verify: fpctl           ## contrôles de cohérence des données chargées
	./bin/fpctl verify

site: ingest verify build

test: db-up       ## rejoue les garanties structurelles sur la base chargée
	@for f in db/tests/*.sql; do \
		echo "--- $$f"; \
		docker compose exec -T db psql -v ON_ERROR_STOP=1 -U fp -d fp -f - < $$f | grep -E 'NOTICE|ERROR' || true; \
	done

reset: db-down    ## repart de zéro (détruit la base, garde l'archive brute)
	docker volume rm -f faits-politiquesfr_fp_pgdata 2>/dev/null || true
