// Package campagne charge les comptes de campagne publiés par la Commission
// nationale des comptes de campagne et des financements politiques.
//
// Le jeu contient, pour chaque candidat, ce qu'il a DÉCLARÉ avoir dépensé et
// reçu, et ce que la Commission a RETENU après contrôle. L'écart entre les deux
// est le fait le plus intéressant : il dit ce qui a été rejeté.
//
// Aucun rattachement automatique à une personne n'est fait. Le fichier ne porte
// ni date de naissance ni identifiant, et 41,9 % des élus de notre base ont un
// homonyme exact. Un lien nom-à-nom produirait des attributions fausses sur un
// sujet — le financement — où une erreur est particulièrement coûteuse.
package campagne

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ConnectorVersion = "campagne-v1"

var Source = archive.Source{
	Slug: "cnccfp-campagne", Label: "CNCCFP — comptes de campagne",
	Publisher:   "Commission nationale des comptes de campagne et des financements politiques",
	Tier:        "PRIMARY_OFFICIAL",
	Licence:     "Aucune licence déclarée par le producteur",
	ReuseClass:  "RESTRICTED",
	Attribution: "Source : Commission nationale des comptes de campagne et des financements politiques",
	Cadence:     "par scrutin",
	Notes: "Licence non déclarée : même situation que les résultats municipaux de " +
		"2020. Classé RESTRICTED en attendant une décision explicite ; à ne pas " +
		"reverser dans un export ouvert.",
}

// Un scrutin par fichier. Les URL sont fixées : c'est cette version-là qui est
// scellée dans l'archive.
var scrutins = []struct {
	typeElection string
	annee        int
	url          string
}{
	{"LEGISLATIVE", 2022,
		"https://static.data.gouv.fr/resources/comptes-de-campagne-elections-legislatives-generales-des-12-et-19-juin-2022/20231012-135051/publications-2022-lg.csv"},
}

