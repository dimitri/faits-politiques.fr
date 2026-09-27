package pipeline

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// EtapePubliee : une ligne de core.pipeline_etape, ses dépendances jointes
// depuis core.pipeline_dependance — ce que Publier y a écrit à la dernière
// exécution d'ingestion. Jamais la source de vérité (le code Go l'est),
// mais ce qu'une commande comme fpctl list deps peut lire sans ouvrir
// internal/ingest/ingest.go.
type EtapePubliee struct {
	Nom                      string
	Description              string
	DerniereExecutionReussie *time.Time
	DependDe                 []string
}

// LireTopologie lit la topologie publiée en base — vide si aucune
// ingestion n'a encore tourné depuis la migration 0142 (core.pipeline_etape
// n'existe alors que par sa structure, pas par son contenu).
func LireTopologie(ctx context.Context, pool *pgxpool.Pool) ([]EtapePubliee, error) {
	rows, err := pool.Query(ctx,
		`SELECT nom, description, derniere_execution_reussie FROM core.pipeline_etape ORDER BY nom`)
	if err != nil {
		return nil, err
	}
	parNom := map[string]*EtapePubliee{}
	var ordre []string
	for rows.Next() {
		var e EtapePubliee
		if err := rows.Scan(&e.Nom, &e.Description, &e.DerniereExecutionReussie); err != nil {
			rows.Close()
			return nil, err
		}
		parNom[e.Nom] = &e
		ordre = append(ordre, e.Nom)
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
		var etape, dependDe string
		if err := drows.Scan(&etape, &dependDe); err != nil {
			return nil, err
		}
		if e, ok := parNom[etape]; ok {
			e.DependDe = append(e.DependDe, dependDe)
		}
	}
	if err := drows.Err(); err != nil {
		return nil, err
	}

	out := make([]EtapePubliee, 0, len(ordre))
	for _, nom := range ordre {
		out = append(out, *parNom[nom])
	}
	return out, nil
}
