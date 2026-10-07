package carto

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Les gouvernements de la Ve République, depuis le document officiel des
// services du Premier ministre.
//
// Seule la DATE DE DÉBUT est transcrite : le document mêle la date de nomination
// et celle de publication au Journal officiel, parfois à deux jours d'écart, et
// en extraire une date de fin produisait des gouvernements de quarante-huit
// heures. La fin est donc déduite — c'est le début du suivant — ce qui est vrai
// par construction et vérifiable.
//
// Les MINISTRES ne viennent pas de là. Le document d'origine est un RTF où la
// couleur du texte porte du sens — ministre jamais remplacé, poste vacant,
// nomination postérieure à la composition initiale — information perdue à
// l'extraction. Les attribuer à partir du texte brut produirait de fausses
// nominations. Ils viennent de l'open data de l'Assemblée, qui ne remonte
// qu'à 2007 : c'est une lacune assumée, pas un oubli.
func IngestGouvernements(ctx context.Context, pool *pgxpool.Pool, csvPath string) error {
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
		return fmt.Errorf("%s : aucun gouvernement", csvPath)
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

	type govRow struct{ name, rank, debut string }
	var govs []govRow
	for _, rec := range recs[1:] {
		x := govRow{get(rec, "premier_ministre"), get(rec, "rang"), get(rec, "debut")}
		if x.name == "" || x.debut == "" {
			continue
		}
		govs = append(govs, x)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM core.gouvernement`); err != nil {
		return err
	}

	var skipped int
	for i, x := range govs {
		fin := ""
		if i+1 < len(govs) {
			fin = govs[i+1].debut
		}
		// Deux gouvernements au même jour : la durée du premier serait nulle.
		// PostgreSQL accepte un intervalle vide sans broncher, mais aucune
		// requête ne le retrouve ensuite. Il est écarté et compté.
		if fin != "" && fin == x.debut {
			skipped++
			continue
		}
		name := "Gouvernement " + titleCase(x.name)
		if x.rank != "" {
			name += " " + x.rank
		}

		// Le Premier ministre est rapproché des personnes déjà connues, mais
		// seulement parmi celles qui ont détenu un mandat national : sans cette
		// restriction, un homonyme parmi les 500 000 élus locaux serait retenu.
		var pm any
		var id int64
		err := tx.QueryRow(ctx, `
			SELECT p.id FROM core.person p
			 WHERE core.f_unaccent(lower(p.family_name)) = core.f_unaccent(lower($1))
			   AND EXISTS (SELECT 1 FROM core.mandate m WHERE m.person_id = p.id
			               AND m.mandate_type IN ('DEPUTE','SENATEUR','MINISTRE',
			                                      'DEPUTE_EUROPEEN','PRESIDENT_REPUBLIQUE'))
			 ORDER BY p.id LIMIT 1`, familyName(x.name)).Scan(&id)
		if err == nil {
			pm = id
		} else if err != pgx.ErrNoRows {
			return err
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO core.gouvernement
			  (nom, premier_ministre_person_id, validity, source_url)
			VALUES ($1, $2, daterange($3::date, nullif($4,'')::date, '[)'), $5)`,
			name, pm, x.debut, fin,
			"https://www.data.gouv.fr/datasets/composition-des-gouvernements-de-la-veme-republique-1959-2014",
		); err != nil {
			return fmt.Errorf("%s : %w", name, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	logs.Notice(fmt.Sprintf("%s, from %s to %s", logs.Plural(len(govs)-skipped, "government"), govs[0].debut, govs[len(govs)-1].debut))
	if skipped > 0 {
		logs.Notice(fmt.Sprintf("%s skipped: same start date as the next one", logs.Plural(skipped, "entry")))
	}
	return nil
}

// titleCase remet en casse normale un nom écrit tout en capitales dans la source.
func titleCase(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, m := range words {
		r := []rune(m)
		if len(r) > 0 {
			words[i] = strings.ToUpper(string(r[0])) + string(r[1:])
		}
	}
	return strings.Join(words, " ")
}

// familyName isole le patronyme : le document écrit « Michel DEBRE », prénom
// d'abord, nom en capitales.
func familyName(s string) string {
	words := strings.Fields(s)
	var upper []string
	for _, m := range words {
		if m == strings.ToUpper(m) && len([]rune(m)) > 1 {
			upper = append(upper, m)
		}
	}
	if len(upper) > 0 {
		return strings.Join(upper, " ")
	}
	return s
}
