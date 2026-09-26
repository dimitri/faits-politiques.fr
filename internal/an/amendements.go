package an

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/bulkload"
	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Les amendements, et surtout leur EXPOSÉ SOMMAIRE — le seul endroit où un
// député écrit lui-même ce qu'il veut changer et pourquoi.
//
// 123 262 fichiers JSON dans une archive de 283 Mo, un par amendement. Lus en
// flux : décompresser l'archive entière sur disque coûterait plusieurs giga-
// octets pour rien.
//
// L'exposé sommaire est un ARGUMENT, pas une description neutre : son auteur
// défend son amendement. Il est stocké verbatim et cité comme tel, jamais
// présenté comme un résumé objectif — même règle que pour l'exposé des motifs.
var SourceAmendements = archive.Source{
	Slug: "an-amendements", Label: "Assemblée nationale — amendements",
	Publisher: "Assemblée nationale", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Licence Ouverte",
	ReuseClass:  "ATTRIBUTION",
	Attribution: "Source : Assemblée nationale, amendements de la 17e législature",
	Cadence:     "en continu",
	Notes: "Un fichier par amendement. L'exposé sommaire est rédigé par l'auteur : " +
		"c'est un plaidoyer, pas une description.",
}

const amendementsURL = Base + "/loi/amendements_div_legis/Amendements.json.zip"

// Le sort publié par l'Assemblée, traduit vers l'énumération du schéma. Les
// libellés inconnus laissent le sort NULL plutôt que d'être rangés au hasard
// dans une catégorie voisine.
var sortAmendement = map[string]string{
	"adopté": "ADOPTE", "rejeté": "REJETE", "retiré": "RETIRE",
	"non soutenu": "NON_SOUTENU", "tombé": "TOMBE",
	"irrecevable": "IRRECEVABLE", "irrecevable 40": "IRRECEVABLE",
	"irrecevable 41": "IRRECEVABLE", "non examiné": "NON_EXAMINE",
	"retiré avant séance": "RETIRE", "retiré en commission": "RETIRE",
}

type amendement struct {
	UID            string  `json:"uid"`
	Legislature    flexStr `json:"legislature"`
	Identification struct {
		NumeroLong flexStr `json:"numeroLong"`
	} `json:"identification"`
	TexteLegislatifRef json.RawMessage `json:"texteLegislatifRef"`
	Signataires        struct {
		Auteur struct {
			TypeAuteur         flexStr         `json:"typeAuteur"`
			ActeurRef          json.RawMessage `json:"acteurRef"`
			GroupePolitiqueRef json.RawMessage `json:"groupePolitiqueRef"`
		} `json:"auteur"`
		Libelle flexStr `json:"libelle"`
	} `json:"signataires"`
	PointeurFragmentTexte struct {
		Division struct {
			Titre flexStr `json:"titre"`
		} `json:"division"`
	} `json:"pointeurFragmentTexte"`
	Corps struct {
		ContenuAuteur struct {
			Dispositif     flexStr `json:"dispositif"`
			ExposeSommaire flexStr `json:"exposeSommaire"`
		} `json:"contenuAuteur"`
	} `json:"corps"`
	CycleDeVie struct {
		DateDepot flexStr `json:"dateDepot"`
		Sort      flexStr `json:"sort"`
	} `json:"cycleDeVie"`
}

