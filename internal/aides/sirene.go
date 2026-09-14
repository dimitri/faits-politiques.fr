package aides

import (
	"archive/zip"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Le répertoire SIRENE, stock des unités légales : la seule source ouverte qui
// donne, pour chaque SIREN, la catégorie d'entreprise (PME, ETI, GE) calculée
// par l'INSEE au niveau du groupe. L'API Sirene demande un compte ; le fichier
// stock, publié chaque mois sur data.gouv.fr, n'en demande pas.
var SourceSirene = archive.Source{
	Slug: "insee-sirene-unites-legales", Label: "INSEE — répertoire SIRENE, stock des unités légales",
	Publisher: "INSEE", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Licence Ouverte v2.0",
	ReuseClass:  "OPEN",
	Attribution: "Source : INSEE, base Sirene des entreprises et de leurs établissements",
	Cadence:     "mensuelle (stock au 1er du mois)",
	Notes: "Seules les personnes morales sont chargées (catégorie juridique ≠ 1000) : les " +
		"entrepreneurs individuels sont des personnes physiques. Catégorie d'entreprise au sens de " +
		"la loi LME, calculée sur l'entreprise profilée (le groupe) et millésimée " +
		"(anneeCategorieEntreprise) : elle n'est pas la tranche d'effectif de l'unité légale. " +
		"Dénomination « [ND] » (diffusion partielle) chargée NULL. Caractère employeur vide dans " +
		"tout le stock de septembre 2026 : lire la tranche d'effectif (NN = non employeuse ou inconnue).",
}

const sireneJeu = "https://www.data.gouv.fr/api/1/datasets/base-sirene-des-entreprises-et-de-leurs-etablissements-siren-siret/"

// urlStockUnitesLegales trouve le fichier du mois dans les métadonnées du jeu :
// son adresse change à chaque publication.
func urlStockUnitesLegales(ctx context.Context, arch *archive.Archive, srcID, runID int64) (string, error) {
	f, err := arch.Fetch(ctx, srcID, runID, sireneJeu, ".json")
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(f.Path)
	if err != nil {
		return "", err
	}
	var jeu struct {
		Resources []struct {
			Title  string `json:"title"`
			Format string `json:"format"`
			URL    string `json:"url"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(b, &jeu); err != nil {
		return "", err
	}
	for _, r := range jeu.Resources {
		if strings.HasPrefix(r.Title, "Sirene : Fichier StockUniteLegale -") && r.Format == "zip" {
			return r.URL, nil
		}
	}
	return "", fmt.Errorf("fichier StockUniteLegale introuvable dans le jeu SIRENE")
}

var colonnesSirene = []string{
	"siren", "denominationUniteLegale", "categorieJuridiqueUniteLegale", "activitePrincipaleUniteLegale",
	"nomenclatureActivitePrincipaleUniteLegale", "etatAdministratifUniteLegale", "categorieEntreprise",
	"anneeCategorieEntreprise", "trancheEffectifsUniteLegale", "anneeEffectifsUniteLegale",
	"caractereEmployeurUniteLegale", "dateCreationUniteLegale",
}

func IngestSirene(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceSirene)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersion)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		err = fmt.Errorf("%s : %w", SourceSirene.Slug, err)
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}
	u, err := urlStockUnitesLegales(ctx, arch, srcID, runID)
	if err != nil {
		return fail(err)
	}
	debut := time.Now()
	f, err := arch.Fetch(ctx, srcID, runID, u, ".zip")
	if err != nil {
		return fail(err)
	}
	fmt.Printf("  SIRENE téléchargé en %s (%s)\n", time.Since(debut).Round(time.Second), u)

	zr, err := zip.OpenReader(f.Path)
	if err != nil {
		return fail(err)
	}
	defer zr.Close()
	if len(zr.File) != 1 {
		return fail(fmt.Errorf("archive SIRENE : %d fichiers, un seul attendu", len(zr.File)))
	}
	rc, err := zr.File[0].Open()
	if err != nil {
		return fail(err)
	}
	defer rc.Close()

	cr := csv.NewReader(rc)
	cr.ReuseRecord = true
	entete, err := cr.Read()
	if err != nil {
		return fail(err)
	}
	pos := map[string]int{}
	for i, c := range entete {
		pos[strings.TrimPrefix(c, "\uFEFF")] = i
	}
	idx := make([]int, len(colonnesSirene))
	for i, c := range colonnesSirene {
		p, ok := pos[c]
		if !ok {
			return fail(fmt.Errorf("colonne %s absente du stock SIRENE", c))
		}
		idx[i] = p
	}

	var lues, chargees, personnesPhysiques int
	entierOuNul := func(s string) any {
		if n, err := strconv.Atoi(s); err == nil {
			return n
		}
		return nil
	}
	texteOuNul := func(s string) any {
		if s == "" || s == "[ND]" {
			return nil
		}
		return s
	}
	suivante := func() ([]any, error) {
		for {
			rec, err := cr.Read()
			if err == io.EOF {
				return nil, nil
			}
			if err != nil {
				return nil, fmt.Errorf("ligne %d : %w", lues+2, err)
			}
			lues++
			v := func(i int) string { return rec[idx[i]] }
			if v(2) == "1000" {
				personnesPhysiques++
				continue
			}
			var creation any
			if t, err := time.Parse("2006-01-02", v(11)); err == nil {
				creation = t
			}
			etat := v(5)
			if etat != "A" && etat != "C" {
				return nil, fmt.Errorf("SIREN %s : état administratif inattendu %q", v(0), etat)
			}
			chargees++
			return []any{v(0), texteOuNul(v(1)), v(2), texteOuNul(v(3)), texteOuNul(v(4)), etat,
				texteOuNul(v(6)), entierOuNul(v(7)), texteOuNul(v(8)), entierOuNul(v(9)), texteOuNul(v(10)),
				creation, srcID, f.DocumentID}, nil
		}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `TRUNCATE ref.unite_legale`); err != nil {
		return fail(err)
	}
	debut = time.Now()
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"ref", "unite_legale"},
		[]string{"siren", "denomination", "categorie_juridique", "activite_principale", "nomenclature_activite",
			"etat_administratif", "categorie_entreprise", "annee_categorie", "tranche_effectifs", "annee_effectifs",
			"caractere_employeur", "date_creation", "source_id", "document_id"},
		pgx.CopyFromFunc(suivante)); err != nil {
		return fail(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	stats := map[string]any{"lues": lues, "personnes_morales": chargees, "personnes_physiques_ecartees": personnesPhysiques}
	arch.EndRun(ctx, runID, "SUCCESS", stats, "")
	fmt.Printf("  %-28s %d unités légales lues, %d personnes morales chargées en %s\n",
		SourceSirene.Slug, lues, chargees, time.Since(debut).Round(time.Second))
	return nil
}
