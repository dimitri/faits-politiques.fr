// Commande ingest : télécharge les jeux de données, les scelle dans l'archive,
// puis reconstruit core à partir de raw.
//
//	go run ./cmd/ingest              chaîne complète
//	go run ./cmd/ingest -only=migrate
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/an"
	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/europe"
	"github.com/faits-politiques/faits-politiques/internal/migrate"
	"github.com/faits-politiques/faits-politiques/internal/partis"
	"github.com/faits-politiques/faits-politiques/internal/senat"
	"github.com/faits-politiques/faits-politiques/internal/store"
)

func main() {
	only := flag.String("only", "", "migrate | download | partis | europe | senat | normalize | media")
	rawDir := flag.String("raw", "raw", "répertoire de l'archive scellée")
	migDir := flag.String("migrations", "db/migrations", "répertoire des migrations")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := run(ctx, *only, *rawDir, *migDir); err != nil {
		fmt.Fprintf(os.Stderr, "\nerreur : %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, only, rawDir, migDir string) error {
	start := time.Now()
	pool, err := store.Open(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	fmt.Println("migrations")
	if err := migrate.Up(ctx, pool, migDir); err != nil {
		return err
	}
	if only == "migrate" {
		return nil
	}

	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		return err
	}
	arch := &archive.Archive{Root: rawDir, Pool: pool}

	if only == "" || only == "download" {
		fmt.Println("\ntéléchargement et scellement")
		fetched, err := an.Download(ctx, arch)
		if err != nil {
			return err
		}
		fmt.Println("\nextraction vers raw.record")
		for slug, f := range fetched {
			n, err := an.Extract(ctx, pool, f)
			if err != nil {
				return fmt.Errorf("%s : %w", slug, err)
			}
			fmt.Printf("  %-12s %d enregistrements\n", slug, n)
		}
	}
	if only == "download" {
		return nil
	}

	if only == "" || only == "partis" {
		fmt.Println("\nréférentiels sur les organisations politiques")
		if err := partis.IngestComptes(ctx, pool, arch); err != nil {
			return err
		}
		if err := partis.IngestPopuList(ctx, pool, arch); err != nil {
			return err
		}
		if err := partis.IngestCHES(ctx, pool, arch); err != nil {
			return err
		}
	}
	if only == "partis" {
		return nil
	}

	if only == "media" {
		fmt.Println("\nportraits et logos librement réutilisables")
		return ingestMedia(ctx, pool, arch, "data", "web/media")
	}

	if only == "" || only == "senat" {
		fmt.Println("\nSénat")
		if err := senat.Ingest(ctx, pool, arch, filepath.Join(rawDir, "senat-work")); err != nil {
			return err
		}
	}
	if only == "senat" {
		return nil
	}

	if only == "" || only == "europe" {
		fmt.Println("\nParlement européen")
		if err := europe.Ingest(ctx, pool, arch); err != nil {
			return err
		}
	}
	if only == "europe" {
		return nil
	}

	fmt.Println("\nnormalisation raw -> core")
	if err := an.Normalize(ctx, pool); err != nil {
		return err
	}

	fmt.Println("\nportraits et logos librement réutilisables")
	if err := ingestMedia(ctx, pool, arch, "data", "web/media"); err != nil {
		return err
	}

	fmt.Printf("\nterminé en %s\n", time.Since(start).Round(time.Second))
	return nil
}
