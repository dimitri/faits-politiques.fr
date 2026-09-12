// Package associations charge le Répertoire national des associations.
//
// Le RNA recense toutes les associations déclarées en préfecture depuis 1901,
// celles qui existent encore comme celles qui ont été dissoutes. Il est publié
// chaque mois par le ministère de l'Intérieur sous Licence Ouverte.
//
// Ce qu'il ne contient PAS : les subventions. Il enregistre des déclarations,
// pas des flux financiers. Aucun jeu national de subventions n'existe — les
// collectivités publient chacune les siennes au schéma SCDL, sans consolidation,
// et la couverture de ces publications reflète surtout quelles collectivités
// publient.
package associations

import (
	"context"
	"encoding/csv"
	"fmt"
	"regexp"
	"strings"
	"time"

	"archive/zip"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ConnectorVersion = "associations-v1"

var Source = archive.Source{
	Slug: "rna", Label: "Répertoire national des associations",
	Publisher: "Ministère de l'Intérieur", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Licence Ouverte",
	ReuseClass:  "OPEN",
	Attribution: "Source : Répertoire national des associations, ministère de l'Intérieur",
	Cadence:     "mensuelle",
	Notes: "Aucune subvention, aucun montant : le RNA enregistre des déclarations. " +
		"Le champ SIRET est vide dans la quasi-totalité des lignes, ce qui interdit " +
		"le passage par SIRENE pour obtenir le code INSEE.",
}

// L'URL porte le millésime mensuel. Elle est fixée plutôt que devinée : c'est
// cette version-là qui est scellée dans l'archive.
const RNAURL = "https://media.interieur.gouv.fr/rna/rna_import_20260901.zip"

// COGMillesime doit suivre celui du référentiel des communes.
const COGMillesime = 2026

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

	f, err := arch.Fetch(ctx, srcID, runID, RNAURL, ".zip")
	if err != nil {
		return fail(err)
	}

	// Le rattachement passe par (département, nom de commune). C'est un
	// rapprochement de noms, borné au département — le seul possible, le RNA ne
	// publiant pas de code INSEE et son champ SIRET étant vide. Les noms
	// ambigus dans un même département sont écartés plutôt qu'arbitrés.
	index, ambigus, err := indexCommunes(ctx, pool)
	if err != nil {
		return fail(err)
	}

	zr, err := zip.OpenReader(f.Path)
	if err != nil {
		return fail(err)
	}
	defer zr.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SET LOCAL work_mem = '256MB'`); err != nil {
		return fail(err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM core.association`); err != nil {
		return fail(err)
	}

	var lot [][]any
	var total, resolues, vus int
	dejaVu := map[string]bool{}

	vider := func() error {
		if len(lot) == 0 {
			return nil
		}
		_, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "association"},
			[]string{"rna_id", "titre", "objet", "objet_social", "nature", "position",
				"date_creation", "date_publication", "code_departement", "code_postal",
				"commune_libelle", "commune_code", "cog_millesime", "source_id"},
			pgx.CopyFromRows(lot))
		lot = lot[:0]
		return err
	}

	for _, zf := range zr.File {
		if !strings.HasSuffix(zf.Name, ".csv") {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return fail(err)
		}
		r := csv.NewReader(rc)
		r.Comma = ';'
		r.FieldsPerRecord = -1
		r.LazyQuotes = true
		head, err := r.Read()
		if err != nil {
			rc.Close()
			continue
		}
		col := map[string]int{}
		for i, h := range head {
			col[strings.TrimSpace(strings.Trim(h, "\ufeff\""))] = i
		}
		get := func(rec []string, k string) string {
			i, ok := col[k]
			if !ok || i >= len(rec) {
				return ""
			}
			return strings.TrimSpace(strings.Trim(rec[i], `"`))
		}
		for {
			rec, err := r.Read()
			if err != nil {
				break
			}
			id := get(rec, "id")
			titre := get(rec, "titre")
			if id == "" || titre == "" {
				continue
			}
			if dejaVu[id] {
				vus++
				continue
			}
			dejaVu[id] = true

			dep := departement(get(rec, "gestion"), id)
			nom := get(rec, "libcom")
			var code, mil any
			if c, ok := index[dep+"|"+normaliser(nomAdministratif(nom))]; ok {
				code, mil = c, COGMillesime
				resolues++
			}
			lot = append(lot, []any{
				id, tronquer(titre, 400), nul(tronquer(get(rec, "objet"), 1000)),
				nul(get(rec, "objet_social1")), nul(get(rec, "nature")),
				nul(get(rec, "position")),
				dateNul(get(rec, "date_creat")), dateNul(get(rec, "date_publi")),
				nul(dep), nul(get(rec, "adrs_codepostal")), nul(tronquer(nom, 120)),
				code, mil, srcID,
			})
			total++
			if len(lot) >= 50000 {
				if err := vider(); err != nil {
					rc.Close()
					return fail(fmt.Errorf("copie : %w", err))
				}
			}
		}
		rc.Close()
	}
	if err := vider(); err != nil {
		return fail(fmt.Errorf("copie : %w", err))
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{
		"associations": total, "resolues": resolues, "doublons": vus}, "")
	fmt.Printf("  RNA : %d associations, %d rattachées à une commune (%.1f %%)\n",
		total, resolues, 100*float64(resolues)/float64(total))
	fmt.Printf("  %d noms de commune ambigus dans leur département : non rattachés\n", ambigus)
	return nil
}

