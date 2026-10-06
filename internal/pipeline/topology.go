package pipeline

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PublishedStep : une ligne de core.pipeline_etape, ses dépendances jointes
// depuis core.pipeline_dependance — ce que Publish y a écrit à la dernière
// exécution d'ingestion. Jamais la source de vérité (le code Go l'est),
// mais ce qu'une commande comme fpctl list deps peut lire sans ouvrir
// internal/ingest/ingest.go.
type PublishedStep struct {
	Name              string
	Description       string
	LastSuccessfulRun *time.Time
	DependsOn         []string
}

// ReadTopology lit la topologie publiée en base — vide si aucune ingestion
// n'a encore tourné depuis la migration 0142 (core.pipeline_etape n'existe
// alors que par sa structure, pas par son contenu).
func ReadTopology(ctx context.Context, pool *pgxpool.Pool) ([]PublishedStep, error) {
	rows, err := pool.Query(ctx,
		`SELECT nom, description, derniere_execution_reussie FROM core.pipeline_etape ORDER BY nom`)
	if err != nil {
		return nil, err
	}
	byName := map[string]*PublishedStep{}
	var order []string
	for rows.Next() {
		var s PublishedStep
		if err := rows.Scan(&s.Name, &s.Description, &s.LastSuccessfulRun); err != nil {
			rows.Close()
			return nil, err
		}
		byName[s.Name] = &s
		order = append(order, s.Name)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	drows, err := pool.Query(ctx, `SELECT etape, depend_de FROM core.pipeline_dependance ORDER BY etape, depend_de`)
	if err != nil {
		return nil, err
	}
	defer drows.Close()
	for drows.Next() {
		var step, dependsOn string
		if err := drows.Scan(&step, &dependsOn); err != nil {
			return nil, err
		}
		if s, ok := byName[step]; ok {
			s.DependsOn = append(s.DependsOn, dependsOn)
		}
	}
	if err := drows.Err(); err != nil {
		return nil, err
	}

	out := make([]PublishedStep, 0, len(order))
	for _, name := range order {
		out = append(out, *byName[name])
	}
	return out, nil
}