func IngestAmendements(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceAmendements)
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

	f, err := arch.Fetch(ctx, srcID, runID, amendementsURL, ".zip")
	if err != nil {
		return fail(err)
	}

	// Les textes et les personnes sont chargés une fois en mémoire : 123 000
	// amendements font 123 000 recherches, et les faire en base multiplierait
	// par dix le temps de chargement.
	textes, err := indexTextes(ctx, pool)
	if err != nil {
		return fail(err)
	}
	acteurs, err := indexActeurs(ctx, pool)
	if err != nil {
		return fail(err)
	}
	groupes, err := indexOrganes(ctx, pool)
	if err != nil {
		return fail(err)
	}

	zr, err := zip.OpenReader(f.Path)
	if err != nil {
		return fail(err)
	}
	defer zr.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	// MERGE plutôt que DELETE+COPY, scopé à l'institution par une vue
	// temporaire (core.amendement pourrait un jour porter des amendements
	// SENAT) : l'ancien DELETE payait le prix des triggers RI — dont la
	// cascade vers amendement_author et amendement_attribution — pour
	// l'intégralité des 125 000 amendements de l'Assemblée à chaque
	// republication, changement ou non.
	if _, err := tx.Exec(ctx, `
		CREATE OR REPLACE TEMPORARY VIEW amendement_an AS
		  SELECT * FROM core.amendement WHERE institution = 'ASSEMBLEE_NATIONALE'
		  WITH LOCAL CHECK OPTION`); err != nil {
		return fail(err)
	}

	type auteur struct {
		uid    string
		person *int64
		org    *int64
		role   string
	}
	var lignes [][]any
	var auteurs []auteur
	var n, sansTexte int
	vus := map[string]bool{}

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_amendement (
			slug text, texte_id bigint, institution core.institution, source_uid text, numero text,
			article_designation text, sort core.amendement_sort, expose_sommaire text,
			dispositif text, date_depot date
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	vider := func() error {
		if len(lignes) == 0 {
			return nil
		}
		_, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_amendement"},
			[]string{"slug", "texte_id", "institution", "source_uid", "numero",
				"article_designation", "sort", "expose_sommaire", "dispositif", "date_depot"},
			pgx.CopyFromRows(lignes))
		lignes = lignes[:0]
		return err
	}

	for _, zf := range zr.File {
		if !strings.HasSuffix(zf.Name, ".json") {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			continue
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}
		var doc struct {
			Amendement amendement `json:"amendement"`
		}
		if err := json.Unmarshal(b, &doc); err != nil {
			continue
		}
		a := doc.Amendement
		if a.UID == "" || vus[a.UID] {
			continue
		}
		vus[a.UID] = true

		var texteID any
		if id, ok := textes[str(a.TexteLegislatifRef)]; ok {
			texteID = id
		} else {
			sansTexte++
		}

		var sort any
		if s, ok := sortAmendement[strings.ToLower(strings.TrimSpace(a.CycleDeVie.Sort.String()))]; ok {
			sort = s
		}

		lignes = append(lignes, []any{
			strings.ToLower(a.UID), texteID, "ASSEMBLEE_NATIONALE", a.UID,
			nulA(a.Identification.NumeroLong.String()),
			nulA(a.PointeurFragmentTexte.Division.Titre.String()),
			sort,
			nulA(texteBrut(a.Corps.ContenuAuteur.ExposeSommaire.String())),
			nulA(texteBrut(a.Corps.ContenuAuteur.Dispositif.String())),
			nulA(a.CycleDeVie.DateDepot.String()),
		})

		// Le rôle doit appartenir à l'énumération du schéma. Le libellé de
		// l'Assemblée — « Député », « Gouvernement », « Commission » — est
		// traduit ; ce qui n'est pas reconnu devient AUTEUR, qui est le fait
		// minimal et vrai : cette personne a déposé cet amendement.
		au := auteur{uid: a.UID, role: roleAuteur(a.Signataires.Auteur.TypeAuteur.String())}
		if p, ok := acteurs[str(a.Signataires.Auteur.ActeurRef)]; ok {
			v := p
			au.person = &v
		}
		if g, ok := groupes[str(a.Signataires.Auteur.GroupePolitiqueRef)]; ok {
			v := g
			au.org = &v
		}
		if au.person != nil || au.org != nil {
			auteurs = append(auteurs, au)
		}

		n++
		if len(lignes) >= 20000 {
			if err := vider(); err != nil {
				return fail(fmt.Errorf("copie des amendements : %w", err))
			}
		}
	}
	if err := vider(); err != nil {
		return fail(fmt.Errorf("copie des amendements : %w", err))
	}

	// Pas de RETURNING sur ce MERGE : il n'émettrait une ligne que pour un
	// amendement dont l'UPDATE a réellement changé quelque chose — le cas
	// courant, sur un exposé sommaire déjà chargé, étant justement qu'il n'a
	// pas changé. Les auteurs, juste après, référencent l'identifiant de
	// CHAQUE amendement du lot, pas seulement ceux que le MERGE a touchés :
	// un SELECT séparé, sans dépendre d'un WHEN, reconstruit la carte en
	// entier.
	var nMerge int64
	err = bulkload.SansContraintesFK(ctx, tx, "core.amendement", func() error {
		ct, err := tx.Exec(ctx, `
			MERGE INTO amendement_an AS tgt
			USING tmp_amendement AS src
			ON tgt.source_uid = src.source_uid
			WHEN MATCHED AND (tgt.slug, tgt.texte_id, tgt.numero, tgt.article_designation,
			                   tgt.sort, tgt.expose_sommaire, tgt.dispositif, tgt.date_depot)
			                  IS DISTINCT FROM
			                  (src.slug, src.texte_id, src.numero, src.article_designation,
			                   src.sort, src.expose_sommaire, src.dispositif, src.date_depot) THEN
			    UPDATE SET slug = src.slug, texte_id = src.texte_id, numero = src.numero,
			               article_designation = src.article_designation, sort = src.sort,
			               expose_sommaire = src.expose_sommaire, dispositif = src.dispositif,
			               date_depot = src.date_depot
			WHEN NOT MATCHED BY TARGET THEN
			    INSERT (slug, texte_id, institution, source_uid, numero, article_designation,
			            sort, expose_sommaire, dispositif, date_depot)
			    VALUES (src.slug, src.texte_id, src.institution, src.source_uid, src.numero,
			            src.article_designation, src.sort, src.expose_sommaire, src.dispositif,
			            src.date_depot)
			WHEN NOT MATCHED BY SOURCE THEN DELETE`)
		if err != nil {
			return err
		}
		nMerge = ct.RowsAffected()
		return nil
	})
	if err != nil {
		return fail(fmt.Errorf("fusion des amendements : %w", err))
	}

	// Les auteurs viennent après : ils référencent l'identifiant du MERGE
	// ci-dessus, résolu par jointure sur source_uid — jamais par RETURNING,
	// pour la même raison.
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE amdt_auteur (uid text, person_id bigint, org_id bigint, role text)
		ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	rows := make([][]any, 0, len(auteurs))
	for _, a := range auteurs {
		rows = append(rows, []any{a.uid, a.person, a.org, a.role})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"amdt_auteur"},
		[]string{"uid", "person_id", "org_id", "role"}, pgx.CopyFromRows(rows)); err != nil {
		return fail(err)
	}
	// La contrainte du schéma impose EXACTEMENT une cible : une personne OU une
	// organisation. C'est juste — un amendement du Gouvernement n'a pas
	// d'auteur individuel, un amendement de député si. Le groupe politique de
	// l'auteur n'est pas son auteur : il va dans amendement_attribution, qui
	// est une attribution DÉRIVÉE et porte son method_version.
	//
	// MERGE plutôt qu'INSERT, scopé par une vue restreinte aux auteurs
	// d'amendements de l'Assemblée (amendement_author n'a pas sa propre
	// colonne d'institution) : sans le DELETE qui précédait cette section
	// dans l'ancien code, un simple INSERT dupliquerait chaque auteur à
	// chaque republication. amendement_author_amendement_id_key (migration
	// 0180) fournit la clé naturelle qu'aucune contrainte ne portait avant.
	if _, err := tx.Exec(ctx, `
		CREATE OR REPLACE TEMPORARY VIEW amendement_author_an AS
		  SELECT * FROM core.amendement_author
		   WHERE amendement_id IN (SELECT id FROM amendement_an)
		  WITH LOCAL CHECK OPTION`); err != nil {
		return fail(err)
	}
	res, err := tx.Exec(ctx, `
		MERGE INTO amendement_author_an AS tgt
		USING (
		  SELECT m.id AS amendement_id,
		         CASE WHEN a.person_id IS NOT NULL THEN a.person_id END AS person_id,
		         CASE WHEN a.person_id IS NULL THEN a.org_id END AS organization_id,
		         a.role, 1 AS rang
		    FROM amdt_auteur a
		    JOIN amendement_an m ON m.source_uid = a.uid
		   WHERE a.person_id IS NOT NULL OR a.org_id IS NOT NULL
		) AS src
		ON tgt.amendement_id = src.amendement_id
		WHEN MATCHED AND (tgt.person_id, tgt.organization_id, tgt.role, tgt.rang)
		                  IS DISTINCT FROM (src.person_id, src.organization_id, src.role, src.rang) THEN
		    UPDATE SET person_id = src.person_id, organization_id = src.organization_id,
		               role = src.role, rang = src.rang
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (amendement_id, person_id, organization_id, role, rang)
		    VALUES (src.amendement_id, src.person_id, src.organization_id, src.role, src.rang)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(err)
	}

	// Le groupe politique de l'auteur principal, tel que la source le publie.
	// PRIMARY_SIGNATORY : c'est le groupe du premier signataire, pas celui de
	// tous les cosignataires — l'énumération du schéma oblige à le dire.
	if _, err := tx.Exec(ctx, `
		CREATE OR REPLACE TEMPORARY VIEW amendement_attribution_an AS
		  SELECT * FROM core.amendement_attribution
		   WHERE amendement_id IN (SELECT id FROM amendement_an)
		  WITH LOCAL CHECK OPTION`); err != nil {
		return fail(err)
	}
	if _, err := tx.Exec(ctx, `
		MERGE INTO amendement_attribution_an AS tgt
		USING (
		  SELECT m.id AS amendement_id, a.org_id AS organization_id,
		         'PRIMARY_SIGNATORY'::core.author_attribution AS attribution, 'amendement-v1' AS method_version
		    FROM amdt_auteur a
		    JOIN amendement_an m ON m.source_uid = a.uid
		   WHERE a.org_id IS NOT NULL AND a.person_id IS NOT NULL
		) AS src
		ON tgt.amendement_id = src.amendement_id
		WHEN MATCHED AND (tgt.organization_id, tgt.attribution, tgt.method_version)
		                  IS DISTINCT FROM (src.organization_id, src.attribution, src.method_version) THEN
		    UPDATE SET organization_id = src.organization_id, attribution = src.attribution,
		               method_version = src.method_version
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (amendement_id, organization_id, attribution, method_version)
		    VALUES (src.amendement_id, src.organization_id, src.attribution, src.method_version)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`); err != nil {
		return fail(fmt.Errorf("attribution : %w", err))
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{
		"amendements": n, "amendements_touches": nMerge, "auteurs": res.RowsAffected(),
		"sans_texte": sansTexte}, "")
	logs.Notice(fmt.Sprintf("%s (%d touched by the merge), %s linked (%d without a known text)",
		logs.Plural(n, "amendment"), nMerge, logs.Plural(int(res.RowsAffected()), "author"), sansTexte))
	return nil
}

func indexTextes(ctx context.Context, pool *pgxpool.Pool) (map[string]int64, error) {
	return indexUID(ctx, pool, `SELECT source_uid, id FROM core.texte WHERE institution='ASSEMBLEE_NATIONALE'`)
}

func indexActeurs(ctx context.Context, pool *pgxpool.Pool) (map[string]int64, error) {
	return indexUID(ctx, pool, `SELECT value, person_id FROM core.person_identifier WHERE scheme='AN_ACTEUR'`)
}

func indexOrganes(ctx context.Context, pool *pgxpool.Pool) (map[string]int64, error) {
	return indexUID(ctx, pool, `SELECT value, organization_id FROM core.organization_identifier WHERE scheme='AN_ORGANE'`)
}

func indexUID(ctx context.Context, pool *pgxpool.Pool, q string) (map[string]int64, error) {
	rows, err := pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var k string
		var v int64
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func nulA(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

// roleAuteur traduit le type d'auteur publié par l'Assemblée vers les valeurs
// admises par le schéma.
func roleAuteur(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "gouvernement":
		return "GOUVERNEMENT"
	case "commission":
		return "COMMISSION"
	case "rapporteur":
		return "RAPPORTEUR"
	default:
		return "AUTEUR"
	}
}
