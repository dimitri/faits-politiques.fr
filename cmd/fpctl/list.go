package main

import (
	"context"
	"fmt"

	"github.com/faits-politiques/faits-politiques/internal/sources"
	"github.com/faits-politiques/faits-politiques/internal/stats"
	"github.com/faits-politiques/faits-politiques/internal/store"
	"github.com/spf13/cobra"
)

// commandeList : fpctl list sources, fpctl list connectors, fpctl list
// stats — trois vues différentes sur la même question, « qu'est-ce qui est
// chargé et par quoi » : les sources déclarées (raw.source), le code qui
// les charge (les fonctions Ingest* d'internal/), et ce que ça pèse une
// fois en base.
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
	)
	return cmd
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
