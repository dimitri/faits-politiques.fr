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

	"github.com/faits-politiques/faits-politiques/internal/agriculture"
	"github.com/faits-politiques/faits-politiques/internal/an"
	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/carto"
	"github.com/faits-politiques/faits-politiques/internal/communes"
	"github.com/faits-politiques/faits-politiques/internal/entreprises"
	"github.com/faits-politiques/faits-politiques/internal/europe"
	"github.com/faits-politiques/faits-politiques/internal/hatvp"
	"github.com/faits-politiques/faits-politiques/internal/macro"
	"github.com/faits-politiques/faits-politiques/internal/migrate"
	"github.com/faits-politiques/faits-politiques/internal/partis"
	"github.com/faits-politiques/faits-politiques/internal/presidentielle"
	"github.com/faits-politiques/faits-politiques/internal/senat"
	"github.com/faits-politiques/faits-politiques/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	only := flag.String("only", "", "migrate | download | partis | europe | senat | normalize | carto | communes | epci | ssmsi | municipales2020 | entreprises | agriculture | hatvp | macro | presidentielle | media")
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
		// Le répertoire des sénateurs vient après les votes : il complète des
		// personnes existantes, il n'en crée aucune.
		if err := senat.IngestSenateurs(ctx, pool, arch); err != nil {
			return err
		}
		// Recharger le Sénat reconstruit ses dossiers avec de NOUVEAUX
		// identifiants, et derived.scrutin_topic les référence en cascade : les
		// 4 806 thèmes hérités par la navette disparaissent sans un message.
		// Un commentaire dans le connecteur n'a pas suffi — le piège s'est
		// refermé deux fois. Le recalcul est donc fait ici, où il ne peut plus
		// être oublié.
		fmt.Println("\nthèmes applicables aux scrutins")
		if err := carto.Themes(ctx, pool); err != nil {
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

	if only == "carto" {
		return cartographie(ctx, pool)
	}

	if only == "" || only == "communes" {
		if err := dimensionLocale(ctx, pool, arch); err != nil {
			return err
		}
	}
	if only == "communes" {
		return nil
	}

	// Le seul groupement : recharger BANATIC sans repasser par les 615 000
	// mandats du RNE ni les 1,7 million de valeurs de l'OFGL.
	if only == "epci" {
		fmt.Println("\nintercommunalités et compétences")
		return communes.IngestBANATIC(ctx, pool, arch)
	}

	if only == "municipales2020" {
		fmt.Println("\nélections municipales 2020")
		return communes.IngestMunicipales2020(ctx, pool, arch)
	}

	if only == "ssmsi" {
		fmt.Println("\ndélinquance enregistrée par commune")
		return communes.IngestSSMSI(ctx, pool, arch)
	}

	if only == "" || only == "hatvp" {
		fmt.Println("\ndéclarations d'intérêts et de patrimoine")
		if err := hatvp.Ingest(ctx, pool, arch); err != nil {
			return err
		}
	}
	if only == "hatvp" {
		return nil
	}

	if only == "" || only == "presidentielle" {
		fmt.Println("\nélection présidentielle, population par âge, participation comparée")
		if err := presidentielle.Ingest(ctx, pool, arch); err != nil {
			return err
		}
		if err := presidentielle.IngestPopulation(ctx, pool, arch); err != nil {
			return err
		}
		if err := presidentielle.IngestTurnout(ctx, pool, arch); err != nil {
			return err
		}
	}
	if only == "presidentielle" {
		return nil
	}

	if only == "" || only == "macro" {
		fmt.Println("\ngrandes séries nationales")
		if err := macro.Ingest(ctx, pool, arch); err != nil {
			return err
		}
		if err := macro.IngestRSA(ctx, pool, arch); err != nil {
			return err
		}
	}
	if only == "" || only == "agriculture" {
		fmt.Println("\nbilans alimentaires et appareil de production agricole")
		if err := agriculture.Ingest(ctx, pool, arch); err != nil {
			return err
		}
	}
	if only == "agriculture" {
		return nil
	}

	if only == "entreprises" {
		fmt.Println("\ncomptes déposés des grandes sociétés")
		return entreprises.Ingest(ctx, pool, arch)
	}

	if only == "" || only == "macro" {
		fmt.Println("\ncomptes déposés des grandes sociétés")
		if err := entreprises.Ingest(ctx, pool, arch); err != nil {
			return err
		}
	}
	if only == "macro" {
		return nil
	}

	fmt.Println("\nnormalisation raw -> core")
	if err := an.Normalize(ctx, pool); err != nil {
		return err
	}

	if err := cartographie(ctx, pool); err != nil {
		return err
	}

	fmt.Println("\nportraits et logos librement réutilisables")
	if err := ingestMedia(ctx, pool, arch, "data", "web/media"); err != nil {
		return err
	}

	fmt.Printf("\nterminé en %s\n", time.Since(start).Round(time.Second))
	return nil
}

// cartographie charge les décisions de rattachement puis en déduit le thème
// applicable à chaque scrutin. Les deux vont ensemble : sans le rattachement
// parti -> groupe, un thème ne se relie à aucune famille politique.
func cartographie(ctx context.Context, pool *pgxpool.Pool) error {
	fmt.Println("\ncartographie éditoriale")
	if err := carto.Ingest(ctx, pool, filepath.Join("data", "organisations.csv")); err != nil {
		return err
	}
	fmt.Println("\nprésidences de la République")
	if err := carto.IngestPresidents(ctx, pool, filepath.Join("data", "presidents.csv")); err != nil {
		return err
	}
	fmt.Println("\nthèmes applicables aux scrutins")
	return carto.Themes(ctx, pool)
}

// dimensionLocale charge la dimension communale, dans un ordre contraint :
// ref.commune est référencé par tout le reste, et les résultats électoraux ne
// peuvent pas être rattachés à une commune qui n'existe pas encore.
func dimensionLocale(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	fmt.Println("\nréférentiel géographique")
	if err := communes.IngestCOG(ctx, pool, arch); err != nil {
		return err
	}
	fmt.Println("\nmaires")
	if err := communes.IngestRNE(ctx, pool, arch); err != nil {
		return err
	}
	fmt.Println("\nélections municipales")
	if err := communes.IngestMunicipales(ctx, pool, arch); err != nil {
		return err
	}
	fmt.Println("\ncomptes des communes")
	if err := communes.IngestOFGL(ctx, pool, arch); err != nil {
		return err
	}
	fmt.Println("\nintercommunalités et compétences")
	if err := communes.IngestBANATIC(ctx, pool, arch); err != nil {
		return err
	}
	fmt.Println("\nélections municipales 2020")
	if err := communes.IngestMunicipales2020(ctx, pool, arch); err != nil {
		return err
	}
	fmt.Println("\ndélinquance enregistrée par commune")
	return communes.IngestSSMSI(ctx, pool, arch)
}
