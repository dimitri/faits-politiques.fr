package jorf

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/jackc/pgx/v5/pgxpool"
)

// La base complète du Journal officiel, et pourquoi il a fallu y descendre.
//
// La composition des gouvernements de la Ve République est publiée en données
// ouvertes par les services du Premier ministre — et s'ARRÊTE EN 2014. Le jeu
// data.gouv.fr n'a pas été mis à jour depuis le 18 juin 2014, et rien ne l'a
// remplacé : une recherche sur le catalogue ne rend que ce jeu et son équivalent
// pour la IVe République.
//
// Les autres pistes, toutes essayées :
//
//   - Légifrance répond HTTP 403 derrière une protection anti-robot ; son API
//     exige un compte PISTE. Écartée.
//   - L'annuaire de service-public.fr publie les ministères, pas l'HISTORIQUE
//     des ministres : c'est un instantané, il ne remonte à rien.
//   - L'open data de l'Assemblée donne des mandats ministériels depuis 2007,
//     mais seulement pour les ministres qui furent députés, et il y mêle les
//     « parlementaires en mission », qui ne sont pas membres du Gouvernement.
//
// Reste la source de droit : le DÉCRET relatif à la composition du Gouvernement,
// publié au Journal officiel. La DILA le diffuse en open data, dans le même
// format que les incréments quotidiens que ce connecteur lit déjà — mais dans la
// base complète, qui pèse 1,1 Go.
//
// Ce chargement-ci ne retient donc QUE les décrets de composition et de
// nomination du Premier ministre. Il traverse le gigaoctet en flux et n'écrit
// qu'une trentaine d'actes : charger la totalité du Journal officiel pour
// répondre à cette question serait disproportionné, et c'est une décision
// séparée.
const baseGlobale = "https://echanges.dila.gouv.fr/OPENDATA/JORFSIMPLE/" +
	"Freemium_jorf_simple_20250713-140000.tar.gz"

// Les titres qui portent la composition du Gouvernement.
//
// Trois formes coexistent depuis 1959 :
//
//	« Décret du 12 octobre 2025 relatif à la composition du Gouvernement »
//	« Décret du 10 octobre 2025 portant nomination du Premier ministre »
//	« Décret du 6 septembre 2024 portant cessation de fonctions du Gouvernement »
//
// Le filtre est volontairement plus large que « composition » : un remaniement
// partiel s'intitule aussi « relatif à la composition », et la nomination du
// Premier ministre fait l'objet d'un décret distinct, publié le même jour ou la
// veille.
var reTitreGouvernement = regexp.MustCompile(
	`(?i)(composition du gouvernement|nomination du premier ministre|` +
		`cessation des fonctions du gouvernement|cessation de fonctions du gouvernement|` +
		`fin des fonctions du gouvernement)`)

// IngestGouvernement traverse la base complète du Journal officiel et n'en
// retient que les décrets de composition du Gouvernement.
//
// Le fichier n'est téléchargé qu'une fois : archive.Fetch le scelle par son
// empreinte, et une seconde exécution le relit depuis le disque.
func IngestGouvernement(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, Source)
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

	fmt.Println("  téléchargement de la base complète du Journal officiel (1,1 Go)…")
	f, err := arch.Fetch(ctx, srcID, runID, baseGlobale, ".tar.gz")
	if err != nil {
		return fail(err)
	}

	var b bilan
	if err := parcourirGlobale(ctx, pool, f.Path, srcID, &b); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{
		"fichiers": b.fichiers, "decodes": b.decodes, "echecs": b.echecs,
		"retenus": b.actes}, "")
	logs.Notice(fmt.Sprintf("full JORF: %s, %d decoded (%d failed), %d government decrees kept",
		logs.Plural(b.fichiers, "file"), b.decodes, b.echecs, b.actes))
	return nil
}

// parcourirGlobale lit l'archive EN FLUX. Elle contient plusieurs centaines de
// milliers de fichiers ; les déplier sur disque coûterait une dizaine de
// gigaoctets pour n'en garder qu'une trentaine.
//
// Le filtre porte sur le TITRE, décodé pour chaque fichier, et non sur le chemin :
// rien dans l'arborescence ne distingue un décret de composition d'un arrêté de
// nomination dans un corps d'État.
func parcourirGlobale(ctx context.Context, pool *pgxpool.Pool, chemin string, srcID int64, b *bilan) error {
	fh, err := os.Open(chemin)
	if err != nil {
		return err
	}
	defer fh.Close()
	gz, err := gzip.NewReader(fh)
	if err != nil {
		return err
	}
	defer gz.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if h.Typeflag != tar.TypeReg || !strings.Contains(h.Name, "JORFTEXT") ||
			!strings.HasSuffix(h.Name, ".xml") {
			continue
		}
		raw, err := io.ReadAll(tr)
		if err != nil {
			return fmt.Errorf("lecture de %s : %w", h.Name, err)
		}
		b.fichiers++
		t, err := decoder(raw)
		if err != nil {
			b.echecs++
			continue
		}
		if t.ID == "" {
			b.sansID++
			continue
		}
		b.decodes++

		if !reTitreGouvernement.MatchString(t.TitreFull) && !reTitreGouvernement.MatchString(t.Titre) {
			continue
		}
		contenu := corps(raw)
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.acte_jo
			  (id, nature, numero, nor, date_publi, date_texte, titre, titre_complet,
			   ministere, contenu, nominatif, source_id)
			VALUES ($1,$2,$3,$4,$5::date,$6::date,$7,$8,$9,$10,true,$11)
			ON CONFLICT (id) DO UPDATE SET
			  titre_complet = EXCLUDED.titre_complet, contenu = EXCLUDED.contenu,
			  -- Les dates entrent dans la mise à jour : une sentinelle 2999-01-01
			  -- déjà chargée doit pouvoir être corrigée par un rechargement.
			  date_publi = EXCLUDED.date_publi, date_texte = EXCLUDED.date_texte,
			  nominatif = true, charge_le = now()`,
			t.ID, nul(t.Nature), nul(t.Num), nul(t.NOR), dateJO(t.DatePubli), dateJO(t.DateTexte),
			nul(t.Titre), nul(t.TitreFull), nul(t.Ministere), nul(contenu), srcID); err != nil {
			return fmt.Errorf("acte %s : %w", t.ID, err)
		}
		b.actes++
		if b.actes%10 == 0 {
			logs.Notice(fmt.Sprintf("%d decrees kept out of %d files read", b.actes, b.fichiers))
		}
	}
	return tx.Commit(ctx)
}
