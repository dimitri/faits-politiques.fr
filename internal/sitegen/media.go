package sitegen

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Media struct {
	Fichier, SourceURL, Licence, Auteur string
	Largeur, Hauteur                    int
}

// loadMedias charge les portraits et logos retenus. Seuls des fichiers sous
// licence libre sont en base : le crédit affiché n'est pas décoratif, il est la
// condition de la réutilisation.
func loadMedias(ctx context.Context, pool *pgxpool.Pool) (map[string]*Media, map[int64]*Media, error) {
	parPersonne := map[string]*Media{}
	rows, err := pool.Query(ctx, `
		SELECT p.slug, m.fichier, m.source_url, m.licence, coalesce(m.auteur,''),
		       coalesce(m.largeur,0), coalesce(m.hauteur,0)
		FROM core.media m JOIN core.person p ON p.id = m.person_id
		WHERE m.kind = 'PORTRAIT'`)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var slug string
		m := &Media{}
		if err := rows.Scan(&slug, &m.Fichier, &m.SourceURL, &m.Licence, &m.Auteur,
			&m.Largeur, &m.Hauteur); err != nil {
			return nil, nil, err
		}
		parPersonne[slug] = m
	}
	rows.Close()

	parOrg := map[int64]*Media{}
	rows, err = pool.Query(ctx, `
		SELECT m.organization_id, m.fichier, m.source_url, m.licence, coalesce(m.auteur,''),
		       coalesce(m.largeur,0), coalesce(m.hauteur,0)
		FROM core.media m WHERE m.kind = 'LOGO' AND m.organization_id IS NOT NULL`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		m := &Media{}
		if err := rows.Scan(&id, &m.Fichier, &m.SourceURL, &m.Licence, &m.Auteur,
			&m.Largeur, &m.Hauteur); err != nil {
			return nil, nil, err
		}
		parOrg[id] = m
	}
	return parPersonne, parOrg, rows.Err()
}

// copyMedia recopie les fichiers dans le site généré.
func copyMedia(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		in, err := os.Open(filepath.Join(src, e.Name()))
		if err != nil {
			return err
		}
		out, err := os.Create(filepath.Join(dst, e.Name()))
		if err != nil {
			in.Close()
			return err
		}
		_, err = io.Copy(out, in)
		in.Close()
		out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
