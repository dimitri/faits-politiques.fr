// Package decp charge les Données essentielles de la commande publique
// (DECP) consolidées — qui a vendu quoi à quel acheteur public, pour combien.
// Voir docs/perimetre.md § 4.4 et le commentaire de core.public_contract.
package decp

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parquet-go/parquet-go"
)

const ConnectorVersion = "decp-v1"

// Le format Parquet permet de ne lire que les colonnes utiles sans
// décompresser les 60 colonnes du fichier — l'alternative CSV (2,5 Go,
// docs/perimetre.md § 4.4) demanderait de tout décompresser pour n'en garder
// qu'une fraction. C'est précisément le problème que le Parquet résout ici.
var SourceDECP = archive.Source{
	Slug: "decp-consolidees", Label: "DECP consolidées — commande publique française",
	Publisher: "Projet decp-processing (Colin Maudry), via data.gouv.fr",
	Tier:      "PRIMARY_OFFICIAL",
	Licence:   "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : DECP consolidées (decp-processing), data.gouv.fr",
	Cadence:     "quotidienne (à la source) ; ce dépôt la relit ponctuellement",
	Notes: "Format Parquet (235 Mo) plutôt que CSV (2,5 Go) — même contenu, colonnes lues " +
		"sélectivement. Une ligne par (marché, titulaire) sur l'état ACTUEL du marché " +
		"(donneesActuelles=true) : les versions antérieures à un avenant ne sont pas " +
		"conservées. Environ 0,3 % des lignes retenues sont des doublons exacts dans la " +
		"source elle-même (déjà documentés par le producteur, resource " +
		"statistiques-doublons-sources.parquet) — dédoublonnés à l'ingestion.",
}

const decpURL = "https://static.data.gouv.fr/resources/donnees-essentielles-de-la-commande-publique-consolidees-format-tabulaire/20260915-051620/decp.parquet"

// decpLigne : seules les colonnes utiles à core.public_contract, sur les 60
// que porte le fichier — parquet-go ne lit que celles déclarées ici.
type decpLigne struct {
	UID                *string  `parquet:"uid,optional"`
	AcheteurID         *string  `parquet:"acheteur_id,optional"`
	AcheteurCommune    *string  `parquet:"acheteur_commune_code,optional"`
	AcheteurRegion     *string  `parquet:"acheteur_region_code,optional"`
	TitulaireID        *string  `parquet:"titulaire_id,optional"`
	TitulaireNom       *string  `parquet:"titulaire_nom,optional"`
	TitulaireRegion    *string  `parquet:"titulaire_region_code,optional"`
	Objet              *string  `parquet:"objet,optional"`
	Type               *string  `parquet:"type,optional"`
	MontantRationalise *float64 `parquet:"montant_rationalise,optional"`
	MontantAnomalie    *string  `parquet:"montant_anomalie,optional"`
	CodeCPV            *string  `parquet:"codeCPV,optional"`
	DureeMois          *int16   `parquet:"dureeMois,optional"`
	DateNotification   *int32   `parquet:"dateNotification,optional"` // jours depuis 1970-01-01 (DATE Parquet)
	ModificationID     *int16   `parquet:"modification_id,optional"`
	DonneesActuelles   *bool    `parquet:"donneesActuelles,optional"`
}

var epoque = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)

