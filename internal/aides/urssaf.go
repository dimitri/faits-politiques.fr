// Package aides charge ce qui permet de dire QUI reçoit les aides publiques
// aux entreprises : la répartition des exonérations de cotisations par taille
// d'entreprise, et la catégorie d'entreprise de l'INSEE pour chaque personne
// morale, en vue de croiser les aides publiées bénéficiaire par bénéficiaire.
// Voir docs/dette-donnees.md § 14.
package aides

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ConnectorVersion = "aides-v1"

// Les deux jeux de l'URSSAF qui partagent la même découpe par tranche
// d'effectif : les exonérations, et l'emploi avec la masse salariale. Le
// second est le dénominateur du premier — sans lui, « 15 % des exonérations »
// ne dit pas si c'est beaucoup.
var SourceUrssafTaille = archive.Source{
	Slug: "urssaf-taille-entreprise", Label: "URSSAF — exonérations, emploi et masse salariale par taille d'entreprise",
	Publisher: "URSSAF", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Open Database License (ODbL)",
	ReuseClass:  "ATTRIBUTION",
	Attribution: "Source : Urssaf, open.urssaf.fr (exonérations et effectifs du secteur privé par tranche de taille d'entreprise)",
	Cadence:     "annuelle (fin d'année + ~200 jours)",
	Notes: "ODbL : PARTAGE À L'IDENTIQUE des bases dérivées. Tranche = taille de l'entreprise au " +
		"sens de l'URSSAF (base Sequoia, effectifs moyens de l'année), société et non groupe : la " +
		"part des grands groupes est minorée. Secteur privé du régime général, hors agriculture et " +
		"Mayotte. CICE inclus de 2013 à 2018 (catégorie 14). Révision du 24 juillet 2026 : " +
		"réintégration des alternants dans les mesures 151 et 161.",
}

const openUrssaf = "https://open.urssaf.fr/api/explore/v2.1/catalog/datasets/"

// Les libellés de tranche portent leur lettre de tri : « a) 0 à 9 ».
var tranches = map[string]string{
	"a) 0 à 9": "a", "b) 10 à 19": "b", "c) 20 à 49": "c", "d) 50 à 99": "d",
	"e) 100 à 249": "e", "f) 250 à 499": "f", "g) 500 à 1999": "g", "h) 2000 et plus": "h",
}

func tranche(libelle string) (string, error) {
	t, ok := tranches[strings.Join(strings.Fields(strings.ReplaceAll(libelle, "\u00a0", " ")), " ")]
	if !ok {
		return "", fmt.Errorf("tranche d'effectif inattendue : %q", libelle)
	}
	return t, nil
}

// annee lit « 2024 » comme « 2024-01-01T00:00:00+00:00 » : l'API renvoie
// l'un ou l'autre selon qu'on interroge les enregistrements ou l'export.
func annee(v any) (int, error) {
	s := fmt.Sprint(v)
	if len(s) < 4 {
		return 0, fmt.Errorf("année illisible : %q", s)
	}
	return strconv.Atoi(s[:4])
}

func nombre(v any) any {
	switch x := v.(type) {
	case float64:
		return x
	case nil:
		return nil
	}
	return nil
}

func IngestUrssafTaille(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceUrssafTaille)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersion)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		err = fmt.Errorf("%s : %w", SourceUrssafTaille.Slug, err)
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}
	lire := func(jeu string) ([]map[string]any, int64, error) {
		f, err := arch.Fetch(ctx, srcID, runID, openUrssaf+jeu+"/exports/json", ".json")
		if err != nil {
			return nil, 0, err
		}
		b, err := os.ReadFile(f.Path)
		if err != nil {
			return nil, 0, err
		}
		var recs []map[string]any
		if err := json.Unmarshal(b, &recs); err != nil {
			return nil, 0, fmt.Errorf("%s : %w", jeu, err)
		}
		if len(recs) == 0 {
			return nil, 0, fmt.Errorf("%s : aucun enregistrement", jeu)
		}
		return recs, f.DocumentID, nil
	}

	exos, docExo, err := lire("exos-secteur-prive-tranche-ent")
	if err != nil {
		return fail(err)
	}
	emplois, docEmp, err := lire("nombre-etab-effectifs-salaries-et-masse-salariale-secteur-prive-tranche-ent")
	if err != nil {
		return fail(err)
	}

	var rowsExo, rowsEmp [][]any
	vus := map[string]bool{}
	for _, r := range exos {
		a, err := annee(r["annee"])
		if err != nil {
			return fail(err)
		}
		t, err := tranche(fmt.Sprint(r["tranche_d_effectif"]))
		if err != nil {
			return fail(err)
		}
		cat := fmt.Sprint(r["code_categorie_de_mesures"])
		cle := fmt.Sprintf("%d|%s|%s", a, t, cat)
		if vus[cle] {
			return fail(fmt.Errorf("exonérations : %s en double", cle))
		}
		vus[cle] = true
		rowsExo = append(rowsExo, []any{a, t, fmt.Sprint(r["code_grande_categorie_de_mesures"]),
			fmt.Sprint(r["grande_categorie_de_mesures"]), cat, fmt.Sprint(r["categorie_de_mesures"]),
			nombre(r["montant_des_exonerations"]), srcID, docExo})
	}
	for _, r := range emplois {
		a, err := annee(r["annee"])
		if err != nil {
			return fail(err)
		}
		t, err := tranche(fmt.Sprint(r["tranche_d_effectif"]))
		if err != nil {
			return fail(err)
		}
		entier := func(k string) any {
			if x, ok := r[k].(float64); ok {
				return int64(x)
			}
			return nil
		}
		rowsEmp = append(rowsEmp, []any{a, t, entier("nombre_d_entreprises"), entier("nombre_d_etablissements"),
			nombre(r["effectifs_salaries_moyens"]), nombre(r["masse_salariale"]), srcID, docEmp})
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	for _, t := range []string{"core.exoneration_tranche", "core.emploi_prive_tranche"} {
		if _, err := tx.Exec(ctx, "DELETE FROM "+t+" WHERE source_id = $1", srcID); err != nil {
			return fail(err)
		}
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "exoneration_tranche"},
		[]string{"annee", "tranche", "code_grande_categorie", "grande_categorie", "code_categorie", "categorie",
			"montant_eur", "source_id", "document_id"}, pgx.CopyFromRows(rowsExo)); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "emploi_prive_tranche"},
		[]string{"annee", "tranche", "nombre_entreprises", "nombre_etablissements", "effectifs_moyens",
			"masse_salariale_eur", "source_id", "document_id"}, pgx.CopyFromRows(rowsEmp)); err != nil {
		return fail(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"exonerations": len(rowsExo), "emploi": len(rowsEmp)}, "")
	fmt.Printf("  %-28s %5d exonérations  %4d lignes d'emploi\n", SourceUrssafTaille.Slug, len(rowsExo), len(rowsEmp))
	return nil
}

// Ingest charge la taille des entreprises : URSSAF puis SIRENE. SIRENE pèse
// près d'un gigaoctet : il se charge aussi seul (-only=sirene).
func Ingest(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	if err := IngestUrssafTaille(ctx, pool, arch); err != nil {
		return err
	}
	return IngestSirene(ctx, pool, arch)
}
