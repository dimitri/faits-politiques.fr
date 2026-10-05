package sante

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/bulkload"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RPPS (Répertoire partagé des professionnels de santé), Annuaire Santé de
// l'ANS. La clé qui relie enfin les professionnels aux établissements déjà
// chargés (ref.finess_etablissement) — voir docs/sante-donnees.md.
var SourceRPPS = archive.Source{
	Slug: "ans-rpps-annuaire-sante", Label: "ANS — Annuaire Santé, professionnels (RPPS)",
	Publisher: "Agence du Numérique en Santé (ANS)", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Agence du Numérique en Santé, Annuaire Santé (RPPS)",
	Cadence:     "quotidienne (à la source) ; ce dépôt la relit ponctuellement",
	Notes: "Une ligne par activité déclarée, pas par professionnel : un même identifiant_pp " +
		"revient sur plusieurs lignes s'il exerce sur plusieurs sites ou avec plusieurs rôles. " +
		"numero_finess_site est vide pour une large part des lignes (exercice libéral hors " +
		"structure) — le pont vers FINESS ne couvre donc jamais l'ensemble des professionnels. " +
		"Fichier plat de ~820 Mo (~2,4 millions de lignes) : lu en flux, jamais chargé entier " +
		"en mémoire, sur le modèle déjà appliqué à la délinquance communale (5,2 millions de " +
		"lignes, internal/communes/ssmsi.go).",
}

const rppsURL = "https://static.data.gouv.fr/resources/annuaire-sante-extractions-des-donnees-en-libre-acces-des-professionnels-intervenant-dans-le-systeme-de-sante-rpps/20260915-114421/ps-libreacces-personne-activite.txt"

// Position des colonnes dans l'en-tête du fichier plat, vérifiée sur un
// extrait réel plutôt que supposée depuis la documentation.
var rppsColonnes = []string{
	"Type d'identifiant PP", "Identifiant PP", "Identification nationale PP",
	"Code civilité d'exercice", "Libellé civilité d'exercice", "Code civilité", "Libellé civilité",
	"Nom d'exercice", "Prénom d'exercice", "Code profession", "Libellé profession",
	"Code catégorie professionnelle", "Libellé catégorie professionnelle",
	"Code type savoir-faire", "Libellé type savoir-faire", "Code savoir-faire", "Libellé savoir-faire",
	"Code mode exercice", "Libellé mode exercice",
	"Numéro SIRET site", "Numéro SIREN site", "Numéro FINESS site", "Numéro FINESS établissement juridique",
	"Identifiant technique de la structure", "Raison sociale site", "Enseigne commerciale site",
	"Complément destinataire (coord. structure)", "Complément point géographique (coord. structure)",
	"Numéro Voie (coord. structure)", "Indice répétition voie (coord. structure)",
	"Code type de voie (coord. structure)", "Libellé type de voie (coord. structure)",
	"Libellé Voie (coord. structure)", "Mention distribution (coord. structure)",
	"Bureau cedex (coord. structure)", "Code postal (coord. structure)", "Code commune (coord. structure)",
	"Libellé commune (coord. structure)", "Code pays (coord. structure)", "Libellé pays (coord. structure)",
	"Téléphone (coord. structure)", "Téléphone 2 (coord. structure)", "Télécopie (coord. structure)",
	"Adresse e-mail (coord. structure)", "Code Département (structure)", "Libellé Département (structure)",
	"Ancien identifiant de la structure", "Autorité d'enregistrement",
	"Code secteur d'activité", "Libellé secteur d'activité",
	"Code section tableau pharmaciens", "Libellé section tableau pharmaciens",
	"Code rôle", "Libellé rôle", "Code genre activité", "Libellé genre activité",
}

