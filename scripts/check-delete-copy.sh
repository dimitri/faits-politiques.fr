#!/usr/bin/env bash
# Détecte le motif DELETE+COPY direct dans une table core.*/ref.* : un
# DELETE FROM sans condition (table entière), suivi d'un COPY qui vise la
# MÊME table réelle dans le même fichier. C'est le motif que ce dépôt a
# converti table par table, un peu à chaque session, jusqu'à ce qu'on
# décide de l'interdire structurellement plutôt que de compter sur la
# vigilance : voir la session qui a converti associations/hatvp/banatic/
# campagne/collectivites/municipal_list/amendements.
#
# La bonne forme est COPY dans une table temporaire (tmp_xxx, ou toute
# table déclarée par CREATE TEMP TABLE dans le même fichier) suivie d'un
# MERGE dans la vraie table — jamais un DELETE FROM <table réelle> suivi
# d'un COPY dans cette même table réelle.
#
# Un DELETE conditionnel (avec WHERE) n'est PAS signalé : il scope
# légitimement une portée (un scrutin, une institution) et n'est pas le
# motif visé ici tant qu'il ne précède pas un COPY dans la même table.
#
# Usage : scripts/check-delete-copy.sh [chemin...]
# Sans argument : scanne tout internal/. Sort en erreur (1) si une
# violation est trouvée, en listant fichier:table.

set -euo pipefail

roots=("$@")
if [ ${#roots[@]} -eq 0 ]; then
	roots=(internal)
fi

violations=0

while IFS= read -r -d '' f; do
	# Tables visées par un DELETE FROM SANS condition (pas de WHERE avant le
	# point-virgule ou la fin de la ligne SQL) sur core.* ou ref.*.
	mapfile -t deleted < <(grep -oE 'DELETE FROM (core|ref)\.[a-zA-Z_]+[^a-zA-Z_]' "$f" \
		| grep -v 'WHERE' \
		| sed -E 's/DELETE FROM ((core|ref)\.[a-zA-Z_]+).*/\1/' \
		| sort -u)
	[ ${#deleted[@]} -eq 0 ] && continue

	for t in "${deleted[@]}"; do
		schema="${t%%.*}"
		table="${t#*.}"
		# Le motif interdit : un COPY visant la même table réelle, exprimé
		# par pgx.Identifier{"schema", "table"} (jamais une table tmp_*).
		if grep -qE "pgx\.Identifier\{\"${schema}\",[[:space:]]*\"${table}\"\}" "$f"; then
			echo "VIOLATION: $f : DELETE FROM $t suivi d'un COPY direct dans $t"
			violations=$((violations + 1))
		fi
	done
done < <(find "${roots[@]}" -name '*.go' -not -name '*_test.go' -print0)

if [ "$violations" -gt 0 ]; then
	echo
	echo "$violations violation(s) : convertir en COPY dans une table temporaire + MERGE" >&2
	echo "(voir internal/communes/collectivites.go pour le patron)" >&2
	exit 1
fi

echo "OK : aucun DELETE+COPY direct détecté."
