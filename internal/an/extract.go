// Package an ingère l'open data de l'Assemblée nationale.
//
// Chaîne : téléchargement -> archive scellée -> raw.record -> core.
// Chaque étape est idempotente : rejouer l'ingestion doit produire un état
// identique (db/README.md, invariant de reconstructibilité).
package an

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ConnectorVersion = "an/1"

const Base = "https://data.assemblee-nationale.fr/static/openData/repository/17"

var Sources = map[string]archive.Source{
	"an-amo": {
		Slug: "an-amo", Label: "AN — Tous acteurs, mandats et organes (AMO30)",
		Publisher: "Assemblée nationale", Tier: "PRIMARY_OFFICIAL",
		Licence: "Licence Ouverte", ReuseClass: "ATTRIBUTION",
		Attribution: "Source : Assemblée nationale, open data",
		Cadence:     "continue, non contractuelle",
		Notes:       "Un champ peut être un objet ou un tableau selon le nombre d'éléments.",
	},
	"an-dossiers": {
		Slug: "an-dossiers", Label: "AN — Dossiers législatifs (17e législature)",
		Publisher: "Assemblée nationale", Tier: "PRIMARY_OFFICIAL",
		Licence: "Licence Ouverte", ReuseClass: "ATTRIBUTION",
		Attribution: "Source : Assemblée nationale, open data",
		Cadence:     "continue",
		Notes: "Les exposés des motifs ne figurent PAS dans le JSON : seuls titres, " +
			"auteurs, dates et étapes y sont. Un résumé rédigé ne peut donc pas en être " +
			"tiré mécaniquement.",
	},
	"an-scrutins": {
		Slug: "an-scrutins", Label: "AN — Scrutins publics (17e législature)",
		Publisher: "Assemblée nationale", Tier: "PRIMARY_OFFICIAL",
		Licence: "Licence Ouverte", ReuseClass: "ATTRIBUTION",
		Attribution: "Source : Assemblée nationale, open data",
		Cadence:     "par séance",
		Notes: "Ne couvre que les scrutins PUBLICS : la majorité des votes ont lieu " +
			"à main levée et ne laissent aucune trace nominative.",
	},
}

// Download récupère les deux archives et les scelle.
func Download(ctx context.Context, arch *archive.Archive) (map[string]*archive.Fetched, error) {
	out := map[string]*archive.Fetched{}
	urls := map[string]string{
		// AMO30 « tous acteurs, tous mandats, tous organes » et non AMO40
		// (« députés actifs ») ni AMO50 : ces deux derniers omettent des députés
		// ayant siégé puis quitté leur siège en cours de législature. Leurs votes
		// figurent pourtant dans les scrutins, et charger sans eux ferait
		// disparaître silencieusement des milliers de votes — c'est-à-dire lire
		// une absence de données comme une absence d'action.
		// Le contrôle de concordance de cmd/verify est précisément là pour
		// empêcher qu'une telle erreur soit publiée.
		"an-amo":      Base + "/amo/tous_acteurs_mandats_organes_xi_legislature/AMO30_tous_acteurs_tous_mandats_tous_organes_historique.json.zip",
		"an-scrutins": Base + "/loi/scrutins/Scrutins.json.zip",
		"an-dossiers": Base + "/loi/dossiers_legislatifs/Dossiers_Legislatifs.json.zip",
	}
	for slug, url := range urls {
		srcID, err := arch.EnsureSource(ctx, Sources[slug])
		if err != nil {
			return nil, err
		}
		runID, err := arch.StartRun(ctx, srcID, ConnectorVersion)
		if err != nil {
			return nil, err
		}
		f, err := arch.Fetch(ctx, srcID, runID, url, ".zip")
		if err != nil {
			arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
			return nil, fmt.Errorf("%s : %w", slug, err)
		}
		arch.EndRun(ctx, runID, "SUCCESS",
			map[string]any{"sha256": f.SHA256, "deja_archive": f.Cached}, "")
		state := "archivé"
		if f.Cached {
			state = "inchangé"
		}
		fmt.Printf("  %-12s %s (%s…)\n", slug, state, f.SHA256[:12])
		out[slug] = f
	}
	return out, nil
}

// Extract déplie une archive zip vers raw.record. Le type d'enregistrement est
// déduit du répertoire, la clé naturelle de l'identifiant officiel.
func Extract(ctx context.Context, pool *pgxpool.Pool, f *archive.Fetched) (int, error) {
	var existing int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM raw.record WHERE document_id = $1`, f.DocumentID).Scan(&existing); err != nil {
		return 0, err
	}
	if existing > 0 {
		return existing, nil // déjà déplié : l'extraction est une fonction pure du document
	}

	zr, err := zip.OpenReader(f.Path)
	if err != nil {
		return 0, err
	}
	defer zr.Close()

	type row struct {
		recType string
		key     string
		payload string
	}
	var rows []row

	for _, e := range zr.File {
		if e.FileInfo().IsDir() || !strings.HasSuffix(e.Name, ".json") {
			continue
		}
		rc, err := e.Open()
		if err != nil {
			return 0, err
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return 0, err
		}

		var wrapper map[string]json.RawMessage
		if err := json.Unmarshal(b, &wrapper); err != nil {
			continue // fichier non conforme : ignoré, jamais deviné
		}
		for k, v := range wrapper {
			var obj map[string]json.RawMessage
			if err := json.Unmarshal(v, &obj); err != nil {
				continue
			}
			uid := str(obj["uid"])
			if uid == "" {
				continue
			}
			rows = append(rows, row{recType: "an." + k, key: uid, payload: string(v)})
		}
	}

	n, err := pool.CopyFrom(ctx,
		pgx.Identifier{"raw", "record"},
		[]string{"document_id", "record_type", "natural_key", "payload"},
		pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
			return []any{f.DocumentID, rows[i].recType, rows[i].key, rows[i].payload}, nil
		}))
	return int(n), err
}
