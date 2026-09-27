package carto

import (
	"context"
	"fmt"

	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ThemeMethodVersion : version de la méthode d'attribution d'un thème à un
// scrutin. Tout chiffre publié à partir de derived.scrutin_topic doit pouvoir
// être rattaché à la méthode qui l'a produit.
const ThemeMethodVersion = "theme-v1-navette"

// Themes reconstruit derived.scrutin_topic. Deux apports :
//
//	DIRECT   le thème est publié sur le scrutin lui-même (EuroVoc, au PE)
//	NAVETTE  le thème est celui de la loi du Sénat que le dossier de
//	         l'Assemblée cite explicitement dans son propre lien senat_chemin
//
// Le chemin de la navette passe par le SIGNET du Sénat (« ppl24-125 »), que
// l'Assemblée reproduit tel quel dans l'URL du dossier sénatorial. C'est une
// égalité exacte entre deux clés publiées, pas un rapprochement de titres : si
// le signet n'est pas cité, il n'y a pas de thème, et c'est très bien ainsi.
//
// Les tables de correspondance sont matérialisées avant la jointure. La forme
// naïve joindrait sur une expression (regexp_replace d'un côté, trim d'un
// character(36) de l'autre) : aucun index utilisable, et un tri de plusieurs
// millions de lignes. Douze mille lignes indexées à la place.
func Themes(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	prep := []string{
		`CREATE TEMP TABLE map_signet ON COMMIT DROP AS
		   SELECT trim(signet) AS signet, trim(loicod) AS loicod
		     FROM senat_raw.loi WHERE signet IS NOT NULL`,
		`CREATE INDEX ON map_signet (signet)`,
		`CREATE TEMP TABLE map_navette ON COMMIT DROP AS
		   SELECT id AS dossier_id,
		          regexp_replace(senat_chemin, '^.*/([^/]+)\.html$', '\1') AS signet
		     FROM core.dossier
		    WHERE institution = 'ASSEMBLEE_NATIONALE' AND senat_chemin IS NOT NULL`,
		`CREATE INDEX ON map_navette (signet)`,
		`DELETE FROM derived.scrutin_topic`,
	}
	for _, q := range prep {
		if _, err := tx.Exec(ctx, q); err != nil {
			return fmt.Errorf("préparation des thèmes : %w", err)
		}
	}

	direct, err := tx.Exec(ctx, `
		INSERT INTO derived.scrutin_topic
		  (scrutin_id, topic_code, origine, via_dossier_id, method_version)
		SELECT DISTINCT ta.scrutin_id, ta.topic_code, 'DIRECT', NULL::bigint, $1
		  FROM core.topic_assignment ta
		 WHERE ta.scrutin_id IS NOT NULL`, ThemeMethodVersion)
	if err != nil {
		return fmt.Errorf("thèmes directs : %w", err)
	}

	// DISTINCT ON : une même loi peut être citée par plusieurs dossiers de
	// l'Assemblée. On retient le dossier de plus petit identifiant pour que le
	// résultat soit reproductible à l'identique d'un calcul à l'autre.
	navette, err := tx.Exec(ctx, `
		INSERT INTO derived.scrutin_topic
		  (scrutin_id, topic_code, origine, via_dossier_id, method_version)
		SELECT DISTINCT ON (sc.id, ta.topic_code)
		       sc.id, ta.topic_code, 'NAVETTE', ds.id, $1
		  FROM core.scrutin sc
		  JOIN map_navette mn ON mn.dossier_id = sc.dossier_id
		  JOIN map_signet  ms ON ms.signet = mn.signet
		  JOIN core.dossier ds
		    ON ds.institution = 'SENAT' AND ds.source_uid = ms.loicod
		  JOIN core.topic_assignment ta ON ta.dossier_id = ds.id
		 WHERE sc.institution = 'ASSEMBLEE_NATIONALE'
		   AND NOT EXISTS (SELECT 1 FROM derived.scrutin_topic st
		                    WHERE st.scrutin_id = sc.id AND st.topic_code = ta.topic_code)
		 ORDER BY sc.id, ta.topic_code, ds.id`, ThemeMethodVersion)
	if err != nil {
		return fmt.Errorf("thèmes hérités par la navette : %w", err)
	}

	// La couverture est publiée, pas seulement constatée : un thème absent sur
	// les deux tiers des scrutins change la lecture de toute statistique qui
	// s'appuie dessus.
	for _, c := range []struct{ scope, metric string }{
		{"ASSEMBLEE_NATIONALE", "scrutins_avec_theme"},
		{"PARLEMENT_EUROPEEN", "scrutins_avec_theme"},
		{"SENAT", "scrutins_avec_theme"},
	} {
		// HAVING count(*) > 0 : un dénominateur nul violerait
		// coverage_denominator_check, à raison — une couverture sur zéro
		// scrutin ne veut rien dire. Ce n'est pas une anomalie : cette
		// institution n'a simplement pas encore été ingérée (l'ordre du
		// pipeline place le Sénat avant le Parlement européen). Ne rien
		// insérer plutôt que forcer une ligne creuse ; la prochaine
		// exécution, une fois la source chargée, l'ajoutera normalement.
		if _, err := tx.Exec(ctx, `
			INSERT INTO derived.coverage
			  (scope, metric, numerator, denominator, method_version)
			SELECT $1, $2,
			       count(*) FILTER (WHERE EXISTS (SELECT 1 FROM derived.scrutin_topic st
			                                       WHERE st.scrutin_id = sc.id)),
			       count(*), $3
			  FROM core.scrutin sc WHERE sc.institution = $4::core.institution
			HAVING count(*) > 0
			ON CONFLICT DO NOTHING`, c.scope, c.metric, ThemeMethodVersion, c.scope); err != nil {
			return fmt.Errorf("couverture %s : %w", c.scope, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	logs.Notice(fmt.Sprintf("%s direct, %s inherited via the shuttle",
		logs.Plural(int(direct.RowsAffected()), "topic"), logs.Plural(int(navette.RowsAffected()), "topic")))
	return nil
}
