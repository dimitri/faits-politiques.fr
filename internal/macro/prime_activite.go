package macro

import (
	"context"
	"fmt"
	"strconv"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// La prime d'activité, qui a remplacé le RSA activité au 1er janvier 2016 —
// distincte du RSA (socle) déjà chargé dans core.minima_sociaux_effectif.
// Voir docs/chomage-donnees.md.
var SourcePrimeActivite = archive.Source{
	Slug: "drees-prime-activite-nationale", Label: "Drees — RSA et prime d'activité, données nationales",
	Publisher: "Direction de la recherche, des études, de l'évaluation et des statistiques",
	Tier:      "PRIMARY_OFFICIAL",
	Licence:   "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Drees, à partir de la Cnaf et de la MSA",
	Cadence:     "annuelle",
	Notes: "Même jeu de données ouvert que core.minima_sociaux_effectif (n° 336), fichier " +
		"distinct : la prime d'activité n'existe pas avant 2016, contrairement au RSA. " +
		"Champ France métropolitaine — la feuille source publie aussi un bloc France " +
		"entière, non retenu ici, publié avec un an de retard (arrêté à 2022 au moment " +
		"de l'écriture contre 2024 pour la métropole).",
}

const primeActiviteURL = "https://data.drees.solidarites-sante.gouv.fr/api/explore/v2.1/catalog/datasets/" +
	"336_minima-sociaux-rsa-et-prime-d-activite/attachments/rsa_et_prime_d_activite_donnees_nationales_xlsx"

func IngestPrimeActivite(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourcePrimeActivite)
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

	f, err := arch.Fetch(ctx, srcID, runID, primeActiviteURL, ".xlsx")
	if err != nil {
		return fail(err)
	}
	x, err := openXLSX(f.Path)
	if err != nil {
		return fail(err)
	}
	defer x.Close()
	lignes, err := x.rows("Tableau 1")
	if err != nil {
		return fail(err)
	}
	annees, err := colonneAnnees(lignes)
	if err != nil {
		return fail(err)
	}

	// La ligne « Prime d'activité » (total, hors RSA) apparaît DEUX FOIS dans
	// cette feuille : un premier bloc « France métropolitaine » (complet
	// jusqu'à 2024), puis un second bloc « France » entière, publié avec un
	// an de retard (s'arrête à 2022 au moment de l'écriture). On ne garde que
	// le PREMIER bloc rencontré — France métropolitaine, le champ déjà
	// retenu pour core.minima_sociaux_effectif — pour ne jamais mélanger deux
	// champs géographiques dans une même série.
	valeurs := map[int]int{}
	trouve := false
	for _, l := range lignes {
		if l["B"] != "Prime d'activité" {
			continue
		}
		for _, ca := range annees {
			v, ok := l[ca.col]
			if !ok || v == "-" {
				continue
			}
			eff, err := strconv.Atoi(v)
			if err != nil {
				continue
			}
			valeurs[ca.annee] = eff
		}
		trouve = true
		break
	}
	if !trouve {
		return fail(fmt.Errorf("ligne « Prime d'activité » introuvable"))
	}
	if len(valeurs) == 0 {
		return fail(fmt.Errorf("ligne « Prime d'activité » introuvable ou vide"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_prime_activite_effectif (
			annee int, effectif int, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	var rows [][]any
	for annee, eff := range valeurs {
		rows = append(rows, []any{annee, eff, srcID})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_prime_activite_effectif"},
		[]string{"annee", "effectif", "source_id"}, pgx.CopyFromRows(rows)); err != nil {
		return fail(fmt.Errorf("prime_activite_effectif : %w", err))
	}
	// MERGE plutôt que DELETE+COPY : ce connecteur est l'unique propriétaire de
	// la table, et l'ancien DELETE payait le prix des triggers RI pour
	// l'intégralité des millésimes à chaque republication, changement ou non.
	ct, err := tx.Exec(ctx, `
		MERGE INTO core.prime_activite_effectif AS tgt
		USING tmp_prime_activite_effectif AS src
		ON tgt.annee = src.annee
		WHEN MATCHED AND (tgt.effectif, tgt.source_id) IS DISTINCT FROM (src.effectif, src.source_id) THEN
		    UPDATE SET effectif = src.effectif, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (annee, effectif, source_id) VALUES (src.annee, src.effectif, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion prime_activite_effectif : %w", err))
	}
	n := ct.RowsAffected()
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"annees_touchees": n}, "")
	fmt.Printf("  prime d'activité : %d millésimes touchés par la fusion\n", n)
	return nil
}
