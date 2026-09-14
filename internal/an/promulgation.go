package an

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PromulgationDossiers reconstruit core.dossier_promulgation : pour chaque
// dossier de l'Assemblée qui porte une étape « PROM » (promulgation) dans son
// propre flux dossierParlementaire, la référence NOR qu'il publie lui-même y
// est comparée à jo.texte.nor — une égalité EXACTE entre deux clés publiées,
// jamais un rapprochement de titres (voir internal/carto/themes.go pour le
// même principe appliqué aux thèmes de scrutin).
//
// Le JSON est déjà scellé dans raw.record (connecteur an-dossiers) : cette
// fonction ne télécharge rien, elle relit ce qui est déjà là. C'est pourquoi
// elle n'a pas de Source propre ni de StartRun — c'est un calcul dérivé, au
// même titre que carto.Themes.
func PromulgationDossiers(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM core.dossier_promulgation`); err != nil {
		return fmt.Errorf("remise à zéro : %w", err)
	}

	// jsonb_path_query descend dans actesLegislatifs.acteLegislatif, un tableau
	// dont chaque élément est une étape (dépôt, lecture, CMP, promulgation…) ;
	// le filtre ?(@.codeActe == "PROM") isole la promulgation. Sa propre
	// sous-étape (toujours codée PROM-PUB dans les dossiers observés) porte
	// codeLoi et infoJO.referenceNOR : les deux clés qui comptent ici.
	//
	// DISTINCT ON (dossier_id) : un dossier ne devrait porter qu'une seule
	// promulgation, mais le JSON source n'exclut pas une répétition de
	// l'étape ; la référence NOR la plus récente est gardée, à défaut d'un
	// signal permettant de trancher autrement.
	tag, err := tx.Exec(ctx, `
		INSERT INTO core.dossier_promulgation
		  (dossier_id, code_loi, reference_nor, numero_jo, date_jo, date_promulgation, jo_texte_id)
		SELECT DISTINCT ON (d.id)
		       d.id,
		       acte->>'codeLoi',
		       acte->'infoJO'->>'referenceNOR',
		       acte->'infoJO'->>'numJO',
		       nullif(acte->'infoJO'->>'dateJO', '')::date,
		       nullif(acte->>'dateActe', '')::date,
		       jo.id
		FROM raw.record r
		JOIN core.dossier d
		  ON d.institution = 'ASSEMBLEE_NATIONALE' AND d.source_uid = r.payload->>'uid'
		CROSS JOIN LATERAL jsonb_path_query(
		  r.payload, '$.actesLegislatifs.acteLegislatif[*] ? (@.codeActe == "PROM")'
		             '.actesLegislatifs.acteLegislatif ? (@.codeActe == "PROM-PUB")'
		) acte
		LEFT JOIN jo.texte jo ON jo.nor = acte->'infoJO'->>'referenceNOR'
		WHERE r.record_type = 'an.dossierParlementaire'
		  AND acte->>'codeLoi' IS NOT NULL
		  AND acte->'infoJO'->>'referenceNOR' IS NOT NULL
		ORDER BY d.id, nullif(acte->'infoJO'->>'dateJO', '')::date DESC NULLS LAST`)
	if err != nil {
		return fmt.Errorf("rattachement des promulgations : %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	var total, avecTexte int64
	_ = pool.QueryRow(ctx, `SELECT count(*), count(jo_texte_id) FROM core.dossier_promulgation`).
		Scan(&total, &avecTexte)
	fmt.Printf("  dossiers promulgués : %d rattachés à une référence NOR (%d inséré%s), "+
		"%d retrouvés dans le corpus JORF chargé\n",
		total, tag.RowsAffected(), plurielS(tag.RowsAffected()), avecTexte)
	return nil
}

func plurielS(n int64) string {
	if n > 1 {
		return "s"
	}
	return ""
}
