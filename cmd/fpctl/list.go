package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/ingest"
	"github.com/faits-politiques/faits-politiques/internal/pipeline"
	"github.com/faits-politiques/faits-politiques/internal/sources"
	"github.com/faits-politiques/faits-politiques/internal/stats"
	"github.com/faits-politiques/faits-politiques/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
)

// commandeList : fpctl list sources, fpctl list connectors, fpctl list
// stats, fpctl list deps — quatre vues différentes sur la même question,
// « qu'est-ce qui est chargé et par quoi » : les sources déclarées
// (raw.source), le code qui les charge (les fonctions Ingest* d'internal/),
// ce que ça pèse une fois en base, et — pour le seul socle parlementaire
// audité (voir fpctl-ingest(1), LE SOCLE PARLEMENTAIRE) — dans quel ordre.
func commandeList() *cobra.Command {
	cmd := &cobra.Command{Use: "list", Short: "Liste une collection"}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "sources [options]",
			Short: "Catalogue des sources de données ingérées",
			Long: "Écrit le catalogue JSON des sources (raw.source), avec leur dernière\n" +
				"exécution d'ingestion (raw.fetch_run) — licence, éditeur, cadence,\n" +
				"date de la dernière collecte réussie. Par défaut dans\n" +
				"docs/catalogue-sources.json (voir -out).",
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				if estDemandeAide(args) {
					return afficherManuel("fpctl-list")
				}
				return executerInterne(cmd.Context(), sources.Run(cmd.Context(), args))
			},
		},
		&cobra.Command{
			Use:   "connectors",
			Short: "Liste les fonctions Ingest* trouvées dans internal/",
			Long: "Le type de connecteur n'est pas déclaré dans un registre à part :\n" +
				"cette commande cherche directement, dans internal/, toute fonction\n" +
				"exportée dont le nom commence par Ingest, et les groupe par paquet.",
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				if estDemandeAide(args) {
					return afficherManuel("fpctl-list")
				}
				return executerInterne(cmd.Context(), listerConnecteurs())
			},
		},
		&cobra.Command{
			Use:   "stats",
			Short: "Résumé du contenu de la base : tables, lignes, taille par schéma",
			Long: "Lit pg_stat_user_tables, groupé par schéma applicatif (core, ref,\n" +
				"geo, raw, derived...) : nombre de tables, lignes estimées (n_live_tup,\n" +
				"pas un COUNT(*) exact), taille sur disque. Puis les dix tables les\n" +
				"plus lourdes, tous schémas confondus.",
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				if estDemandeAide(args) {
					return afficherManuel("fpctl-list")
				}
				return executerInterne(cmd.Context(), afficherStats(cmd.Context()))
			},
		},
		&cobra.Command{
			Use:   "deps [nom]",
			Short: "Graphe de dépendances du socle parlementaire, tel que publié en base",
			Long: "Le socle parlementaire (download, partis, normalize, carto, senat,\n" +
				"europe, themes) est la seule partie du catalogue d'ingestion dont\n" +
				"les dépendances sont déclarées et vérifiées — voir internal/pipeline\n" +
				"et fpctl-ingest(1). Republié à chaque exécution touchant ce socle\n" +
				"(fpctl ingest parlement ... ou l'une de ces sept sources) ; vide\n" +
				"avant la première.\n\n" +
				"Avec un nom, limite l'affichage à une seule chose : l'une des sept\n" +
				"étapes (avec la chaîne complète de ce dont elle dépend,\n" +
				"transitivement), ou une section de fpctl build (scrutin, communes,\n" +
				"reste — ses préalables d'ingestion, chacun développé à son tour s'il\n" +
				"appartient lui-même au socle).",
			Args: cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				var nom string
				if len(args) > 0 {
					nom = args[0]
				}
				return executerInterne(cmd.Context(), afficherDeps(cmd.Context(), nom))
			},
		},
	)
	return cmd
}

