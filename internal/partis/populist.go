package partis

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PopuList est une CLASSIFICATION, pas un fait : un jugement d'experts, publié
// avec son vocabulaire propre. Il est stocké tel quel et affiché sous la forme
// « far right selon PopuList v4.0 ». Jamais traduit — « far right » n'est pas
// « extrême droite » : la littérature distingue radical (rejette la démocratie
// libérale, accepte l'élection) et extrême (rejette la démocratie), distinction
// que l'usage français ignore. Jamais synthétisé non plus : moyenner deux
// référentiels fabriquerait un jugement et le présenterait comme un fait.
var SourcePopuList = archive.Source{
	Slug: "populist-v4", Label: "The PopuList 4.0",
	Publisher: "The PopuList", Tier: "SECONDARY_PRESS",
	Licence: "CC BY 4.0", ReuseClass: "ATTRIBUTION",
	Attribution: "Source : The PopuList 4.0 (Rooduijn et al.), CC BY 4.0",
	Cadence:     "par version",
	Notes: "Classification qualitative informée par experts. Les catégories sont DATÉES : " +
		"un parti peut n'y entrer qu'à partir d'une certaine année.",
}

const PopuListURL = "https://popu-list.github.io/Data/The%20PopuList%204.0.csv"

var popuListCategories = []string{"populist", "farright", "farleft", "eurosceptic"}

func IngestPopuList(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourcePopuList)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersion)
	if err != nil {
		return err
	}
	f, err := arch.Fetch(ctx, srcID, runID, PopuListURL, ".csv")
	if err != nil {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	var setID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO ref.classification_set (slug, provider, version, label, vocabulary, source_id)
		VALUES ('populist-v4','PopuList','4.0','The PopuList 4.0',
		        'populist / far right / far left / eurosceptic — vocabulaire anglophone de la source, affiché sans traduction',
		        $1)
		ON CONFLICT (provider, version) DO UPDATE SET label = EXCLUDED.label
		RETURNING id`, srcID).Scan(&setID); err != nil {
		return err
	}

	if _, err := pool.Exec(ctx,
		`DELETE FROM core.party_classification WHERE classification_set_id = $1`, setID); err != nil {
		return err
	}

	fh, err := os.Open(f.Path)
	if err != nil {
		return err
	}
	defer fh.Close()
	r := csv.NewReader(fh)
	r.Comma = ';'
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		return err
	}
	head[0] = strings.TrimPrefix(head[0], "\ufeff")
	idx := map[string]int{}
	for i, h := range head {
		idx[strings.TrimSpace(h)] = i
	}
	get := func(rec []string, k string) string {
		if i, ok := idx[k]; ok && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}

	// La correspondance parti français -> parti PopuList est une DÉCISION
	// éditoriale : elle vit dans data/organisations.csv, pas ici. L'ingestion
	// ne fait que charger la table de référence, indexée par le nom publié par
	// PopuList ; le rapprochement est fait au build.
	nParties, nRows := 0, 0
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		if get(rec, "country_name") != "France" {
			continue
		}
		nom := get(rec, "party_name")
		if nom == "" {
			continue
		}
		orgID, err := upsertClassifiedParty(ctx, pool, nom, get(rec, "party_name_short"),
			get(rec, "partyfacts_id"))
		if err != nil {
			return err
		}
		nParties++
		for _, cat := range popuListCategories {
			if get(rec, cat) != "1" {
				continue
			}
			debut := yearOf(get(rec, cat+"_start"))
			fin := yearOf(get(rec, cat+"_end"))
			if _, err := pool.Exec(ctx, `
				INSERT INTO core.party_classification
				  (classification_set_id, party_id, category, periode_debut, periode_fin)
				VALUES ($1,$2,$3,$4,$5)
				ON CONFLICT DO NOTHING`, setID, orgID, cat, debut, fin); err != nil {
				return err
			}
			nRows++
		}
	}
	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"partis_france": nParties, "classifications": nRows}, "")
	fmt.Printf("  PopuList 4.0  %d partis français, %d classifications\n", nParties, nRows)
	return nil
}

// upsertClassifiedParty crée l'organisation portant le nom publié par le
// référentiel. Elle est distincte d'une éventuelle entrée CNCCFP : les
// rapprocher est une décision, prise dans data/organisations.csv.
func upsertClassifiedParty(ctx context.Context, pool *pgxpool.Pool, nom, abrev, pf string) (int64, error) {
	var orgID int64
	slug := "populist-" + Slugify(nom)
	err := pool.QueryRow(ctx, `
		INSERT INTO core.organization (slug, kind, name, short_name)
		VALUES ($1,'PARTY',$2,NULLIF($3,''))
		ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
		RETURNING id`, slug, nom, abrev).Scan(&orgID)
	if err != nil {
		return 0, err
	}
	if pf != "" {
		// Party Facts : la table de correspondance entre référentiels. C'est
		// par elle qu'on rapproche, jamais par le nom.
		_, _ = pool.Exec(ctx, `
			INSERT INTO core.organization_identifier (organization_id, scheme, value)
			VALUES ($1,'PARTYFACTS',$2) ON CONFLICT (scheme, value) DO NOTHING`, orgID, pf)
	}
	return orgID, nil
}

// PopuList borne ses catégories par des années, et emploie 1900 et 2100 comme
// sentinelles pour « depuis toujours » et « encore en cours ». Elles sont
// stockées telles quelles et interprétées à l'affichage : les remplacer par NULL
// ferait passer une borne ouverte pour une absence d'information.
func yearOf(s string) any {
	s = strings.TrimSpace(s)
	if len(s) >= 4 {
		if y, err := strconv.Atoi(s[:4]); err == nil && y >= 1900 && y <= 2100 {
			return y
		}
	}
	return nil
}
