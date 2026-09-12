DATABASE_URL ?= postgres://fp:fp@localhost:55432/fp?sslmode=disable
export DATABASE_URL

.PHONY: db-up db-down migrate ingest build test reset site

db-up:            ## démarre Postgres local
	docker compose up -d --wait db

db-down:
	docker compose down

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
