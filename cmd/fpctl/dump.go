package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/faits-politiques/faits-politiques/internal/matview"
	"github.com/faits-politiques/faits-politiques/internal/objectstore"
	"github.com/faits-politiques/faits-politiques/internal/store"
	"github.com/faits-politiques/faits-politiques/internal/toolrun"
	"github.com/minio/minio-go/v7"
	"github.com/spf13/cobra"
)

// commandeDump : fpctl dump ci, fpctl dump restore. Le périmètre CI —
// internal/matview.Perimetre(), le schéma mv plus les TablesDirectes —
// exporté vers un fichier pg_dump -Fc puis restauré dans une base vide, pour
// valider fpctl build site sans les ~10 Go de core/ref/geo/raw ni les
// connecteurs d'ingestion qui les remplissent (voir docs/ci-pipeline.md).
func commandeDump() *cobra.Command {
	cmd := &cobra.Command{Use: "dump", Short: "Exporte ou restaure le périmètre CI (schéma mv + TablesDirectes)"}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "ci [options]",
			Short: "Actualise les matvues puis exporte le périmètre CI dans un fichier pg_dump -Fc",
			Long: "Actualise d'abord chaque matvue (internal/matview.ActualiserToutes —\n" +
				"un REFRESH sauté si rien n'a changé, jamais gratuit à ignorer : sans\n" +
				"cet appel, un export lancé hors de « fpctl ingest all » capturerait\n" +
				"silencieusement le contenu d'un cycle d'ingestion antérieur), puis\n" +
				"recopie chaque matvue et chaque TableDirecte dans un schéma jetable\n" +
				"sous forme de tables ordinaires — jamais la matvue elle-même :\n" +
				"pg_restore recalculerait son contenu par REFRESH, ce qui exigerait\n" +
				"exactement les grandes tables core/ref que ce périmètre existe pour\n" +
				"éviter. pg_dump -n de ce seul schéma, puis nettoyage.\n\n" +
				"-upload envoie le fichier obtenu vers un object storage compatible\n" +
				"S3 (mêmes variables d'environnement que fpctl sync, voir\n" +
				"internal/objectstore) sous -bucket/-key, en plus de le garder en local.",
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				if estDemandeAide(args) {
					return afficherManuel("fpctl-dump")
				}
				return executerInterne(cmd.Context(), runDumpCI(cmd.Context(), args))
			},
		},
		&cobra.Command{
			Use:   "restore [options]",
			Short: "Restaure un export CI dans la base ciblée par DATABASE_URL",
			Long: "Restaure le schéma jetable de fpctl dump ci puis remet chaque table à\n" +
				"sa place réelle (core.mandate, mv.person_actif...) — la base cible\n" +
				"n'a besoin d'aucun schéma core/ref/mv préexistant, seulement d'être\n" +
				"vide et joignable. Pensé pour une base CI fraîche, mais tout aussi\n" +
				"utile pour rejouer localement la méthode qui referme le périmètre :\n" +
				"restaurer, lancer fpctl build site, ajouter à internal/matview ce\n" +
				"qu'une erreur de relation manquante désigne, recommencer.\n\n" +
				"-download récupère d'abord le fichier depuis l'object storage\n" +
				"(-bucket/-key) avant de le restaurer.",
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				if estDemandeAide(args) {
					return afficherManuel("fpctl-dump")
				}
				return executerInterne(cmd.Context(), runRestoreCI(cmd.Context(), args))
			},
		},
	)
	return cmd
}

// ciBucket/ciKey : où fpctl dump ci envoie le fichier par défaut, et où
// fpctl dump restore -download va le chercher — le préfixe ci/ du bucket
// fp-archive déjà décidé pour l'archive scellée (docs/ci-pipeline.md),
// jamais un second bucket à sécuriser séparément pour 56 Mo.
const (
	ciBucket = "fp-archive"
	ciKey    = "ci/perimetre.dump"
)

