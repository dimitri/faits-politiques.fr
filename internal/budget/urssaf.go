package budget

import (
	"context"
	"fmt"
	"strconv"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// L'URSSAF publie cent vingt-quatre jeux, tous en ODbL. Ils décrivent l'ACTIVITÉ
// du recouvrement — combien d'exonérations, combien de masse salariale, combien
// de redressements — et jamais les COMPTES : on ne trouve nulle part le tableau
// d'équilibre d'une branche.
//
// L'ODbL est redistribuable, y compris commercialement, mais impose le partage à
// l'identique : toute base dérivée qui en incorpore le contenu doit être publiée
// sous la même licence. L'obligation contamine les exports, d'où sa trace ici et
// dans Notes — le projet a une vue raw.source_redistribuable qu'il faudra savoir
// justifier ligne à ligne.
var SourceURSSAFExonerations = archive.Source{
	Slug:        "urssaf-exonerations",
	Label:       "URSSAF — exonérations de cotisations par mesure",
	Publisher:   "Urssaf Caisse nationale",
	Tier:        "PRIMARY_OFFICIAL",
	Licence:     "Open Database License (ODbL) 1.0",
	ReuseClass:  "ATTRIBUTION",
	Attribution: "Source : Urssaf Caisse nationale, données ouvertes (ODbL)",
	Cadence:     "annuelle",
	Notes: "ODbL : PARTAGE À L'IDENTIQUE. Toute base dérivée incorporant ces données " +
		"doit être publiée sous ODbL ; l'obligation contamine les exports et doit être " +
		"vérifiée avant toute redistribution. " +
		"Le jeu donne le montant des allègements, pas leur COMPENSATION : une " +
		"exonération compensée par l'État et une exonération non compensée y figurent à " +
		"l'identique. 2,63 Md€ restaient officiellement non compensés en 2026.",
}

var SourceURSSAFMasseSalariale = archive.Source{
	Slug:        "urssaf-masse-salariale",
	Label:       "URSSAF — masse salariale du secteur privé",
	Publisher:   "Urssaf Caisse nationale",
	Tier:        "PRIMARY_OFFICIAL",
	Licence:     "Open Database License (ODbL) 1.0",
	ReuseClass:  "ATTRIBUTION",
	Attribution: "Source : Urssaf Caisse nationale, données ouvertes (ODbL)",
	Cadence:     "trimestrielle",
	Notes: "ODbL : partage à l'identique, voir urssaf-exonerations. " +
		"Champ secteur privé France entière : ni fonction publique, ni régime agricole. " +
		"Les colonnes CVS sont corrigées des variations saisonnières et ne doivent pas " +
		"être totalisées sur l'année.",
}

type ligneExo struct {
	GrandeCategorie     string   `json:"grande_categorie_de_mesures"`
	CodeGrandeCategorie string   `json:"code_grande_categorie_de_mesures"`
	Categorie           string   `json:"categorie_de_mesures"`
	CodeCategorie       string   `json:"code_categorie_de_mesures"`
	Mesure              string   `json:"mesure_d_exoneration"`
	CodeMesure          string   `json:"code_mesure_d_exoneration"`
	Annee               string   `json:"annee"`
	Montant             *float64 `json:"montant_des_exonerations"`
}

type ligneMasse struct {
	Annee       string   `json:"annee"`
	Trimestre   *int     `json:"trimestre"`
	DernierJour string   `json:"dernier_jour_trim"`
	Brut50      *float64 `json:"ms_t_50j_brut"`
	Brut60      *float64 `json:"ms_t_60j_brut"`
	Cvs50       *float64 `json:"ms_t_50j_cvs"`
	Cvs60       *float64 `json:"ms_t_60j_cvs"`
}

// IngestURSSAF charge les deux jeux du recouvrement : les allègements, et
// l'assiette sur laquelle ils s'appliquent. Les deux vont ensemble — un montant
// d'exonérations qui augmente pendant que la masse salariale augmente autant ne
// dit pas la même chose qu'un montant qui augmente seul.
func IngestURSSAF(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	if err := ingestExonerations(ctx, pool, arch); err != nil {
		return err
	}
	return ingestMasseSalariale(ctx, pool, arch)
}

func ingestExonerations(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceURSSAFExonerations)
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

	f, err := arch.Fetch(ctx, srcID, runID,
		exportJSON("open.urssaf.fr", "exos-secteur-prive-france-entiere-par-mesures"), ".json")
	if err != nil {
		return fail(err)
	}
	var lignes []ligneExo
	if err := lireJSON(f.Path, &lignes); err != nil {
		return fail(err)
	}
	if len(lignes) == 0 {
		return fail(fmt.Errorf("URSSAF exonérations : export vide"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	rows := make([][]any, 0, len(lignes))
	for _, l := range lignes {
		annee, err := strconv.Atoi(l.Annee)
		if err != nil {
			return fail(fmt.Errorf("URSSAF exonérations : année illisible %q", l.Annee))
		}
		if l.CodeMesure == "" {
			return fail(fmt.Errorf("URSSAF exonérations : mesure sans code en %d (%q)", annee, l.Mesure))
		}
		rows = append(rows, []any{
			annee, l.CodeGrandeCategorie, l.GrandeCategorie,
			l.CodeCategorie, l.Categorie, l.CodeMesure, l.Mesure,
			nulF(l.Montant), "SECTEUR_PRIVE_URSSAF", srcID, f.DocumentID,
		})
	}
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_exoneration_cotisation (
			annee smallint NOT NULL,
			code_grande_categorie text NOT NULL,
			grande_categorie text NOT NULL,
			code_categorie text NOT NULL,
			categorie text NOT NULL,
			code_mesure text NOT NULL,
			mesure text NOT NULL,
			montant_eur double precision,
			perimetre text NOT NULL,
			source_id bigint NOT NULL,
			document_id bigint NOT NULL
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_exoneration_cotisation"},
		[]string{"annee", "code_grande_categorie", "grande_categorie",
			"code_categorie", "categorie", "code_mesure", "mesure",
			"montant_eur", "perimetre", "source_id", "document_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(fmt.Errorf("URSSAF exonérations : %w", err))
	}
	ct, err := tx.Exec(ctx, `
		MERGE INTO core.exoneration_cotisation AS tgt
		USING tmp_exoneration_cotisation AS src
		ON tgt.annee = src.annee AND tgt.code_mesure = src.code_mesure
		WHEN MATCHED AND (tgt.code_grande_categorie, tgt.grande_categorie, tgt.code_categorie,
				tgt.categorie, tgt.mesure, tgt.montant_eur, tgt.perimetre,
				tgt.source_id, tgt.document_id)
			IS DISTINCT FROM (src.code_grande_categorie, src.grande_categorie, src.code_categorie,
				src.categorie, src.mesure, src.montant_eur, src.perimetre,
				src.source_id, src.document_id) THEN
			UPDATE SET code_grande_categorie = src.code_grande_categorie,
				grande_categorie = src.grande_categorie, code_categorie = src.code_categorie,
				categorie = src.categorie, mesure = src.mesure, montant_eur = src.montant_eur,
				perimetre = src.perimetre, source_id = src.source_id, document_id = src.document_id
		WHEN NOT MATCHED BY TARGET THEN
			INSERT (annee, code_grande_categorie, grande_categorie, code_categorie, categorie,
				code_mesure, mesure, montant_eur, perimetre, source_id, document_id)
			VALUES (src.annee, src.code_grande_categorie, src.grande_categorie, src.code_categorie,
				src.categorie, src.code_mesure, src.mesure, src.montant_eur, src.perimetre,
				src.source_id, src.document_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("URSSAF exonérations, fusion : %w", err))
	}
	n := ct.RowsAffected()

	var annees, min, max int
	var total float64
	if err := tx.QueryRow(ctx, `
		SELECT count(DISTINCT annee), min(annee), max(annee),
		       coalesce(sum(montant_eur) FILTER (WHERE annee = (SELECT max(annee) FROM core.exoneration_cotisation)), 0)
		  FROM core.exoneration_cotisation`).Scan(&annees, &min, &max, &total); err != nil {
		return fail(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": n, "annees": annees}, "")
	fmt.Printf("  URSSAF exonérations : %d mesures touchées par la fusion, %d millésimes de %d à %d (%.1f Md€ en %d)\n",
		n, annees, min, max, total/1e9, max)
	return nil
}

func ingestMasseSalariale(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceURSSAFMasseSalariale)
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

	f, err := arch.Fetch(ctx, srcID, runID,
		exportJSON("open.urssaf.fr", "masse-salariale-du-secteur-prive-france-entiere"), ".json")
	if err != nil {
		return fail(err)
	}
	var lignes []ligneMasse
	if err := lireJSON(f.Path, &lignes); err != nil {
		return fail(err)
	}
	if len(lignes) == 0 {
		return fail(fmt.Errorf("URSSAF masse salariale : export vide"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	rows := make([][]any, 0, len(lignes))
	for _, l := range lignes {
		annee, err := strconv.Atoi(l.Annee)
		if err != nil {
			return fail(fmt.Errorf("URSSAF masse salariale : année illisible %q", l.Annee))
		}
		if l.Trimestre == nil || *l.Trimestre < 1 || *l.Trimestre > 4 {
			return fail(fmt.Errorf("URSSAF masse salariale : trimestre absent ou hors bornes en %d", annee))
		}
		if l.DernierJour == "" {
			return fail(fmt.Errorf("URSSAF masse salariale : dernier jour de trimestre absent en %d T%d",
				annee, *l.Trimestre))
		}
		rows = append(rows, []any{
			annee, *l.Trimestre, l.DernierJour,
			nulF(l.Brut50), nulF(l.Brut60), nulF(l.Cvs50), nulF(l.Cvs60),
			"SECTEUR_PRIVE_URSSAF", srcID, f.DocumentID,
		})
	}
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_masse_salariale (
			annee smallint NOT NULL,
			trimestre smallint NOT NULL,
			dernier_jour date NOT NULL,
			brut_50j_eur double precision,
			brut_60j_eur double precision,
			cvs_50j_eur double precision,
			cvs_60j_eur double precision,
			perimetre text NOT NULL,
			source_id bigint NOT NULL,
			document_id bigint NOT NULL
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_masse_salariale"},
		[]string{"annee", "trimestre", "dernier_jour", "brut_50j_eur", "brut_60j_eur",
			"cvs_50j_eur", "cvs_60j_eur", "perimetre", "source_id", "document_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(fmt.Errorf("URSSAF masse salariale : %w", err))
	}
	ct, err := tx.Exec(ctx, `
		MERGE INTO core.masse_salariale AS tgt
		USING tmp_masse_salariale AS src
		ON tgt.annee = src.annee AND tgt.trimestre = src.trimestre
		WHEN MATCHED AND (tgt.dernier_jour, tgt.brut_50j_eur, tgt.brut_60j_eur,
				tgt.cvs_50j_eur, tgt.cvs_60j_eur, tgt.perimetre, tgt.source_id, tgt.document_id)
			IS DISTINCT FROM (src.dernier_jour, src.brut_50j_eur, src.brut_60j_eur,
				src.cvs_50j_eur, src.cvs_60j_eur, src.perimetre, src.source_id, src.document_id) THEN
			UPDATE SET dernier_jour = src.dernier_jour, brut_50j_eur = src.brut_50j_eur,
				brut_60j_eur = src.brut_60j_eur, cvs_50j_eur = src.cvs_50j_eur,
				cvs_60j_eur = src.cvs_60j_eur, perimetre = src.perimetre,
				source_id = src.source_id, document_id = src.document_id
		WHEN NOT MATCHED BY TARGET THEN
			INSERT (annee, trimestre, dernier_jour, brut_50j_eur, brut_60j_eur,
				cvs_50j_eur, cvs_60j_eur, perimetre, source_id, document_id)
			VALUES (src.annee, src.trimestre, src.dernier_jour, src.brut_50j_eur, src.brut_60j_eur,
				src.cvs_50j_eur, src.cvs_60j_eur, src.perimetre, src.source_id, src.document_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("URSSAF masse salariale, fusion : %w", err))
	}
	n := ct.RowsAffected()

	var min, max int
	if err := tx.QueryRow(ctx,
		`SELECT min(annee), max(annee) FROM core.masse_salariale`).Scan(&min, &max); err != nil {
		return fail(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"trimestres": n}, "")
	fmt.Printf("  URSSAF masse salariale : %d trimestres touchés par la fusion, de %d à %d\n", n, min, max)
	return nil
}