func IngestRPPS(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceRPPS)
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

	f, err := arch.Fetch(ctx, srcID, runID, rppsURL, ".txt")
	if err != nil {
		return fail(err)
	}
	fichier, err := os.Open(f.Path)
	if err != nil {
		return fail(err)
	}
	defer fichier.Close()

	sc := bufio.NewScanner(fichier)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	if !sc.Scan() {
		return fail(fmt.Errorf("fichier vide"))
	}
	entete := strings.Split(sc.Text(), "|")
	idx := map[string]int{}
	for i, c := range entete {
		idx[c] = i
	}
	for _, c := range rppsColonnes {
		if _, ok := idx[c]; !ok {
			return fail(fmt.Errorf("colonne attendue absente de l'en-tête : %q", c))
		}
	}
	champ := func(l []string, nom string) string {
		i := idx[nom]
		if i >= len(l) {
			return ""
		}
		return l[i]
	}
	ouNil := func(s string) any {
		if s == "" {
			return nil
		}
		return s
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	// MERGE plutôt que DELETE+COPY : table entière, ce connecteur en est
	// l'unique propriétaire ; l'ancien DELETE payait le prix des triggers RI
	// pour l'intégralité des 2,3 millions de lignes à chaque relecture,
	// changement ou non. Pas de clé naturelle publiée par la source (une
	// même personne porte plusieurs activités, et de vrais doublons
	// existent sur toutes les colonnes publiées) : rang fixe la position
	// d'apparition dans le fichier pour chaque identifiant_pp (migration
	// 0184), la même logique que core.declaration_item (HATVP).
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_rpps_professionnel_activite (
			identifiant_pp text, rang int, nom text, prenom text, code_civilite text,
			code_profession text, libelle_profession text, code_categorie_pro text, libelle_categorie_pro text,
			code_savoir_faire text, libelle_savoir_faire text, code_mode_exercice text, libelle_mode_exercice text,
			numero_finess_site text, code_departement text, libelle_departement text, code_commune text,
			code_role text, libelle_role text, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}

	colonnesCible := []string{
		"identifiant_pp", "rang", "nom", "prenom", "code_civilite",
		"code_profession", "libelle_profession", "code_categorie_pro", "libelle_categorie_pro",
		"code_savoir_faire", "libelle_savoir_faire", "code_mode_exercice", "libelle_mode_exercice",
		"numero_finess_site", "code_departement", "libelle_departement", "code_commune",
		"code_role", "libelle_role", "source_id",
	}
	var lot [][]any
	var totalCopie int64
	const tailleLot = 50000
	rangParPP := map[string]int{}

	vider := func() error {
		if len(lot) == 0 {
			return nil
		}
		n, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_rpps_professionnel_activite"}, colonnesCible,
			pgx.CopyFromRows(lot))
		if err != nil {
			return err
		}
		totalCopie += n
		lot = lot[:0]
		return nil
	}

	nLignes := 0
	for sc.Scan() {
		nLignes++
		l := strings.Split(sc.Text(), "|")
		idPP := champ(l, "Identifiant PP")
		if idPP == "" {
			continue
		}
		rang := rangParPP[idPP]
		rangParPP[idPP] = rang + 1
		lot = append(lot, []any{
			idPP, rang,
			ouNil(champ(l, "Nom d'exercice")), ouNil(champ(l, "Prénom d'exercice")),
			ouNil(champ(l, "Code civilité")),
			champ(l, "Code profession"), champ(l, "Libellé profession"),
			ouNil(champ(l, "Code catégorie professionnelle")), ouNil(champ(l, "Libellé catégorie professionnelle")),
			ouNil(champ(l, "Code savoir-faire")), ouNil(champ(l, "Libellé savoir-faire")),
			ouNil(champ(l, "Code mode exercice")), ouNil(champ(l, "Libellé mode exercice")),
			ouNil(champ(l, "Numéro FINESS site")),
			ouNil(champ(l, "Code Département (structure)")), ouNil(champ(l, "Libellé Département (structure)")),
			ouNil(champ(l, "Code commune (coord. structure)")),
			ouNil(champ(l, "Code rôle")), ouNil(champ(l, "Libellé rôle")),
			srcID,
		})
		if len(lot) >= tailleLot {
			if err := vider(); err != nil {
				return fail(fmt.Errorf("ligne %d : %w", nLignes, err))
			}
		}
	}
	if err := sc.Err(); err != nil {
		return fail(fmt.Errorf("lecture du fichier : %w", err))
	}
	if err := vider(); err != nil {
		return fail(fmt.Errorf("dernier lot : %w", err))
	}
	if totalCopie == 0 {
		return fail(fmt.Errorf("aucune ligne chargée"))
	}

	var total int64
	err = bulkload.SansContraintesFK(ctx, tx, "core.rpps_professionnel_activite", func() error {
		ct, err := tx.Exec(ctx, `
			MERGE INTO core.rpps_professionnel_activite AS tgt
			USING tmp_rpps_professionnel_activite AS src
			ON tgt.identifiant_pp = src.identifiant_pp AND tgt.rang = src.rang
			WHEN MATCHED AND (tgt.nom, tgt.prenom, tgt.code_civilite, tgt.code_profession, tgt.libelle_profession,
			                   tgt.code_categorie_pro, tgt.libelle_categorie_pro, tgt.code_savoir_faire,
			                   tgt.libelle_savoir_faire, tgt.code_mode_exercice, tgt.libelle_mode_exercice,
			                   tgt.numero_finess_site, tgt.code_departement, tgt.libelle_departement,
			                   tgt.code_commune, tgt.code_role, tgt.libelle_role, tgt.source_id)
			                  IS DISTINCT FROM
			                  (src.nom, src.prenom, src.code_civilite, src.code_profession, src.libelle_profession,
			                   src.code_categorie_pro, src.libelle_categorie_pro, src.code_savoir_faire,
			                   src.libelle_savoir_faire, src.code_mode_exercice, src.libelle_mode_exercice,
			                   src.numero_finess_site, src.code_departement, src.libelle_departement,
			                   src.code_commune, src.code_role, src.libelle_role, src.source_id) THEN
			    UPDATE SET nom = src.nom, prenom = src.prenom, code_civilite = src.code_civilite,
			               code_profession = src.code_profession, libelle_profession = src.libelle_profession,
			               code_categorie_pro = src.code_categorie_pro, libelle_categorie_pro = src.libelle_categorie_pro,
			               code_savoir_faire = src.code_savoir_faire, libelle_savoir_faire = src.libelle_savoir_faire,
			               code_mode_exercice = src.code_mode_exercice, libelle_mode_exercice = src.libelle_mode_exercice,
			               numero_finess_site = src.numero_finess_site, code_departement = src.code_departement,
			               libelle_departement = src.libelle_departement, code_commune = src.code_commune,
			               code_role = src.code_role, libelle_role = src.libelle_role, source_id = src.source_id
			WHEN NOT MATCHED BY TARGET THEN
			    INSERT (identifiant_pp, rang, nom, prenom, code_civilite, code_profession, libelle_profession,
			            code_categorie_pro, libelle_categorie_pro, code_savoir_faire, libelle_savoir_faire,
			            code_mode_exercice, libelle_mode_exercice, numero_finess_site, code_departement,
			            libelle_departement, code_commune, code_role, libelle_role, source_id)
			    VALUES (src.identifiant_pp, src.rang, src.nom, src.prenom, src.code_civilite, src.code_profession,
			            src.libelle_profession, src.code_categorie_pro, src.libelle_categorie_pro,
			            src.code_savoir_faire, src.libelle_savoir_faire, src.code_mode_exercice,
			            src.libelle_mode_exercice, src.numero_finess_site, src.code_departement,
			            src.libelle_departement, src.code_commune, src.code_role, src.libelle_role, src.source_id)
			WHEN NOT MATCHED BY SOURCE THEN DELETE`)
		if err != nil {
			return err
		}
		total = ct.RowsAffected()
		return nil
	})
	if err != nil {
		return fail(fmt.Errorf("fusion : %w", err))
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": totalCopie, "touchees": total}, "")
	fmt.Printf("  RPPS, professionnels de santé (ANS) : %d lignes (%d touchées par la fusion)\n", totalCopie, total)
	return nil
}