func Ingest(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, Source)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersion)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_compte (
			type_election text, annee int, candidat_ref text, candidat_nom text,
			circonscription text, departement text, code_departement text, nuance text,
			monnaie text, depenses_declarees numeric, depenses_retenues numeric,
			recettes_declarees numeric, recettes_retenues numeric
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}

	var nComptes, nPostes int
	for _, s := range scrutins {
		f, err := arch.Fetch(ctx, srcID, runID, s.url, ".csv")
		if err != nil {
			return fail(fmt.Errorf("%s %d : %w", s.typeElection, s.annee, err))
		}
		recs, entetes, err := lireCSV(f.Path)
		if err != nil {
			return fail(err)
		}
		if _, err := tx.Exec(ctx,
			`DELETE FROM core.compte_campagne WHERE type_election=$1 AND annee=$2`,
			s.typeElection, s.annee); err != nil {
			return fail(err)
		}
		if _, err := tx.Exec(ctx, `TRUNCATE tmp_compte`); err != nil {
			return fail(err)
		}

		var comptes []map[string]string
		var compteRows [][]any
		for _, r := range recs {
			nom := strings.TrimSpace(r["nom"])
			if nom == "" {
				continue
			}
			comptes = append(comptes, r)
			compteRows = append(compteRows, []any{
				s.typeElection, s.annee, nul(r["candidat"]), nom,
				nul(r["circonscription"]), nul(r["département"]), nul(r["code département"]),
				nul(r["nuance"]), nul(r["monnaie"]),
				montant(r["dépenses totales déclarées"]), montant(r["depenses totales retenues"]),
				montant(r["recettes totales déclarées"]), montant(r["recettes totales retenues"]),
			})
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_compte"},
			[]string{"type_election", "annee", "candidat_ref", "candidat_nom", "circonscription",
				"departement", "code_departement", "nuance", "monnaie", "depenses_declarees",
				"depenses_retenues", "recettes_declarees", "recettes_retenues"},
			pgx.CopyFromRows(compteRows)); err != nil {
			return fail(err)
		}
		res, err := tx.Query(ctx, `
			WITH upsert AS (
				INSERT INTO core.compte_campagne
				  (type_election, annee, candidat_ref, candidat_nom, circonscription,
				   departement, code_departement, nuance, monnaie,
				   depenses_declarees, depenses_retenues, recettes_declarees, recettes_retenues, source_id)
				SELECT type_election, annee, candidat_ref, candidat_nom, circonscription,
				       departement, code_departement, nuance, monnaie,
				       depenses_declarees, depenses_retenues, recettes_declarees, recettes_retenues, $1
				  FROM tmp_compte
				ON CONFLICT (type_election, annee, candidat_nom, circonscription) DO NOTHING
				RETURNING id, candidat_nom, circonscription
			)
			SELECT candidat_nom, coalesce(circonscription, ''), id FROM upsert`, srcID)
		if err != nil {
			return fail(err)
		}
		idParCandidat := map[[2]string]int64{}
		for res.Next() {
			var nom, circo string
			var id int64
			if err := res.Scan(&nom, &circo, &id); err != nil {
				res.Close()
				return fail(err)
			}
			idParCandidat[[2]string{nom, circo}] = id
		}
		res.Close()
		if err := res.Err(); err != nil {
			return fail(err)
		}
		nComptes += len(idParCandidat)

		// Les postes : toute colonne suffixée « (déclaré) » ou « (retenu) ».
		// Le libellé est conservé tel quel, sans regroupement. Un compte en
		// doublon (nom+circonscription déjà pris) n'a pas d'id : ses postes
		// sont ignorés, comme avant.
		type clePoste struct {
			id          int64
			poste, etat string
		}
		vusPostes := map[clePoste]bool{}
		var posteRows [][]any
		for _, r := range comptes {
			id, ok := idParCandidat[[2]string{strings.TrimSpace(r["nom"]), strings.TrimSpace(r["circonscription"])}]
			if !ok {
				continue
			}
			for _, h := range entetes {
				var etat string
				var poste string
				switch {
				case strings.HasSuffix(h, "(déclaré)"):
					etat, poste = "DECLARE", strings.TrimSpace(strings.TrimSuffix(h, "(déclaré)"))
				case strings.HasSuffix(h, "(retenu)"):
					etat, poste = "RETENU", strings.TrimSpace(strings.TrimSuffix(h, "(retenu)"))
				default:
					continue
				}
				m := montant(r[h])
				if m == nil {
					continue
				}
				// Un en-tête dupliqué dans le CSV source produirait la même
				// clé (compte_id, poste, etat) : la première valeur gagne,
				// comme le faisait l'ON CONFLICT DO NOTHING ligne à ligne.
				cle := clePoste{id, poste, etat}
				if vusPostes[cle] {
					continue
				}
				vusPostes[cle] = true
				posteRows = append(posteRows, []any{id, poste, etat, m})
			}
		}
		n, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "compte_campagne_poste"},
			[]string{"compte_id", "poste", "etat", "montant"}, pgx.CopyFromRows(posteRows))
		if err != nil {
			return fail(fmt.Errorf("postes : %w", err))
		}
		nPostes += int(n)
		logs.Notice(fmt.Sprintf("campaign accounts %s %d: %s", s.typeElection, s.annee,
			logs.Plural(len(idParCandidat), "account")))
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"comptes": nComptes, "postes": nPostes}, "")
	logs.Notice(fmt.Sprintf("campaign accounts done: %s, %s",
		logs.Plural(nComptes, "account"), logs.Plural(nPostes, "line item")))
	return nil
}

func lireCSV(path string) ([]map[string]string, []string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	texte := string(b)
	if !utf8.Valid(b) {
		r := make([]rune, len(b))
		for i, c := range b {
			r[i] = rune(c)
		}
		texte = string(r)
	}
	rd := csv.NewReader(strings.NewReader(texte))
	rd.Comma = ';'
	rd.FieldsPerRecord = -1
	rd.LazyQuotes = true
	recs, err := rd.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("%s : %w", path, err)
	}
	if len(recs) < 2 {
		return nil, nil, fmt.Errorf("%s : aucun compte", path)
	}
	head := make([]string, len(recs[0]))
	for i, h := range recs[0] {
		head[i] = strings.TrimSpace(strings.Trim(h, "\ufeff\""))
	}
	out := make([]map[string]string, 0, len(recs)-1)
	for _, rec := range recs[1:] {
		m := make(map[string]string, len(head))
		for i, h := range head {
			if i < len(rec) {
				m[h] = strings.TrimSpace(rec[i])
			}
		}
		out = append(out, m)
	}
	return out, head, nil
}

func montant(s string) any {
	s = strings.NewReplacer(" ", "", " ", "", "€", "", ",", ".").Replace(strings.TrimSpace(s))
	if s == "" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return v
}

func nul(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