// composantesEtape : ce que chaque étape du socle parlementaire remplit,
// pour afficher combien ça pèse à côté du graphe — tenu à la main, comme
// ingestPrealables (cmd/fpctl/build.go) : ces sept étapes sont assez rares
// et assez stables pour que ça reste à jour sans registre séparé.
//
// SlugsSources : les documents archivés (raw.document, via raw.retrieval)
// que cette étape télécharge — vide pour une étape qui n'en télécharge
// aucun. Tables : ce qu'elle écrit dans core/derived, mesuré par
// pg_total_relation_size — une table peut apparaître sous plusieurs étapes
// (carto.IngestPresidents écrit dans core.person/core.mandate, que
// normalize remplit aussi) : la taille affichée est alors celle de TOUTE
// la table, pas la part de cette seule étape — une approximation
// assumée pour un résumé de terminal, pas une comptabilité exacte.
var composantesEtape = map[string]struct {
	SlugsSources []string
	Tables       []string
}{
	"download":  {SlugsSources: []string{"an-amo", "an-amo-15", "an-amo-16", "an-dossiers", "an-scrutins"}},
	"partis":    {SlugsSources: []string{"ches-2024", "cnccfp-comptes", "populist-v4"}},
	"senat":     {SlugsSources: []string{"senat-dosleg", "senat-senateurs"}},
	"europe":    {SlugsSources: []string{"howtheyvote"}},
	"normalize": {Tables: []string{"core.person", "core.organization", "core.mandate", "core.affiliation", "core.scrutin", "core.ballot", "core.dossier", "core.texte"}},
	"carto":     {Tables: []string{"core.party_group_link", "core.party_referential_link", "core.mapping_lineage", "core.mapping_revision", "core.gouvernement"}},
	"themes":    {Tables: []string{"derived.scrutin_topic", "derived.coverage"}},
}

// tailleEtape additionne les octets archivés (SlugsSources) et occupés en
// base (Tables) d'une étape — 0, sans erreur, pour une étape non déclarée
// dans composantesEtape (rien à afficher plutôt qu'un blocage).
func tailleEtape(ctx context.Context, pool *pgxpool.Pool, nom string) (archive, base int64, err error) {
	c, ok := composantesEtape[nom]
	if !ok {
		return 0, 0, nil
	}
	if len(c.SlugsSources) > 0 {
		if err := pool.QueryRow(ctx, `
			SELECT coalesce(sum(d.byte_size), 0) FROM raw.document d WHERE d.id IN (
				SELECT DISTINCT r.document_id FROM raw.retrieval r JOIN raw.source s ON s.id = r.source_id
				WHERE s.slug = ANY($1) AND r.document_id IS NOT NULL)`,
			c.SlugsSources).Scan(&archive); err != nil {
			return 0, 0, err
		}
	}
	for _, t := range c.Tables {
		// to_regclass plutôt que caster directement en regclass : une table
		// qui n'existe pas encore (base pas à jour) ne doit pas faire
		// échouer tout l'affichage du graphe, juste compter pour 0 ici.
		var o *int64
		if err := pool.QueryRow(ctx, `SELECT pg_total_relation_size(to_regclass($1))`, t).Scan(&o); err != nil {
			return 0, 0, fmt.Errorf("taille de %s : %w", t, err)
		}
		if o != nil {
			base += *o
		}
	}
	return archive, base, nil
}

