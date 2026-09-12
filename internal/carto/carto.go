// Package carto charge la cartographie éditoriale : les décisions qui relient
// un parti à son groupe parlementaire et aux référentiels tiers qui le nomment
// autrement.
//
// Ces liens ne sont publiés par personne. Le registre CNCCFP ne dit pas quel
// groupe un parti compose ; CHES ne cite aucun identifiant français. Ce sont
// donc des décisions, prises dans data/organisations.csv, et elles vivent en
// base dans une révision de cartographie versionnée et gelable — jamais en dur
// dans le code de rendu.
//
// Avant ce chargement, core.party_group_link était vide et aucune requête ne
// pouvait joindre un score CHES à un vote (D-020).
package carto

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Version de la méthode de cartographie. À incrémenter dès que la façon de
// résoudre un rattachement change, pas quand le contenu du CSV change.
const MethodVersion = "carto-v1"

// LineageSlug : la cartographie de référence, celle que le site publie. Les
// cartographies alternatives des lecteurs sont des forks de celle-ci.
const LineageSlug = "reference"

type ligne struct {
	Slug, Libelle string
	CodeCNCCFP    string
	CHESNom       string
	PopuListNom   string
	GroupeANUID   string
	Justification string
}

// Ingest relit le CSV éditorial et reconstruit intégralement les liens de la
// révision courante. Reconstruction et non complétion : un rattachement retiré
// du CSV doit disparaître de la base, sinon la base garderait une décision que
// personne n'assume plus.
func Ingest(ctx context.Context, pool *pgxpool.Pool, csvPath string) error {
	lignes, err := lire(csvPath)
	if err != nil {
		return err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	revID, err := revisionCourante(ctx, tx)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(ctx,
		`DELETE FROM core.party_group_link WHERE mapping_revision_id = $1`, revID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM core.party_referential_link WHERE mapping_revision_id = $1`, revID); err != nil {
		return err
	}

	var nGroupes, nRef int
	var ignores []string

	for _, l := range lignes {
		if l.CodeCNCCFP == "" {
			// Sans entrée au registre, pas de parti canonique : on ne fabrique
			// pas une identité que l'État ne reconnaît pas.
			if l.CHESNom != "" || l.GroupeANUID != "" {
				ignores = append(ignores, l.Slug+" (aucun code CNCCFP)")
			}
			continue
		}
		partyID, err := parIdentifiant(ctx, tx, "CNCCFP", l.CodeCNCCFP)
		if err != nil {
			return fmt.Errorf("%s : parti CNCCFP %s : %w", l.Slug, l.CodeCNCCFP, err)
		}

		if l.GroupeANUID != "" {
			groupID, err := parIdentifiant(ctx, tx, "AN_ORGANE", l.GroupeANUID)
			if err != nil {
				return fmt.Errorf("%s : groupe %s : %w", l.Slug, l.GroupeANUID, err)
			}
			// La validité du lien est celle du groupe lui-même : un groupe de
			// la 17e législature n'existe pas avant sa constitution.
			if _, err := tx.Exec(ctx, `
				INSERT INTO core.party_group_link
				  (mapping_revision_id, party_id, group_id, group_kind,
				   relation, validity, rationale_code)
				SELECT $1, $2, o.id, o.kind, 'COMPOSANTE', o.validity, $4
				  FROM core.organization o WHERE o.id = $3`,
				revID, partyID, groupID, motif(l)); err != nil {
				return fmt.Errorf("%s : lien groupe : %w", l.Slug, err)
			}
			nGroupes++
		}

		// Le schéma nomme le référentiel dans core.party_referential_link ;
		// le fournisseur est celui déclaré par ref.classification_set.
		for _, ref := range []struct{ scheme, fournisseur, nom string }{
			{"CHES", "CHES", l.CHESNom},
			{"POPULIST", "PopuList", l.PopuListNom},
		} {
			if ref.nom == "" {
				continue
			}
			// Résolution par le NOM, parce que c'est exactement ce que le CSV
			// affirme : « dans ce référentiel, ce parti s'appelle ainsi ». Le
			// nom n'est pas deviné, il est transcrit ; toute ambiguïté est une
			// erreur du CSV et doit faire échouer le chargement.
			refID, err := parNomDeReferentiel(ctx, tx, ref.fournisseur, ref.nom)
			if err != nil {
				return fmt.Errorf("%s : %s %q : %w", l.Slug, ref.scheme, ref.nom, err)
			}
			if refID == partyID {
				continue
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO core.party_referential_link
				  (mapping_revision_id, party_id, referential_id, scheme, rationale_code)
				VALUES ($1, $2, $3, $4, $5)`,
				revID, partyID, refID, ref.scheme, motif(l)); err != nil {
				return fmt.Errorf("%s : lien %s : %w", l.Slug, ref.scheme, err)
			}
			nRef++
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	fmt.Printf("  révision %d : %d liens parti->groupe, %d liens parti->référentiel\n",
		revID, nGroupes, nRef)
	for _, s := range ignores {
		fmt.Printf("  ignoré : %s\n", s)
	}
	return nil
}

// motif traduit la justification éditoriale en code de motif. Le texte libre du
// CSV reste la justification lisible ; le code sert aux requêtes.
func motif(l ligne) string {
	j := strings.ToLower(l.Justification)
	switch {
	case strings.Contains(j, "identique"):
		return "DENOMINATION_IDENTIQUE"
	case strings.Contains(j, "sigle"):
		return "SIGLE_OFFICIEL"
	case strings.Contains(j, "succession") || strings.Contains(j, "successeur"):
		return "SUCCESSION_DOCUMENTEE"
	case strings.Contains(j, "revendiqu"):
		return "REVENDIQUE_PAR_PARTI"
	default:
		return "USAGE_CONSTANT"
	}
}

// revisionCourante renvoie la tête non gelée de la cartographie de référence,
// en la créant au besoin. Une seule tête peut exister à la fois — c'est un
// index unique partiel qui le garantit, pas cette fonction.
func revisionCourante(ctx context.Context, tx pgx.Tx) (int64, error) {
	var lineageID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO core.mapping_lineage (slug, label, kind, listed)
		VALUES ($1, 'Cartographie de référence', 'REFERENCE', true)
		ON CONFLICT (slug) DO UPDATE SET slug = EXCLUDED.slug
		RETURNING id`, LineageSlug).Scan(&lineageID); err != nil {
		return 0, fmt.Errorf("lignée de cartographie : %w", err)
	}

	var revID int64
	err := tx.QueryRow(ctx, `
		SELECT id FROM core.mapping_revision
		 WHERE lineage_id = $1 AND frozen_at IS NULL`, lineageID).Scan(&revID)
	if err == nil {
		return revID, nil
	}
	if err != pgx.ErrNoRows {
		return 0, err
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO core.mapping_revision (lineage_id, revision, note)
		SELECT $1, coalesce(max(revision), 0) + 1, $2
		  FROM core.mapping_revision WHERE lineage_id = $1
		RETURNING id`, lineageID,
		"Rattachements déclarés dans data/organisations.csv ("+MethodVersion+")").Scan(&revID); err != nil {
		return 0, fmt.Errorf("révision de cartographie : %w", err)
	}
	return revID, nil
}

