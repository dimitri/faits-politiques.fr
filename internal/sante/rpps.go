package sante

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
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
	if _, err := tx.Exec(ctx, `DELETE FROM core.rpps_professionnel_activite`); err != nil {
		return fail(err)
	}

	colonnesCible := []string{
		"identifiant_pp", "nom", "prenom", "code_civilite",
		"code_profession", "libelle_profession", "code_categorie_pro", "libelle_categorie_pro",
		"code_savoir_faire", "libelle_savoir_faire", "code_mode_exercice", "libelle_mode_exercice",
		"numero_finess_site", "code_departement", "libelle_departement", "code_commune",
		"code_role", "libelle_role", "source_id",
	}
	var lot [][]any
	var total int64
	const tailleLot = 50000

	vider := func() error {
		if len(lot) == 0 {
			return nil
		}
		n, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "rpps_professionnel_activite"}, colonnesCible,
			pgx.CopyFromRows(lot))
		if err != nil {
			return err
		}
		total += n
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
		lot = append(lot, []any{
			idPP,
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
	if total == 0 {
		return fail(fmt.Errorf("aucune ligne chargée"))
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": total}, "")
	fmt.Printf("  RPPS, professionnels de santé (ANS) : %d lignes\n", total)
	return nil
}
