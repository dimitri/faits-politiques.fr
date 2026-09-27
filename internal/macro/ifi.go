package macro

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"
)

// SourceIFICOM : la DGFiP publie chaque année, pour les seules communes de
// plus de 20 000 habitants comptant plus de 50 redevables à l'IFI, le nombre
// de redevables et les moyennes de patrimoine et d'impôt — voir
// docs/sci-holding-donnees.md. Un fichier par millésime, format qui a changé
// d'unité en 2021 (patrimoine et impôt moyens en euros bruts, pas en
// millions/milliers comme en 2020 et avant) : seuls 2021-2025 sont chargés.
var SourceIFICOM = archive.Source{
	Slug: "dgfip-ificom", Label: "DGFiP — Impôt sur la fortune immobilière (IFI), répartition par commune",
	Publisher: "Direction générale des finances publiques (DGFiP)", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : DGFiP (IFICOM)",
	Cadence:     "annuelle",
	Notes: "Publié pour les seules communes de plus de 20 000 habitants comptant plus de 50 redevables à " +
		"l'IFI — un seuil de publication de la DGFiP, qui exclut la grande majorité des communes de France, " +
		"pas un filtre de ce dépôt. Paris est publié par arrondissement certaines années (jusqu'à 20 lignes " +
		"« 751xx »), agrégés à l'affichage cartographique faute d'exister dans le référentiel communal. " +
		"Millésime 2020 et antérieur : unités différentes (patrimoine en millions d'euros, impôt en milliers " +
		"d'euros), non chargées ici plutôt que converties sans certitude sur l'arrondi d'origine.",
}

var urlsIFICOM = map[int]string{
	2021: "https://static.data.gouv.fr/resources/impot-de-solidarite-sur-la-fortune-impot-sur-la-fortune-immobiliere/20220621-104658/ificom-2021.xlsx",
	2022: "https://static.data.gouv.fr/resources/impot-de-solidarite-sur-la-fortune-impot-sur-la-fortune-immobiliere-par-collectivite-territoriale/20230425-174501/ificom-2022.xlsx",
	2023: "https://static.data.gouv.fr/resources/impot-de-solidarite-sur-la-fortune-impot-sur-la-fortune-immobiliere-par-collectivite-territoriale/20240503-124218/ificom2023.xlsx",
	2024: "https://static.data.gouv.fr/resources/impot-de-solidarite-sur-la-fortune-impot-sur-la-fortune-immobiliere-par-collectivite-territoriale/20250415-081200/ificom2024.xlsx",
	2025: "https://static.data.gouv.fr/resources/impot-de-solidarite-sur-la-fortune-impot-sur-la-fortune-immobiliere-par-collectivite-territoriale/20260414-080108/ificom2025.xlsx",
}

func nombreIFICOM(s string) (float64, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	if s == "" {
		return 0, fmt.Errorf("valeur vide")
	}
	return strconv.ParseFloat(s, 64)
}

func ligneIFICOM(annee int, srcID int64, r []string, idx map[string]int) ([]any, error) {
	get := func(col string) string {
		i, ok := idx[col]
		if !ok || i >= len(r) {
			return ""
		}
		return strings.TrimSpace(r[i])
	}
	codeInsee := strings.ReplaceAll(get("Code commune (INSEE)"), " ", "")
	if codeInsee == "" {
		return nil, fmt.Errorf("code commune vide")
	}
	nom := get("Commune")
	if nom == "" {
		return nil, fmt.Errorf("nom de commune vide pour %q", codeInsee)
	}
	nb, err := nombreIFICOM(get("nombre de redevables"))
	if err != nil {
		return nil, fmt.Errorf("%s (%s) : nombre de redevables illisible : %w", nom, codeInsee, err)
	}
	patrimoine, err := nombreIFICOM(get("patrimoine moyen en €"))
	if err != nil {
		return nil, fmt.Errorf("%s (%s) : patrimoine moyen illisible : %w", nom, codeInsee, err)
	}
	impot, err := nombreIFICOM(get("impôt moyen en €"))
	if err != nil {
		return nil, fmt.Errorf("%s (%s) : impôt moyen illisible : %w", nom, codeInsee, err)
	}
	var dept, region *string
	if d := get("Départements"); d != "" {
		dept = &d
	}
	if rg := get("Région"); rg != "" {
		region = &rg
	}
	return []any{annee, codeInsee, nom, dept, region, int(nb), patrimoine, impot, srcID}, nil
}