func IngestDECP(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceDECP)
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

	f, err := arch.Fetch(ctx, srcID, runID, decpURL, ".parquet")
	if err != nil {
		return fail(err)
	}
	fichier, err := os.Open(f.Path)
	if err != nil {
		return fail(err)
	}
	defer fichier.Close()

	r := parquet.NewGenericReader[decpLigne](fichier)
	defer r.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SET LOCAL work_mem = '256MB'`); err != nil {
		return fail(err)
	}
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE decp_in (
		  source_uid text, acheteur_siret text, commune_code text, acheteur_region_code text,
		  titulaire_siret text, titulaire_nom text, titulaire_region_code text,
		  objet text, marche_type text, montant numeric, montant_anomalie text,
		  cpv text, duree_mois integer, date_notification date, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}

	colonnes := []string{"source_uid", "acheteur_siret", "commune_code", "acheteur_region_code",
		"titulaire_siret", "titulaire_nom", "titulaire_region_code",
		"objet", "marche_type", "montant", "montant_anomalie",
		"cpv", "duree_mois", "date_notification", "source_id"}

	const tailleLot = 20000
	buf := make([]decpLigne, tailleLot)
	var lot [][]any
	var lu, retenues int64

	vider := func() error {
		if len(lot) == 0 {
			return nil
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"decp_in"}, colonnes, pgx.CopyFromRows(lot)); err != nil {
			return err
		}
		lot = lot[:0]
		return nil
	}

	for {
		n, err := r.Read(buf)
		for i := 0; i < n; i++ {
			lu++
			l := buf[i]
			if l.DonneesActuelles == nil || !*l.DonneesActuelles || l.UID == nil {
				continue
			}
			retenues++
			var titulaireID string
			if l.TitulaireID != nil {
				titulaireID = *l.TitulaireID
			}
			var modID string
			if l.ModificationID != nil {
				modID = fmt.Sprintf("%d", *l.ModificationID)
			}
			sourceUID := fmt.Sprintf("%s#%s#%s", *l.UID, titulaireID, modID)

			var dateNotif *time.Time
			if l.DateNotification != nil {
				d := epoque.AddDate(0, 0, int(*l.DateNotification))
				dateNotif = &d
			}
			lot = append(lot, []any{
				sourceUID, l.AcheteurID, l.AcheteurCommune, l.AcheteurRegion,
				l.TitulaireID, l.TitulaireNom, l.TitulaireRegion,
				l.Objet, l.Type, l.MontantRationalise, l.MontantAnomalie,
				l.CodeCPV, dureeInt(l.DureeMois), dateNotif, srcID,
			})
			if len(lot) >= tailleLot {
				if err := vider(); err != nil {
					return fail(fmt.Errorf("ligne %d : %w", lu, err))
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fail(fmt.Errorf("lecture Parquet à la ligne %d : %w", lu, err))
		}
	}
	if err := vider(); err != nil {
		return fail(fmt.Errorf("dernier lot : %w", err))
	}
	if retenues == 0 {
		return fail(fmt.Errorf("aucune ligne retenue (donneesActuelles=true)"))
	}

	// Dédoublonnage : ~0,3 % des lignes retenues partagent un source_uid
	// (marché, titulaire et modification identiques) — des doublons exacts
	// déjà documentés par le producteur lui-même, pas des marchés distincts.
	// DISTINCT ON garde une ligne arbitraire par source_uid.
	//
	// MERGE plutôt que DELETE (la table entière, ce connecteur en est
	// l'unique propriétaire) + INSERT : core.public_contract porte 2,1
	// millions de lignes, la plus grosse table de ce projet — un
	// DELETE+INSERT y paierait le prix des triggers RI pour la table
	// ENTIÈRE à chaque republication du jeu DECP, changement ou non. Sans
	// compter un vrai bug que ça évite : core.evidence.public_contract_id
	// est ON DELETE CASCADE, donc l'ancien DELETE détruisait TOUTE preuve
	// rattachée à un marché public à chaque ré-ingestion, même pour des
	// marchés inchangés d'un run à l'autre — un MERGE ne touche que ce qui
	// a vraiment changé, donc ne cascade plus que sur les marchés
	// effectivement disparus du jeu de données.
	res, err := tx.Exec(ctx, `
		WITH dedup AS (
			SELECT DISTINCT ON (source_uid)
			  source_uid, acheteur_siret, commune_code, acheteur_region_code,
			  titulaire_siret, titulaire_nom, titulaire_region_code,
			  objet, marche_type, montant, montant_anomalie,
			  cpv, duree_mois, date_notification, source_id
			FROM decp_in
			ORDER BY source_uid
		)
		MERGE INTO core.public_contract AS tgt
		USING dedup AS src
		ON tgt.source_uid = src.source_uid
		WHEN MATCHED AND (tgt.acheteur_siret, tgt.commune_code, tgt.acheteur_region_code,
		                   tgt.titulaire_siret, tgt.titulaire_nom, tgt.titulaire_region_code,
		                   tgt.objet, tgt.marche_type, tgt.montant, tgt.montant_anomalie,
		                   tgt.cpv, tgt.duree_mois, tgt.date_notification, tgt.source_id)
		                  IS DISTINCT FROM
		                  (src.acheteur_siret, src.commune_code, src.acheteur_region_code,
		                   src.titulaire_siret, src.titulaire_nom, src.titulaire_region_code,
		                   src.objet, src.marche_type, src.montant, src.montant_anomalie,
		                   src.cpv, src.duree_mois, src.date_notification, src.source_id) THEN
		    UPDATE SET
		      acheteur_siret = src.acheteur_siret, commune_code = src.commune_code,
		      acheteur_region_code = src.acheteur_region_code, titulaire_siret = src.titulaire_siret,
		      titulaire_nom = src.titulaire_nom, titulaire_region_code = src.titulaire_region_code,
		      objet = src.objet, marche_type = src.marche_type, montant = src.montant,
		      montant_anomalie = src.montant_anomalie, cpv = src.cpv, duree_mois = src.duree_mois,
		      date_notification = src.date_notification, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (source_uid, acheteur_siret, commune_code, acheteur_region_code,
		            titulaire_siret, titulaire_nom, titulaire_region_code,
		            objet, marche_type, montant, montant_anomalie,
		            cpv, duree_mois, date_notification, source_id)
		    VALUES (src.source_uid, src.acheteur_siret, src.commune_code, src.acheteur_region_code,
		            src.titulaire_siret, src.titulaire_nom, src.titulaire_region_code,
		            src.objet, src.marche_type, src.montant, src.montant_anomalie,
		            src.cpv, src.duree_mois, src.date_notification, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion core.public_contract : %w", err))
	}
	inseres := res.RowsAffected()

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lues": lu, "retenues": retenues, "inserees": inseres}, "")
	fmt.Printf("  DECP : %d lignes lues, %d retenues (état actuel), %d lignes touchées par la fusion\n", lu, retenues, inseres)
	return nil
}

func dureeInt(v *int16) *int {
	if v == nil {
		return nil
	}
	n := int(*v)
	return &n
}

func Ingest(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	return IngestDECP(ctx, pool, arch)
}