func parIdentifiant(ctx context.Context, tx pgx.Tx, scheme, value string) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `
		SELECT organization_id FROM core.organization_identifier
		 WHERE scheme = $1 AND value = $2`, scheme, value).Scan(&id)
	if err == pgx.ErrNoRows {
		return 0, fmt.Errorf("aucune organisation ne porte cet identifiant")
	}
	return id, err
}

// parNomDeReferentiel retrouve l'organisation créée par le connecteur d'un
// référentiel tiers. Elle se reconnaît à son nom ET au fait qu'elle porte une
// classification issue de ce référentiel — pas à un identifiant, car tous les
// référentiels n'en publient pas : PopuList désigne ses partis par le code
// Party Facts, parfois par rien du tout. Sans cette seconde condition, un
// parti français homonyme pourrait être retenu à la place de l'entrée du
// référentiel.
func parNomDeReferentiel(ctx context.Context, tx pgx.Tx, fournisseur, nom string) (int64, error) {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT o.id FROM core.organization o
		  JOIN core.party_classification c ON c.party_id = o.id
		  JOIN ref.classification_set s ON s.id = c.classification_set_id
		 WHERE s.provider = $1 AND o.name = $2`, fournisseur, nom)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	switch len(ids) {
	case 1:
		return ids[0], nil
	case 0:
		return 0, fmt.Errorf("aucune entrée de ce nom dans le référentiel")
	default:
		return 0, fmt.Errorf("%d entrées de ce nom : le CSV doit lever l'ambiguïté", len(ids))
	}
}

func lire(path string) ([]ligne, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comment = '#'
	r.FieldsPerRecord = -1
	recs, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(recs) < 2 {
		return nil, fmt.Errorf("%s : aucune organisation", path)
	}
	idx := map[string]int{}
	for i, h := range recs[0] {
		idx[strings.TrimSpace(h)] = i
	}
	get := func(rec []string, k string) string {
		if i, ok := idx[k]; ok && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}
	var out []ligne
	for _, rec := range recs[1:] {
		l := ligne{
			Slug: get(rec, "slug"), Libelle: get(rec, "libelle"),
			CodeCNCCFP: get(rec, "code_cnccfp"), CHESNom: get(rec, "ches_nom"),
			PopuListNom: get(rec, "populist_nom"), GroupeANUID: get(rec, "groupe_an_uid"),
			Justification: get(rec, "justification"),
		}
		if l.Slug == "" {
			continue
		}
		out = append(out, l)
	}
	return out, nil
}
