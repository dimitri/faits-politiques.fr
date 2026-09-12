// Package communes charge la dimension locale : le référentiel géographique,
// les maires, les résultats des élections municipales et les comptes communaux.
//
// L'ordre n'est pas négociable. ref.commune est référencé par tout le reste :
// sans le Code officiel géographique et son millésime, un code INSEE ne désigne
// rien de stable — les communes fusionnent, se scindent et changent de code.
package communes

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ConnectorVersion = "communes-v1"

// Millésime du COG chargé. Toute donnée communale y est rattachée : changer ce
// millésime impose de recharger ce qui en dépend, pas de le mélanger.
const COGMillesime = 2026

var SourceCOG = archive.Source{
	Slug: "insee-cog", Label: "Code officiel géographique",
	Publisher: "INSEE", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Licence Ouverte v2.0",
	ReuseClass:  "OPEN",
	Attribution: "Source : INSEE, Code officiel géographique",
	Cadence:     "annuelle, au 1er janvier",
	Notes: "Le millésime est structurant : un code commune ne vaut que pour un " +
		"millésime donné. Le fichier des mouvements permet de suivre fusions et scissions.",
}

const (
	COGCommunesURL = "https://www.insee.fr/fr/statistiques/fichier/8740222/v_commune_2026.csv"
	COGMouvURL     = "https://www.insee.fr/fr/statistiques/fichier/8740222/v_mvt_commune_2026.csv"
)

// Codes MOD du fichier des mouvements, traduits vers les quatre types que le
// schéma admet. Les codes non listés sont ignorés : ils concernent des objets
// que nous ne suivons pas (communes déléguées, associées, arrondissements).
var modVersType = map[string]string{
	"10": "RENOMMAGE",
	"20": "CHANGEMENT_CODE", // création
	"21": "SCISSION",        // rétablissement d'une commune
	"30": "FUSION",          // suppression au profit d'une autre
	"31": "FUSION",          // fusion simple
	"32": "FUSION",          // création d'une commune nouvelle
	"33": "FUSION",          // fusion-association
	"41": "CHANGEMENT_CODE",
	"50": "CHANGEMENT_CODE",
	"70": "CHANGEMENT_CODE",
}

func IngestCOG(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceCOG)
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

	fCom, err := arch.Fetch(ctx, srcID, runID, COGCommunesURL, ".csv")
	if err != nil {
		return fail(err)
	}
	fMvt, err := arch.Fetch(ctx, srcID, runID, COGMouvURL, ".csv")
	if err != nil {
		return fail(err)
	}

	recs, err := lireCSV(fCom.Path, ',')
	if err != nil {
		return fail(err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	// Reconstruction plutôt que complétion : une commune disparue du millésime
	// doit disparaître de la table, sinon le référentiel accumule des fantômes.
	if _, err := tx.Exec(ctx,
		`DELETE FROM ref.commune_change WHERE cog_millesime = $1`, COGMillesime); err != nil {
		return fail(err)
	}

	var rows [][]any
	seen := map[string]bool{}
	for _, r := range recs {
		// TYPECOM : COM = commune de plein exercice. Les communes déléguées
		// (COMD), associées (COMA) et arrondissements municipaux (ARM) ne sont
		// pas des communes et n'ont ni compte ni conseil propre.
		if r["TYPECOM"] != "COM" {
			continue
		}
		code := r["COM"]
		if code == "" || seen[code] {
			continue
		}
		seen[code] = true
		rows = append(rows, []any{code, COGMillesime, r["LIBELLE"], r["DEP"], r["REG"], r["NCC"]})
	}

	// INSERT ... SELECT depuis une table temporaire alimentée par COPY : c'est
	// le chemin le plus court pour 35 000 lignes, et il évite 35 000 allers-
	// retours.
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE cog_in (code text, mil int, nom text, dep text, reg text, ncc text)
		ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"cog_in"},
		[]string{"code", "mil", "nom", "dep", "reg", "ncc"}, pgx.CopyFromRows(rows)); err != nil {
		return fail(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO ref.commune (code_insee, cog_millesime, nom, code_departement, code_region, nom_clair)
		SELECT code, mil, nom, dep, reg, ncc FROM cog_in
		ON CONFLICT (code_insee, cog_millesime) DO UPDATE
		  SET nom = EXCLUDED.nom,
		      code_departement = EXCLUDED.code_departement,
		      code_region = EXCLUDED.code_region,
		      nom_clair = EXCLUDED.nom_clair`); err != nil {
		return fail(err)
	}

	// Mouvements.
	mvts, err := lireCSV(fMvt.Path, ',')
	if err != nil {
		return fail(err)
	}
	var nMvt int
	for _, m := range mvts {
		t, ok := modVersType[m["MOD"]]
		if !ok || m["COM_AV"] == "" || m["COM_AP"] == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO ref.commune_change
			  (effective_date, change_type, code_avant, code_apres, cog_millesime)
			VALUES ($1::date, $2, $3, $4, $5)`,
			m["DATE_EFF"], t, m["COM_AV"], m["COM_AP"], COGMillesime); err != nil {
			return fail(fmt.Errorf("mouvement %s %s->%s : %w", m["MOD"], m["COM_AV"], m["COM_AP"], err))
		}
		nMvt++
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"communes": len(rows), "mouvements": nMvt}, "")
	fmt.Printf("  COG %d : %d communes, %d mouvements\n", COGMillesime, len(rows), nMvt)
	return nil
}

// lireCSV renvoie les lignes sous forme de maps colonne -> valeur. Les fichiers
// de l'administration mêlent les séparateurs et les encodages ; le séparateur
// est donc explicite à chaque appel.
func lireCSV(path string, sep rune) ([]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comma = sep
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	recs, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%s : %w", path, err)
	}
	if len(recs) < 2 {
		return nil, fmt.Errorf("%s : fichier vide", path)
	}
	head := make([]string, len(recs[0]))
	for i, h := range recs[0] {
		head[i] = strings.TrimSpace(strings.TrimPrefix(h, "\ufeff"))
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
	return out, nil
}

func readFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}
