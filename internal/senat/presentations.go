package senat

import (
	"context"
	"fmt"

	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PresentationVersion identifie la règle de reprise, pas la donnée.
const PresentationVersion = "presentation-senat-v1"

// NormalizePresentations reprend l'objet que le Sénat écrit pour chacun de ses
// dossiers.
//
// Rien à télécharger : tout est dans le dump Dosleg déjà scellé. `senat_raw.loi`
// porte `objet` — une présentation rédigée, parfois longue de plusieurs pages —
// `motclef`, et l'adresse de la page « La loi en clair » quand le service des
// études en a fait une.
//
// Les 12 429 dossiers du Sénat se rattachent tous à une ligne de `loi` par
// `loicod` : jointure sur identifiant, aucun rapprochement de titre.
func NormalizePresentations(ctx context.Context, pool *pgxpool.Pool) error {
	var srcID int64
	if err := pool.QueryRow(ctx,
		`SELECT id FROM raw.source WHERE slug = 'senat-dosleg'`).Scan(&srcID); err != nil {
		return fmt.Errorf("source du Sénat introuvable : %w", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Portée bornée aux dossiers du Sénat : ce connecteur ne possède qu'eux.
	if _, err := tx.Exec(ctx, `
		DELETE FROM core.dossier_presentation p
		 USING core.dossier d
		 WHERE d.id = p.dossier_id AND d.institution = 'SENAT'`); err != nil {
		return err
	}

	res, err := tx.Exec(ctx, `
		INSERT INTO core.dossier_presentation
		  (dossier_id, objet, mots_clefs, en_clair_url, source_id, method_version)
		SELECT d.id,
		       nullif(trim(l.objet), ''),
		       nullif(trim(coalesce(l.motclef, '')), ''),
		       nullif(trim(coalesce(l.en_clair_url, '')), ''),
		       $1, $2
		  FROM core.dossier d
		  JOIN senat_raw.loi l ON trim(l.loicod) = d.source_uid
		 WHERE d.institution = 'SENAT'
		   AND (nullif(trim(l.objet), '') IS NOT NULL
		     OR nullif(trim(coalesce(l.motclef, '')), '') IS NOT NULL
		     OR nullif(trim(coalesce(l.en_clair_url, '')), '') IS NOT NULL)`,
		srcID, PresentationVersion)
	if err != nil {
		return fmt.Errorf("présentations : %w", err)
	}

	var objets, motsClefs, enClair int
	if err := tx.QueryRow(ctx, `
		SELECT count(objet), count(mots_clefs), count(en_clair_url)
		  FROM core.dossier_presentation`).Scan(&objets, &motsClefs, &enClair); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	logs.Notice(fmt.Sprintf("Senate presentations: %s, %s written, %s of keywords, %s plain-text",
		logs.Plural(int(res.RowsAffected()), "bill"), logs.Plural(objets, "summary"),
		logs.Plural(motsClefs, "set"), logs.Plural(enClair, "page")))
	return nil
}