func runDumpCI(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("dump ci", flag.ContinueOnError)
	outFile := fs.String("out", "ci.dump", "fichier de sortie (format pg_dump -Fc)")
	upload := fs.Bool("upload", false, "envoie aussi le fichier vers l'object storage (-bucket/-key)")
	bucket := fs.String("bucket", ciBucket, "bucket de destination (avec -upload)")
	key := fs.String("key", ciKey, "clé de destination dans le bucket (avec -upload)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	pool, err := store.Open(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	// Sans cet appel, ExportCI capturerait le contenu déjà présent dans
	// mv.* — potentiellement celui d'un cycle d'ingestion antérieur si
	// personne n'a relancé "fpctl ingest all"/"fpctl ingest systeme
	// matviews" entretemps. ActualiserToutes ne fait rien (donc ne coûte
	// qu'un aller-retour d'empreinte par matvue) quand tout est déjà à jour.
	if err := matview.ActualiserToutes(ctx, pool); err != nil {
		return fmt.Errorf("actualisation des matvues avant export : %w", err)
	}

	if err := matview.ExportCI(ctx, pool); err != nil {
		return err
	}
	// Nettoyé même si pg_dump échoue : le schéma jetable ne doit jamais
	// survivre à cette commande dans la base réelle.
	defer pool.Exec(ctx, "DROP SCHEMA IF EXISTS "+matview.ExportSchema+" CASCADE")

	if err := (toolrun.Cmd{
		Name: "pg_dump",
		Args: []string{"-Fc", "-n", matview.ExportSchema, "-f", *outFile, store.DSN()},
		Ctx:  ctx,
	}).Run(); err != nil {
		return err
	}
	fmt.Printf("périmètre CI exporté dans %s\n", *outFile)

	if !*upload {
		return nil
	}
	return uploadDumpFile(ctx, *outFile, *bucket, *key)
}

// uploadDumpFile envoie chemin sous bucket/cle — internal/objectstore ne
// sait synchroniser qu'un RÉPERTOIRE (SyncDir, déjà utilisé par fpctl sync
// archive/site) : un répertoire temporaire d'un seul fichier, nommé comme la
// clé voulue, le lui fait faire sans dupliquer sa logique de déduplication
// par taille (un fichier déjà présent à la même taille n'est pas renvoyé).
func uploadDumpFile(ctx context.Context, path, bucket, key string) error {
	tmp, err := os.MkdirTemp("", "fpctl-dump-upload-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	dest := filepath.Join(tmp, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return err
	}

	c, err := objectstore.Client(objectstore.DepuisEnv())
	if err != nil {
		return err
	}
	n, octets, err := objectstore.SyncDir(ctx, c, bucket, tmp)
	if err != nil {
		return err
	}
	if n == 0 {
		fmt.Printf("%s → %s/%s : déjà à jour (même taille), rien envoyé\n", path, bucket, key)
		return nil
	}
	fmt.Printf("%s → %s/%s : envoyé (%.1f Mo)\n", path, bucket, key, float64(octets)/1e6)
	return nil
}

func runRestoreCI(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("dump restore", flag.ContinueOnError)
	inFile := fs.String("fichier", "ci.dump", "fichier à restaurer (produit par fpctl dump ci)")
	download := fs.Bool("download", false, "récupère d'abord le fichier depuis l'object storage (-bucket/-key)")
	bucket := fs.String("bucket", ciBucket, "bucket source (avec -download)")
	key := fs.String("key", ciKey, "clé source dans le bucket (avec -download)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *download {
		c, err := objectstore.Client(objectstore.DepuisEnv())
		if err != nil {
			return err
		}
		if err := c.FGetObject(ctx, *bucket, *key, *inFile, minio.GetObjectOptions{}); err != nil {
			return fmt.Errorf("téléchargement de %s/%s : %w", *bucket, *key, err)
		}
		fmt.Printf("%s/%s → %s\n", *bucket, *key, *inFile)
	}

	if err := (toolrun.Cmd{
		Name: "pg_restore",
		Args: []string{"-d", store.DSN(), "--clean", "--if-exists", "--no-owner", *inFile},
		Ctx:  ctx,
	}).Run(); err != nil {
		return err
	}

	pool, err := store.Open(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := matview.RestoreExported(ctx, pool); err != nil {
		return err
	}
	fmt.Println("périmètre CI restauré et remis en place")
	return nil
}
