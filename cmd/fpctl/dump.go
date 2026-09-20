package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/faits-politiques/faits-politiques/internal/matview"
	"github.com/faits-politiques/faits-politiques/internal/store"
	"github.com/faits-politiques/faits-politiques/internal/toolrun"
	"github.com/spf13/cobra"
)

// commandeDump : fpctl dump ci, fpctl dump restaurer. Le périmètre CI —
// internal/matview.Perimetre(), le schéma mv plus les TablesDirectes —
// exporté vers un fichier pg_dump -Fc puis restauré dans une base vide, pour
// valider fpctl build site sans les ~10 Go de core/ref/geo/raw ni les
// connecteurs d'ingestion qui les remplissent (voir docs/ci-pipeline.md).
func commandeDump() *cobra.Command {
	cmd := &cobra.Command{Use: "dump", Short: "Exporte ou restaure le périmètre CI (schéma mv + TablesDirectes)"}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "ci [options]",
			Short: "Exporte le périmètre CI dans un fichier pg_dump -Fc",
			Long: "Recopie chaque matvue et chaque TableDirecte (internal/matview) dans\n" +
				"un schéma jetable sous forme de tables ordinaires — jamais la matvue\n" +
				"elle-même : pg_restore recalculerait son contenu par REFRESH, ce qui\n" +
				"exigerait exactement les grandes tables core/ref que ce périmètre\n" +
				"existe pour éviter. pg_dump -n de ce seul schéma, puis nettoyage.",
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				if estDemandeAide(args) {
					return afficherManuel("fpctl-dump")
				}
				return executerInterne(cmd.Context(), dumpCI(cmd.Context(), args))
			},
		},
		&cobra.Command{
			Use:   "restaurer [options]",
			Short: "Restaure un export CI dans la base ciblée par DATABASE_URL",
			Long: "Restaure le schéma jetable de fpctl dump ci puis remet chaque table à\n" +
				"sa place réelle (core.mandate, mv.person_actif...) — la base cible\n" +
				"n'a besoin d'aucun schéma core/ref/mv préexistant, seulement d'être\n" +
				"vide et joignable. Pensé pour une base CI fraîche, mais tout aussi\n" +
				"utile pour rejouer localement la méthode qui referme le périmètre :\n" +
				"restaurer, lancer fpctl build site, ajouter à internal/matview ce\n" +
				"qu'une erreur de relation manquante désigne, recommencer.",
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				if estDemandeAide(args) {
					return afficherManuel("fpctl-dump")
				}
				return executerInterne(cmd.Context(), restaurerCI(cmd.Context(), args))
			},
		},
	)
	return cmd
}

func dumpCI(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("dump ci", flag.ContinueOnError)
	sortie := fs.String("out", "ci.dump", "fichier de sortie (format pg_dump -Fc)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	pool, err := store.Open(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := matview.ExporterCI(ctx, pool); err != nil {
		return err
	}
	// Nettoyé même si pg_dump échoue : le schéma jetable ne doit jamais
	// survivre à cette commande dans la base réelle.
	defer pool.Exec(ctx, "DROP SCHEMA IF EXISTS "+matview.SchemaExport+" CASCADE")

	if err := (toolrun.Cmd{
		Name: "pg_dump",
		Args: []string{"-Fc", "-n", matview.SchemaExport, "-f", *sortie, store.DSN()},
		Ctx:  ctx,
	}).Run(); err != nil {
		return err
	}
	fmt.Printf("périmètre CI exporté dans %s\n", *sortie)
	return nil
}

func restaurerCI(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("dump restaurer", flag.ContinueOnError)
	fichier := fs.String("fichier", "ci.dump", "fichier à restaurer (produit par fpctl dump ci)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if err := (toolrun.Cmd{
		Name: "pg_restore",
		Args: []string{"-d", store.DSN(), "--clean", "--if-exists", "--no-owner", *fichier},
		Ctx:  ctx,
	}).Run(); err != nil {
		return err
	}

	pool, err := store.Open(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := matview.RemettreEnPlace(ctx, pool); err != nil {
		return err
	}
	fmt.Println("périmètre CI restauré et remis en place")
	return nil
}
