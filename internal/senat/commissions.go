package senat

import (
	"context"
	"fmt"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Les appartenances aux commissions du Sénat, depuis 1983.
//
// C'est l'équivalent sénatorial des 28 441 appartenances de l'Assemblée : où un
// parlementaire travaille réellement, et avec quelle fonction — membre,
// vice-président, rapporteur. Le Sénat le publie dans son répertoire, indexé
// par MATRICULE : appariement par identifiant, sans risque d'homonymie.
const commissionsURL = "https://data.senat.fr/data/senateurs/ODSEN_COMS.csv"

func IngestCommissions(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceSenateurs)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, Version)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, commissionsURL, ".csv")
	if err != nil {
		return fail(err)
	}
	recs, err := lireCSVSenat(f.Path)
	if err != nil {
		return fail(err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	// Portée bornée : les appartenances rattachées à un organe du Sénat.
	if _, err := tx.Exec(ctx, `
		DELETE FROM core.affiliation a
		 WHERE EXISTS (SELECT 1 FROM core.organization_identifier i
		               WHERE i.organization_id = a.organization_id
		                 AND i.scheme = 'SENAT_GROUPE')`); err != nil {
		return fail(err)
	}

	// Les organes du Sénat. La source ne publie pas d'identifiant d'organe :
	// le nom de la commission EST la clé, et il est stable dans le fichier.
	// C'est une égalité de chaîne publiée par un producteur avec lui-même, pas
	// un rapprochement entre deux sources.
	organes := map[string]int64{}
	for _, r := range recs {
		nom := strings.TrimSpace(r["Nom commission"])
		if nom == "" || organes[nom] != 0 {
			continue
		}
		typ := strings.TrimSpace(r["Type commission"])
		kind := "PARLIAMENTARY_BODY"
		if strings.HasPrefix(strings.ToLower(typ), "commission") {
			kind = "COMMITTEE"
		}
		slug := "senat-" + slugSenat(nom)
		var id int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO core.organization (slug, kind, name, organ_type)
			VALUES ($1, $2::core.organization_kind, $3, $4)
			ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
			RETURNING id`, slug, kind, nom, nulS(typ)).Scan(&id); err != nil {
			return fail(fmt.Errorf("organe %q : %w", nom, err))
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.organization_identifier (organization_id, scheme, value)
			VALUES ($1, 'SENAT_GROUPE', $2) ON CONFLICT (scheme, value) DO NOTHING`,
			id, slug); err != nil {
			return fail(err)
		}
		organes[nom] = id
	}

	var lignes [][]any
	var sansPersonne int
	vus := map[string]bool{}
	for _, r := range recs {
		mat := strings.TrimSpace(r["Matricule"])
		nom := strings.TrimSpace(r["Nom commission"])
		debut := dateSenat(r["Début d'appartenance"])
		if mat == "" || nom == "" || debut == nil {
			continue
		}
		orgID := organes[nom]
		if orgID == 0 {
			continue
		}
		k := mat + "|" + nom + "|" + r["Début d'appartenance"]
		if vus[k] {
			continue
		}
		vus[k] = true
		lignes = append(lignes, []any{
			mat, orgID, debut, dateSenat(r["Fin d'appartenance"]),
			nulS(strings.TrimSpace(r["Fonction"])),
		})
	}

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE coms_in (matricule text, org_id bigint, debut date, fin date, fonction text)
		ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"coms_in"},
		[]string{"matricule", "org_id", "debut", "fin", "fonction"},
		pgx.CopyFromRows(lignes)); err != nil {
		return fail(fmt.Errorf("copie des appartenances : %w", err))
	}

	// La jointure passe par le matricule. Les appartenances d'un sénateur que
	// nous ne connaissons pas sont comptées, jamais rattachées de force.
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM coms_in c
		 WHERE NOT EXISTS (SELECT 1 FROM core.person_identifier i
		                   WHERE i.scheme='SENAT_MATRICULE' AND i.value=c.matricule)`).
		Scan(&sansPersonne); err != nil {
		return fail(err)
	}

	res, err := tx.Exec(ctx, `
		INSERT INTO core.affiliation
		  (person_id, organization_id, organization_kind, role, validity, declared_via)
		SELECT i.person_id, c.org_id, o.kind, c.fonction,
		       daterange(c.debut, c.fin, '[]'), 'INSTITUTION'
		  FROM coms_in c
		  JOIN core.person_identifier i
		    ON i.scheme = 'SENAT_MATRICULE' AND i.value = c.matricule
		  JOIN core.organization o ON o.id = c.org_id
		ON CONFLICT DO NOTHING`)
	if err != nil {
		return fail(fmt.Errorf("appartenances : %w", err))
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{
		"appartenances": res.RowsAffected(), "organes": len(organes),
		"sans_personne": sansPersonne}, "")
	fmt.Printf("  commissions du Sénat : %d appartenances sur %d organes (%d sans sénateur connu)\n",
		res.RowsAffected(), len(organes), sansPersonne)
	return nil
}

func slugSenat(s string) string {
	s = strings.ToLower(s)
	for from, to := range map[string]string{
		"à": "a", "â": "a", "ç": "c", "é": "e", "è": "e", "ê": "e", "ë": "e",
		"î": "i", "ï": "i", "ô": "o", "ö": "o", "ù": "u", "û": "u", "ü": "u", "'": " ",
	} {
		s = strings.ReplaceAll(s, from, to)
	}
	var b strings.Builder
	for _, c := range s {
		switch {
		case (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9'):
			b.WriteRune(c)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
