package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/faits-politiques/faits-politiques/internal/objectstore"
	"github.com/spf13/cobra"
)

// commandeSync : fpctl sync archive, fpctl sync site. Pousse un répertoire
// local vers un bucket compatible S3 (voir internal/objectstore) — jamais
// le chemin de service normal aujourd'hui (le site est servi depuis le
// disque, l'archive lue depuis raw/), mais ce qu'il faut pour évaluer
// l'alternative sans improviser un script à chaque fois.
func commandeSync() *cobra.Command {
	cmd := &cobra.Command{Use: "sync", Short: "Envoie un répertoire local vers l'object store"}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "archive [options]",
			Short: "Envoie l'archive scellée (raw/) vers le bucket fp-archive",
			Long: "Envoie chaque document de l'archive scellée vers un object storage\n" +
				"compatible S3 (fpctl provision store pour un MinIO local), sous une\n" +
				"clé égale à son chemin relatif. Un objet déjà présent à la même\n" +
				"taille n'est pas renvoyé.",
			DisableFlagParsing: true,
			RunE: func(_ *cobra.Command, args []string) error {
				if estDemandeAide(args) {
					return afficherManuel("fpctl-sync")
				}
				return executerInterne(syncVers("archive", "raw", args))
			},
		},
		&cobra.Command{
			Use:   "site [options]",
			Short: "Envoie le site généré (site/) vers le bucket fp-site",
			Long: "Envoie chaque page du site généré par fpctl build site vers un\n" +
				"object storage compatible S3, sous une clé égale à son chemin\n" +
				"relatif — de quoi comparer une piste « servir depuis le Blob\n" +
				"Storage » à ce que sert aujourd'hui le disque local.",
			DisableFlagParsing: true,
			RunE: func(_ *cobra.Command, args []string) error {
				if estDemandeAide(args) {
					return afficherManuel("fpctl-sync")
				}
				return executerInterne(syncVers("site", "site", args))
			},
		},
	)
	return cmd
}

func syncVers(bucketDefaut, racineDefaut string, args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	bucket := fs.String("bucket", "fp-"+bucketDefaut, "bucket de destination")
	racine := fs.String("dir", racineDefaut, "répertoire local à envoyer")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx := context.Background()
	c, err := objectstore.Client(objectstore.DepuisEnv())
	if err != nil {
		return err
	}
	n, octets, err := objectstore.SyncDir(ctx, c, *bucket, *racine)
	if err != nil {
		return err
	}
	fmt.Printf("%s → %s : %d objets envoyés (%.1f Mo)\n", *racine, *bucket, n, float64(octets)/1e6)
	return nil
}