func afficherDeps(ctx context.Context, nom string) error {
	pool, err := store.Open(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	// core.pipeline_etape (migration récente) peut manquer sur une base pas
	// encore migrée par cette version de fpctl — un « list » reste
	// délibérément en lecture seule, il n'applique jamais de migration lui-
	// même (contrairement à fpctl ingest, qui le fait toujours en premier) :
	// dire clairement quoi lancer plutôt que remonter l'erreur SQL brute.
	var existe bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('core.pipeline_etape') IS NOT NULL`).Scan(&existe); err != nil {
		return fmt.Errorf("vérification du schéma : %w", err)
	}
	if !existe {
		return fmt.Errorf("core.pipeline_etape n'existe pas encore sur cette base — lancez d'abord « fpctl ingest migrate »")
	}

	etapes, err := pipeline.LireTopologie(ctx, pool)
	if err != nil {
		return err
	}
	if len(etapes) == 0 {
		fmt.Println("rien à afficher — lancez « fpctl ingest parlement all » (ou l'une de ses sources) au moins une fois")
		return nil
	}
	parNom := map[string]pipeline.EtapePubliee{}
	for _, e := range etapes {
		parNom[e.Nom] = e
	}

	if nom == "" {
		for _, e := range etapes {
			if err := imprimerEtape(ctx, pool, e, ""); err != nil {
				return err
			}
		}
		return nil
	}

	if e, ok := parNom[nom]; ok {
		if err := imprimerEtape(ctx, pool, e, ""); err != nil {
			return err
		}
		vus := map[string]bool{nom: true}
		return imprimerTransitivement(ctx, pool, parNom, e.DependDe, vus, "  ")
	}

	if prealables, ok := ingestPrealables[nom]; ok {
		fmt.Printf("fpctl build %s ingère d'abord :\n", nom)
		vus := map[string]bool{}
		for _, p := range prealables {
			if e, ok := parNom[p]; ok {
				if err := imprimerEtape(ctx, pool, e, "  "); err != nil {
					return err
				}
				vus[p] = true
				if err := imprimerTransitivement(ctx, pool, parNom, e.DependDe, vus, "    "); err != nil {
					return err
				}
				continue
			}
			description := p
			if s, ok := ingest.SourceParNom(p); ok {
				description = s.Description
			}
			fmt.Printf("  %s — %s\n", p, description)
		}
		return nil
	}

	var connus []string
	for _, e := range etapes {
		connus = append(connus, e.Nom)
	}
	for s := range ingestPrealables {
		connus = append(connus, s)
	}
	sort.Strings(connus)
	return fmt.Errorf("%s inconnu (attendu : %s)", nom, strings.Join(connus, ", "))
}

// imprimerTransitivement développe, dans l'ordre, ce dont dépendent (encore)
// les noms donnés — chacun une fois (vus), pour ne jamais boucler ni
// répéter une étape déjà remontée par un autre chemin.
func imprimerTransitivement(ctx context.Context, pool *pgxpool.Pool, parNom map[string]pipeline.EtapePubliee,
	noms []string, vus map[string]bool, indent string) error {
	for _, n := range noms {
		if vus[n] {
			continue
		}
		vus[n] = true
		e, ok := parNom[n]
		if !ok {
			continue
		}
		if err := imprimerEtape(ctx, pool, e, indent); err != nil {
			return err
		}
		if err := imprimerTransitivement(ctx, pool, parNom, e.DependDe, vus, indent+"  "); err != nil {
			return err
		}
	}
	return nil
}

func imprimerEtape(ctx context.Context, pool *pgxpool.Pool, e pipeline.EtapePubliee, indent string) error {
	derniere := "jamais exécutée avec succès"
	if e.DerniereExecutionReussie != nil {
		derniere = e.DerniereExecutionReussie.Local().Format("2006-01-02 15:04")
	}
	archive, base, err := tailleEtape(ctx, pool, e.Nom)
	if err != nil {
		return fmt.Errorf("%s : %w", e.Nom, err)
	}
	fmt.Printf("%s%s — %s", indent, e.Nom, derniere)
	if archive > 0 || base > 0 {
		fmt.Printf("  (archive %s, base %s)", tailleLisible(archive), tailleLisible(base))
	}
	fmt.Println()
	if e.Description != "" {
		fmt.Printf("%s  %s\n", indent, e.Description)
	}
	if len(e.DependDe) > 0 {
		fmt.Printf("%s  dépend de : %s\n", indent, strings.Join(e.DependDe, ", "))
	}
	return nil
}

func listerConnecteurs() error {
	cs, err := sources.ListerConnecteurs(".")
	if err != nil {
		return err
	}
	paquet := ""
	for _, c := range cs {
		if c.Paquet != paquet {
			paquet = c.Paquet
			fmt.Printf("%s\n", paquet)
		}
		fmt.Printf("  %s\n", c.Fonction)
	}
	fmt.Printf("\n%d connecteurs, %d paquets\n", len(cs), nombrePaquets(cs))
	return nil
}

func nombrePaquets(cs []sources.Connecteur) int {
	vus := map[string]bool{}
	for _, c := range cs {
		vus[c.Paquet] = true
	}
	return len(vus)
}

func afficherStats(ctx context.Context) error {
	pool, err := store.Open(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	schemas, err := stats.Resume(ctx, pool)
	if err != nil {
		return err
	}
	fmt.Printf("%-12s %8s %14s %10s\n", "schéma", "tables", "lignes (est.)", "taille")
	var totalOctets, totalLignes int64
	for _, s := range schemas {
		fmt.Printf("%-12s %8d %14d %10s\n", s.Nom, s.Tables, s.LignesEstimee, tailleLisible(s.Octets))
		totalOctets += s.Octets
		totalLignes += s.LignesEstimee
	}
	fmt.Printf("%-12s %8s %14d %10s\n", "total", "", totalLignes, tailleLisible(totalOctets))

	grosses, err := stats.PlusGrossesTables(ctx, pool, 10)
	if err != nil {
		return err
	}
	fmt.Printf("\nplus grosses tables :\n")
	for _, t := range grosses {
		fmt.Printf("  %-10s %-40s %12d lignes  %10s\n", t.Schema, t.Nom, t.LignesEstimee, tailleLisible(t.Octets))
	}
	return nil
}

func tailleLisible(octets int64) string {
	const unite = 1024.0
	v := float64(octets)
	for _, suffixe := range []string{"o", "Ko", "Mo", "Go", "To"} {
		if v < unite {
			return fmt.Sprintf("%.1f %s", v, suffixe)
		}
		v /= unite
	}
	return fmt.Sprintf("%.1f Po", v)
}
