package carto

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Les présidents de la Ve République entrent en base comme des mandats.
//
// Jusqu'ici ils ne vivaient que dans data/presidents.csv, lu au moment du
// rendu : aucune requête ne pouvait demander « quels mandats se sont déroulés
// sous telle présidence ». C'est pourtant ce qui donne son sens à une frise de
// carrière — un mandat de ministre sans la présidence qui l'encadre est une
// date sans contexte.
//
// Ce n'est PAS une imputation. Un ministre n'est pas responsable des actes du
// président, ni l'inverse : la concomitance est un repère chronologique, et
// toute page qui l'affiche doit le dire.
//
// Les intérims du président du Sénat sont inclus. Les omettre laisserait des
// trous dans la chronologie, et un mandat tombant dans un trou n'aurait aucun
// contexte du tout.
func IngestPresidents(ctx context.Context, pool *pgxpool.Pool, csvPath string) error {
	f, err := os.Open(csvPath)
	if err != nil {
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comment = '#'
	r.FieldsPerRecord = -1
	recs, err := r.ReadAll()
	if err != nil {
		return err
	}
	if len(recs) < 2 {
		return fmt.Errorf("%s : aucune présidence", csvPath)
	}
	idx := map[string]int{}
	for i, h := range recs[0] {
		idx[strings.TrimSpace(h)] = i
	}
	get := func(rec []string, k string) string {
		if i, ok := idx[k]; ok && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`DELETE FROM core.mandate WHERE mandate_type = 'PRESIDENT_REPUBLIQUE'`); err != nil {
		return err
	}

	var n int
	for _, rec := range recs[1:] {
		nom := get(rec, "nom")
		debut := get(rec, "debut")
		if nom == "" || debut == "" {
			continue
		}
		famille, prenom := decouperNom(nom)

		// Un président est très souvent déjà en base comme député ou ministre.
		// Le rapprochement se fait sur le nom complet exact, insensible aux
		// accents et à la casse — et uniquement parmi les personnes qui ont
		// déjà un mandat national, pour ne pas capter un homonyme parmi les
		// 500 000 élus locaux.
		var personID int64
		err := tx.QueryRow(ctx, `
			SELECT p.id FROM core.person p
			 WHERE core.f_unaccent(lower(p.family_name)) = core.f_unaccent(lower($1))
			   AND core.f_unaccent(lower(p.given_name))  = core.f_unaccent(lower($2))
			   AND EXISTS (SELECT 1 FROM core.mandate m
			               WHERE m.person_id = p.id
			                 AND m.mandate_type IN ('DEPUTE','SENATEUR','MINISTRE','DEPUTE_EUROPEEN'))
			 ORDER BY p.id LIMIT 1`, famille, prenom).Scan(&personID)
		if err == pgx.ErrNoRows {
			// ON CONFLICT plutôt qu'un simple INSERT : un même nom peut revenir
			// plusieurs fois dans le fichier. Alain Poher a assuré deux intérims,
			// en 1969 et en 1974 — deux périodes, une seule personne.
			if err := tx.QueryRow(ctx, `
				INSERT INTO core.person (slug, family_name, given_name)
				VALUES ($1, $2, $3)
				ON CONFLICT (slug) DO UPDATE SET slug = EXCLUDED.slug
				RETURNING id`,
				"president-"+slugFR(nom), famille, prenom).Scan(&personID); err != nil {
				return fmt.Errorf("%s : %w", nom, err)
			}
		} else if err != nil {
			return err
		}

		// Borne haute exclusive : la passation se fait le jour dit, et le
		// successeur commence ce même jour. Un intervalle fermé ferait se
		// chevaucher deux présidences d'une journée.
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.mandate (person_id, mandate_type, validity, role)
			VALUES ($1, 'PRESIDENT_REPUBLIQUE',
			        daterange($2::date, nullif($3,'')::date, '[)'), $4)`,
			personID, debut, get(rec, "fin"), get(rec, "qualite")); err != nil {
			return fmt.Errorf("%s : %w", nom, err)
		}
		n++
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	fmt.Printf("  %d périodes de présidence\n", n)
	return nil
}

// decouperNom sépare « Valéry Giscard d'Estaing » en prénom et nom de famille.
// Le premier mot est le prénom : c'est la convention du fichier, qui est écrit
// à la main et vérifié.
func decouperNom(nom string) (famille, prenom string) {
	parts := strings.SplitN(nom, " ", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[1], parts[0]
}

func slugFR(s string) string {
	s = strings.ToLower(s)
	for from, to := range map[string]string{
		"à": "a", "â": "a", "ç": "c", "é": "e", "è": "e", "ê": "e", "ë": "e",
		"î": "i", "ï": "i", "ô": "o", "ö": "o", "ù": "u", "û": "u", "ü": "u",
	} {
		s = strings.ReplaceAll(s, from, to)
	}
	var b strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			b.WriteRune(c)
		} else if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
