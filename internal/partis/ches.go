package partis

import (
	"context"
	"encoding/csv"
	"os"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CHES publie des scores continus issus d'une enquête auprès d'experts. Comme
// PopuList, c'est une CLASSIFICATION, pas un fait.
//
// Particularité juridique : le site ne publie AUCUNE licence explicite,
// seulement une obligation de citation. Une absence de licence n'est pas une
// autorisation : la source est donc enregistrée en RESTRICTED. Elle est
// affichée sur le site, mais n'entre pas dans raw.source_redistribuable et ne
// doit jamais être reversée dans un export ouvert.
var SourceCHES = archive.Source{
	Slug: "ches-2024", Label: "Chapel Hill Expert Survey 2024",
	Publisher: "Chapel Hill Expert Survey", Tier: "SECONDARY_PRESS",
	Licence:     "Aucune licence explicite publiée ; citation exigée",
	ReuseClass:  "RESTRICTED",
	Attribution: "Source : 2024 Chapel Hill Expert Survey (Jolly, Bakker, Hooghe, Marks, Polk, Rovny, Steenbergen, Vachudova)",
	Cadence:     "par vague, environ tous les quatre ans",
	Notes: "Scores continus de 0 à 10, moyennes d'appréciations d'experts. " +
		"Absence de licence explicite : à ne pas reverser dans un export ouvert.",
}

const CHESURL = "https://github.com/chesdata/chesdata.github.io/releases/download/ches-europe/CHES_2024_final_v2.csv"

// Code pays CHES de la France.
const chesFrance = "6"

// Les dimensions retenues. CHES en publie davantage ; n'en exposer que cinq est
// un choix, donc il est explicite.
var chesDimensions = []string{"lrgen", "lrecon", "galtan", "eu_position", "immigrate_policy"}

func IngestCHES(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceCHES)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersion)
	if err != nil {
		return err
	}
	f, err := arch.Fetch(ctx, srcID, runID, CHESURL, ".csv")
	if err != nil {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	var setID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO ref.classification_set (slug, provider, version, label, vocabulary, source_id)
		VALUES ('ches-2024','CHES','2024','Chapel Hill Expert Survey 2024',
		        'scores continus de 0 à 10 : lrgen, lrecon, galtan, eu_position, immigrate_policy — moyennes d''appréciations d''experts',
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

	nParties, nRows := 0, 0
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		if get(rec, "country") != chesFrance {
			continue
		}
		abrev := get(rec, "party")
		if abrev == "" {
			continue
		}
		orgID, err := upsertCHESParty(ctx, pool, abrev, get(rec, "party_id"))
		if err != nil {
			return err
		}
		nParties++
		for _, dim := range chesDimensions {
			v := get(rec, dim)
			if v == "" {
				continue // non renseigné : jamais converti en zéro
			}
			val, err := strconv.ParseFloat(v, 64)
			if err != nil {
				continue
			}
			if _, err := pool.Exec(ctx, `
				INSERT INTO core.party_classification
				  (classification_set_id, party_id, dimension, value, period_year)
				VALUES ($1,$2,$3,$4,2024)
				ON CONFLICT DO NOTHING`, setID, orgID, dim, val); err != nil {
				return err
			}
			nRows++
		}
	}
	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"partis_france": nParties, "scores": nRows}, "")
	logs.Notice("CHES 2024 normalisé", "partis_france", nParties, "scores", nRows, "reuse_class", "RESTRICTED")
	return nil
}

// CHES ne publie que l'abréviation du parti. Le rapprochement avec une
// organisation française est une DÉCISION, prise dans data/organisations.csv ;
// ici on ne crée que l'entrée du référentiel, nommée comme lui la nomme.
func upsertCHESParty(ctx context.Context, pool *pgxpool.Pool, abrev, chesID string) (int64, error) {
	var orgID int64
	err := pool.QueryRow(ctx, `
		INSERT INTO core.organization (slug, kind, name, short_name)
		VALUES ($1,'PARTY',$2,$2)
		ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
		RETURNING id`, "ches-"+Slugify(abrev), abrev).Scan(&orgID)
	if err != nil {
		return 0, err
	}
	if chesID != "" {
		_, _ = pool.Exec(ctx, `
			INSERT INTO core.organization_identifier (organization_id, scheme, value)
			VALUES ($1,'CHES',$2) ON CONFLICT (scheme, value) DO NOTHING`, orgID, chesID)
	}
	return orgID, nil
}
