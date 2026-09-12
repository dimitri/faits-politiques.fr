package senat

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Le Sénat publie son propre répertoire de sénateurs, distinct du dump Dosleg
// des travaux législatifs. Il porte ce que Dosleg n'a pas : la DATE DE
// NAISSANCE, la date de décès, le groupe politique, la circonscription et la
// profession — le tout indexé par le matricule, c'est-à-dire par l'identifiant
// que nous stockons déjà.
//
// Cela rend inutile le rapprochement par nom qui était envisagé : joindre sur
// un identifiant publié vaut toujours mieux que d'apparier des homonymes, même
// quand l'ensemble est petit et qu'on a vérifié qu'il n'y en a pas.
var SourceSenateurs = archive.Source{
	Slug: "senat-senateurs", Label: "Sénat — répertoire des sénateurs",
	Publisher: "Sénat", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Licence Ouverte",
	ReuseClass:  "OPEN",
	Attribution: "Source : Sénat, base Sénateurs (data.senat.fr)",
	Cadence:     "continue",
	Notes: "Fichier en ISO-8859-1, précédé de dix-huit lignes de commentaire " +
		"reproduisant la requête SQL qui l'a produit.",
}

const (
	senateursURL   = "https://data.senat.fr/data/senateurs/ODSEN_GENERAL.csv"
	commissionsURL = "https://data.senat.fr/data/senateurs/ODSEN_COMS.csv"
)

// IngestSenateurs complète les personnes du Sénat déjà créées à partir des
// votes. Il ne crée personne : une personne qui n'a jamais voté n'a pas à
// entrer en base par ce chemin.
func IngestSenateurs(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceSenateurs)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, Version)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, senateursURL, ".csv")
	if err != nil {
		return fail(err)
	}
	recs, err := lireCSVSenat(f.Path)
	if err != nil {
		return fail(err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE sen_in (
		  matricule text, naissance date, deces date, etat text,
		  groupe text, circonscription text, profession text
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}

	var lignes [][]any
	for _, r := range recs {
		mat := strings.TrimSpace(r["Matricule"])
		if mat == "" {
			continue
		}
		lignes = append(lignes, []any{
			mat, dateSenat(r["Date naissance"]), dateSenat(r["Date de décès"]),
			nulS(r["État"]), nulS(r["Groupe politique"]),
			nulS(r["Circonscription"]), nulS(r["Description de la profession"]),
		})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"sen_in"},
		[]string{"matricule", "naissance", "deces", "etat", "groupe",
			"circonscription", "profession"}, pgx.CopyFromRows(lignes)); err != nil {
		return fail(fmt.Errorf("copie des sénateurs : %w", err))
	}

	// La jointure se fait sur le matricule : aucun rapprochement de noms, donc
	// aucun homonyme possible.
	res, err := tx.Exec(ctx, `
		UPDATE core.person p
		   SET birth_date = coalesce(p.birth_date, s.naissance),
		       death_date = coalesce(p.death_date, s.deces),
		       profession = coalesce(p.profession, s.profession)
		  FROM sen_in s
		  JOIN core.person_identifier i
		    ON i.scheme = 'SENAT_MATRICULE' AND i.value = s.matricule
		 WHERE p.id = i.person_id
		   AND (s.naissance IS NOT NULL OR s.deces IS NOT NULL OR s.profession IS NOT NULL)`)
	if err != nil {
		return fail(err)
	}

	var restants int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM core.person p
		  JOIN core.person_identifier i ON i.person_id = p.id AND i.scheme = 'SENAT_MATRICULE'
		 WHERE p.birth_date IS NULL`).Scan(&restants); err != nil {
		return fail(err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"senateurs": len(lignes), "complets": res.RowsAffected()}, "")
	fmt.Printf("  Sénateurs : %d au répertoire, %d personnes complétées, %d encore sans date de naissance\n",
		len(lignes), res.RowsAffected(), restants)
	return nil
}

// lireCSVSenat lit un fichier du Sénat : ISO-8859-1, séparé par des virgules,
// précédé de lignes de commentaire commençant par « % » qui reproduisent la
// requête SQL ayant produit l'export.
func lireCSVSenat(path string) ([]map[string]string, error) {
	brut, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	texte := latin1(brut)

	lignes := strings.Split(texte, "\n")
	var utiles []string
	for _, l := range lignes {
		if strings.HasPrefix(l, "%") {
			continue
		}
		utiles = append(utiles, strings.TrimRight(l, "\r"))
	}

	r := csv.NewReader(strings.NewReader(strings.Join(utiles, "\n")))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	recs, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%s : %w", path, err)
	}
	if len(recs) < 2 {
		return nil, fmt.Errorf("%s : aucune donnée", path)
	}
	head := recs[0]
	out := make([]map[string]string, 0, len(recs)-1)
	for _, rec := range recs[1:] {
		m := make(map[string]string, len(head))
		for i, h := range head {
			if i < len(rec) {
				m[strings.TrimSpace(h)] = strings.TrimSpace(rec[i])
			}
		}
		out = append(out, m)
	}
	return out, nil
}

// latin1 convertit de l'ISO-8859-1 vers UTF-8. Écrit à la main plutôt que par
// golang.org/x/text : la conversion tient en trois lignes, et le projet n'a
// qu'une dépendance.
func latin1(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

// dateSenat lit « 1930-06-19 00:00:00.0 ».
func dateSenat(s string) any {
	s = strings.TrimSpace(s)
	if len(s) < 10 {
		return nil
	}
	t, err := time.Parse("2006-01-02", s[:10])
	if err != nil {
		return nil
	}
	return t
}

func nulS(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.TrimSpace(s)
}
