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

		for _, r := range recs {
			nom := strings.TrimSpace(r["nom"])
			if nom == "" {
				continue
			}
			var id int64
			if err := tx.QueryRow(ctx, `
				INSERT INTO core.compte_campagne
				  (type_election, annee, candidat_ref, candidat_nom, circonscription,
				   departement, code_departement, nuance, monnaie,
				   depenses_declarees, depenses_retenues, recettes_declarees, recettes_retenues, source_id)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
				ON CONFLICT (type_election, annee, candidat_nom, circonscription) DO NOTHING
				RETURNING id`,
				s.typeElection, s.annee, nul(r["candidat"]), nom,
				nul(r["circonscription"]), nul(r["département"]), nul(r["code département"]),
				nul(r["nuance"]), nul(r["monnaie"]),
				montant(r["dépenses totales déclarées"]), montant(r["depenses totales retenues"]),
				montant(r["recettes totales déclarées"]), montant(r["recettes totales retenues"]),
				srcID).Scan(&id); err != nil {
				continue // doublon de nom dans la même circonscription : ignoré
			}
			nComptes++

			// Les postes : toute colonne suffixée « (déclaré) » ou « (retenu) ».
			// Le libellé est conservé tel quel, sans regroupement.
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
				if _, err := tx.Exec(ctx, `
					INSERT INTO core.compte_campagne_poste (compte_id, poste, etat, montant)
					VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, id, poste, etat, m); err != nil {
					return fail(fmt.Errorf("poste %q : %w", poste, err))
				}
				nPostes++
			}
		}
		fmt.Printf("    %s %d : %d comptes\n", s.typeElection, s.annee, nComptes)
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"comptes": nComptes, "postes": nPostes}, "")
	fmt.Printf("  comptes de campagne : %d comptes, %d postes détaillés\n", nComptes, nPostes)
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