// IngestIFICOM charge les millésimes 2021 à 2025 de la répartition
// communale de l'IFI.
func IngestIFICOM(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceIFICOM)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "ificom-v1")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	requis := []string{"Code commune (INSEE)", "Commune", "nombre de redevables", "patrimoine moyen en €", "impôt moyen en €"}

	var toutesLignes [][]any
	compteParAnnee := map[int]int{}
	for annee := 2021; annee <= 2025; annee++ {
		url := urlsIFICOM[annee]
		f, err := arch.Fetch(ctx, srcID, runID, url, ".xlsx")
		if err != nil {
			return fail(fmt.Errorf("%d : %w", annee, err))
		}
		wb, err := excelize.OpenFile(f.Path)
		if err != nil {
			return fail(fmt.Errorf("%d : classeur illisible : %w", annee, err))
		}
		sheets := wb.GetSheetList()
		if len(sheets) == 0 {
			wb.Close()
			return fail(fmt.Errorf("%d : classeur sans feuille", annee))
		}
		rows, err := wb.GetRows(sheets[0])
		wb.Close()
		if err != nil {
			return fail(fmt.Errorf("%d : %w", annee, err))
		}
		if len(rows) < 3 {
			return fail(fmt.Errorf("%d : moins de 3 lignes (titre + en-tête + données attendues)", annee))
		}
		header := rows[1]
		idx := map[string]int{}
		for i, h := range header {
			idx[strings.TrimSpace(h)] = i
		}
		for _, col := range requis {
			if _, ok := idx[col]; !ok {
				return fail(fmt.Errorf("%d : colonne %q absente — le format a peut-être changé", annee, col))
			}
		}
		n := 0
		for i, r := range rows[2:] {
			ligne, err := ligneIFICOM(annee, srcID, r, idx)
			if err != nil {
				return fail(fmt.Errorf("%d, ligne %d : %w", annee, i+3, err))
			}
			toutesLignes = append(toutesLignes, ligne)
			n++
		}
		if n == 0 {
			return fail(fmt.Errorf("%d : aucune ligne lue", annee))
		}
		compteParAnnee[annee] = n
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_ifi_commune (
			annee int, code_insee text, nom_commune text, code_departement text, region text,
			nombre_redevables int, patrimoine_moyen_eur numeric, impot_moyen_eur numeric, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_ifi_commune"},
		[]string{"annee", "code_insee", "nom_commune", "code_departement", "region", "nombre_redevables", "patrimoine_moyen_eur", "impot_moyen_eur", "source_id"},
		pgx.CopyFromRows(toutesLignes)); err != nil {
		return fail(fmt.Errorf("core.ifi_commune : %w", err))
	}
	// MERGE plutôt que DELETE+COPY : ce connecteur est l'unique propriétaire de
	// la table (seuls les millésimes 2021-2025 y sont jamais chargés), et
	// l'ancien DELETE payait le prix des triggers RI pour l'intégralité du
	// périmètre à chaque republication annuelle, changement ou non.
	ct, err := tx.Exec(ctx, `
		MERGE INTO core.ifi_commune AS tgt
		USING tmp_ifi_commune AS src
		ON tgt.annee = src.annee AND tgt.code_insee = src.code_insee
		WHEN MATCHED AND (tgt.nom_commune, tgt.code_departement, tgt.region, tgt.nombre_redevables,
		                   tgt.patrimoine_moyen_eur, tgt.impot_moyen_eur, tgt.source_id)
		                  IS DISTINCT FROM
		                  (src.nom_commune, src.code_departement, src.region, src.nombre_redevables,
		                   src.patrimoine_moyen_eur, src.impot_moyen_eur, src.source_id) THEN
		    UPDATE SET nom_commune = src.nom_commune, code_departement = src.code_departement,
		               region = src.region, nombre_redevables = src.nombre_redevables,
		               patrimoine_moyen_eur = src.patrimoine_moyen_eur, impot_moyen_eur = src.impot_moyen_eur,
		               source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (annee, code_insee, nom_commune, code_departement, region, nombre_redevables,
		            patrimoine_moyen_eur, impot_moyen_eur, source_id)
		    VALUES (src.annee, src.code_insee, src.nom_commune, src.code_departement, src.region,
		            src.nombre_redevables, src.patrimoine_moyen_eur, src.impot_moyen_eur, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion : %w", err))
	}
	touchees := ct.RowsAffected()
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"communes_par_annee": compteParAnnee, "touchees": touchees}, "")
	fmt.Printf("  IFICOM : %d communes×années chargées (2021-2025), %d touchées par la fusion\n",
		len(toutesLignes), touchees)
	return nil
}