// indexCommunes construit (département, nom normalisé) -> code INSEE, en
// écartant les noms qui apparaissent deux fois dans le même département.
func indexCommunes(ctx context.Context, pool *pgxpool.Pool) (map[string]string, int, error) {
	rows, err := pool.Query(ctx,
		`SELECT code_insee, code_departement, coalesce(nom_clair, nom) FROM ref.commune
		  WHERE cog_millesime = $1`,
		COGMillesime)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	index := map[string]string{}
	doublons := map[string]bool{}
	for rows.Next() {
		var code, dep, nom string
		if err := rows.Scan(&code, &dep, &nom); err != nil {
			return nil, 0, err
		}
		k := dep + "|" + normaliser(nom)
		if _, existe := index[k]; existe {
			doublons[k] = true
			continue
		}
		index[k] = code
	}
	for k := range doublons {
		delete(index, k)
	}
	return index, len(doublons), rows.Err()
}

// departement lit le code département dans le champ de gestion. Celui-ci vaut
// « 091P » pour l'Ariège, « 381P » pour l'Isère : le département occupe les DEUX
// premiers caractères, le reste identifiant la préfecture ou sous-préfecture.
//
// Une première version prenait trois caractères et retirait le zéro de tête,
// ce qui transformait l'Ariège (091) en Essonne (91). Le taux de rattachement
// tombé à 0,0 % l'a révélé — sans quoi des associations auraient été attribuées
// au mauvais département sans que rien ne le signale.
func departement(gestion, id string) string {
	s := gestion
	if s == "" {
		s = id
	}
	if len(s) < 2 {
		return ""
	}
	// L'outre-mer est codé sur trois chiffres.
	if strings.HasPrefix(s, "97") || strings.HasPrefix(s, "98") {
		if len(s) >= 3 {
			return s[:3]
		}
	}
	return s[:2]
}

// nomAdministratif remet en tête l'article que les fichiers administratifs
// rejettent en fin de chaîne : « BUISSON DE CADOUIN L » désigne Le
// Buisson-de-Cadouin. La comparaison se fait ensuite sur le nom sans article,
// qui est la colonne NCC du Code officiel géographique.
func nomAdministratif(s string) string {
	champs := strings.Fields(strings.ToUpper(s))
	if len(champs) > 1 {
		switch champs[len(champs)-1] {
		case "L", "LA", "LE", "LES", "L'":
			champs = champs[:len(champs)-1]
		}
	}
	return strings.Join(champs, " ")
}

var nonAlpha = regexp.MustCompile(`[^a-z0-9]+`)

func normaliser(s string) string {
	s = strings.ToLower(s)
	for from, to := range map[string]string{
		"à": "a", "â": "a", "ä": "a", "ç": "c", "é": "e", "è": "e", "ê": "e",
		"ë": "e", "î": "i", "ï": "i", "ô": "o", "ö": "o", "ù": "u", "û": "u",
		"ü": "u", "ÿ": "y", "œ": "oe", "æ": "ae",
	} {
		s = strings.ReplaceAll(s, from, to)
	}
	return strings.Trim(nonAlpha.ReplaceAllString(s, "-"), "-")
}

func dateNul(s string) any {
	s = strings.TrimSpace(s)
	if len(s) < 10 || strings.HasPrefix(s, "0001") {
		return nil
	}
	t, err := time.Parse("2006-01-02", s[:10])
	if err != nil {
		return nil
	}
	return t
}

func tronquer(s string, n int) string {
	s = strings.Join(strings.Fields(strings.ToValidUTF8(s, "")), " ")
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func nul(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
