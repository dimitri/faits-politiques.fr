package an

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
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

	for _, q := range []string{
		`SET LOCAL work_mem = '256MB'`,
		`DELETE FROM core.amendement_author a USING core.amendement m
		  WHERE m.id = a.amendement_id AND m.institution = 'ASSEMBLEE_NATIONALE'`,
		`DELETE FROM core.amendement_attribution a USING core.amendement m
		  WHERE m.id = a.amendement_id AND m.institution = 'ASSEMBLEE_NATIONALE'`,
		`DELETE FROM core.amendement WHERE institution = 'ASSEMBLEE_NATIONALE'`,
	} {
		if _, err := tx.Exec(ctx, q); err != nil {
			return fail(err)
		}
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

	vider := func() error {
		if len(lignes) == 0 {
			return nil
		}
		_, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "amendement"},
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

	// Les auteurs viennent après : ils référencent l'identifiant que la copie
	// vient d'attribuer.
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
	res, err := tx.Exec(ctx, `
		INSERT INTO core.amendement_author (amendement_id, person_id, organization_id, role, rang)
		SELECT m.id,
		       CASE WHEN a.person_id IS NOT NULL THEN a.person_id END,
		       CASE WHEN a.person_id IS NULL THEN a.org_id END,
		       a.role, 1
		  FROM amdt_auteur a
		  JOIN core.amendement m
		    ON m.source_uid = a.uid AND m.institution = 'ASSEMBLEE_NATIONALE'
		 WHERE a.person_id IS NOT NULL OR a.org_id IS NOT NULL`)
	if err != nil {
		return fail(err)
	}

	// Le groupe politique de l'auteur principal, tel que la source le publie.
	// PRIMARY_SIGNATORY : c'est le groupe du premier signataire, pas celui de
	// tous les cosignataires — l'énumération du schéma oblige à le dire.
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.amendement_attribution
		  (amendement_id, organization_id, attribution, method_version)
		SELECT m.id, a.org_id, 'PRIMARY_SIGNATORY', 'amendement-v1'
		  FROM amdt_auteur a
		  JOIN core.amendement m
		    ON m.source_uid = a.uid AND m.institution = 'ASSEMBLEE_NATIONALE'
		 WHERE a.org_id IS NOT NULL AND a.person_id IS NOT NULL
		ON CONFLICT (amendement_id) DO NOTHING`); err != nil {
		return fail(fmt.Errorf("attribution : %w", err))
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{
		"amendements": n, "auteurs": res.RowsAffected(), "sans_texte": sansTexte}, "")
	fmt.Printf("  amendements    %d, %d auteurs rattachés (%d sans texte connu)\n",
		n, res.RowsAffected(), sansTexte)
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
