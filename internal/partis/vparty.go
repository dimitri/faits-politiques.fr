package partis

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/jackc/pgx/v5/pgxpool"
)

// V-Party (Varieties of Party Identity and Organization) est, comme CHES, une
// CLASSIFICATION : des jugements d'experts agrégés par un modèle de réponse à
// l'item, pas des faits. Elle couvre 178 pays de 1970 à 2019 avec les mêmes
// questions, ce que CHES (Europe seulement) ne fait pas. Pour la France, la
// dernière élection codée est la présidentielle de 2017 : ni le Rassemblement
// national sous ce nom, ni Reconquête, ni Horizons n'y figurent. Ces valeurs
// décrivent les partis tels qu'ils étaient en 2017.
//
// Les échelles ne sont pas celles de CHES (0 à 10) : les questions à réponses
// ordonnées (immigration, pluralisme, violence…) sont des échelles de variable
// latente, centrées sur zéro, dont l'unité n'a de sens que par comparaison ; seuls
// les indices (anti-pluralisme, populisme) sont bornés de 0 à 1.
//
// Particularité juridique : le codebook porte « Copyright © University of
// Gothenburg, V-Dem Institute. All rights reserved » et une citation suggérée ;
// aucune licence pour ce jeu-ci. Comme CHES, la source est RESTRICTED :
// affichée, jamais reversée dans un export ouvert.
var SourceVParty = archive.Source{
	Slug: "vparty-v2", Label: "V-Party v2 (Varieties of Party Identity and Organization)",
	Publisher: "V-Dem Institute (Université de Göteborg)", Tier: "SECONDARY_PRESS",
	License:     "Copyright Université de Göteborg, aucune licence explicite pour ce jeu ; citation exigée",
	ReuseClass:  "RESTRICTED",
	Attribution: "Source : Lindberg et al., Varieties of Party Identity and Organization (V-Party) Dataset V2, V-Dem Project, 2022, https://doi.org/10.23696/vpartydsv2",
	Cadence:     "irrégulière (v2 : février 2022, couvre 1970-2019)",
	Notes: "Jugements d'experts agrégés (modèle de réponse à l'item), 178 pays. Pour la France, dernière élection codée : 2017. " +
		"Absence de licence explicite : à ne pas reverser dans un export ouvert.",
}

// L'institut V-Dem ne publie le jeu en CSV que derrière un formulaire (adresse
// électronique, acceptation de ses conditions). Son dépôt public vdemdata en
// contient la même table au format R, sans formulaire. L'adresse est figée sur le
// commit qui a produit la v2 (mars 2022) : une branche qui bouge changerait le
// contenu d'un relevé sans que l'empreinte du document archivé le dise.
const VPartyURL = "https://raw.githubusercontent.com/vdeminstitute/vdemdata/cf87496460cd6eff8ef534b9d7f39e20dc558d23/data/vparty.RData"

func VPartyDownloadTargets() []archive.DownloadTarget {
	return []archive.DownloadTarget{{Name: "vparty", Source: SourceVParty, URL: VPartyURL, Ext: ".RData"}}
}

// vpartyDimensions : les variables retenues, sous le nom que V-Party leur donne.
// Les quatre premières composent l'indice d'anti-pluralisme (codebook § 2.1).
var vpartyDimensions = []string{
	"v2xpa_antiplural", "v2xpa_popul",
	"v2paopresp", "v2paplur", "v2paminor", "v2paviol",
	"v2pariglef", "v2paimmig", "v2paculsup",
}

const vpartyFrance = "FRA"

// vpartyDepuis : les élections codées avant 2002 concernent des partis (RPR, MRP,
// UNR…) qu'aucune page du site n'utilise ; ne charger que les élections qui
// portent les partis encore existants évite d'ajouter trente organisations mortes
// au référentiel pour rien. Les remonter est une constante à changer.
const vpartyDepuis = 2002

func IngestVParty(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceVParty)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersion)
	if err != nil {
		return err
	}
	f, err := arch.Fetch(ctx, srcID, runID, VPartyURL, ".RData")
	if err != nil {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}
	fh, err := os.Open(f.Path)
	if err != nil {
		return err
	}
	defer fh.Close()
	tab, err := ReadRData(fh)
	if err != nil {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return fmt.Errorf("V-Party : %w", err)
	}
	for _, c := range append([]string{"country_text_id", "year", "v2paid", "v2pashname", "v2paenname"}, vpartyDimensions...) {
		if tab.Column(c) == nil {
			err := fmt.Errorf("V-Party : colonne %q absente", c)
			arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
			return err
		}
	}

	var setID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO ref.classification_set (slug, provider, version, label, vocabulary, source_id)
		VALUES ('vparty-v2','V-Party','2','V-Party v2 (1970-2019)',
		        'indices de 0 à 1 (v2xpa_antiplural, v2xpa_popul) et échelles de variable latente centrées sur zéro (v2paopresp, v2paplur, v2paminor, v2paviol, v2pariglef, v2paimmig, v2paculsup) — jugements d''experts agrégés',
		        $1)
		ON CONFLICT (provider, version) DO UPDATE SET label = EXCLUDED.label, vocabulary = EXCLUDED.vocabulary
		RETURNING id`, srcID).Scan(&setID); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx,
		`DELETE FROM core.party_classification WHERE classification_set_id = $1`, setID); err != nil {
		return err
	}

	country, year := tab.Column("country_text_id"), tab.Column("year")
	nParties, nRows := map[string]bool{}, 0
	for i := 0; i < tab.Rows; i++ {
		if country[i] != vpartyFrance {
			continue
		}
		y, err := strconv.ParseFloat(year[i], 64)
		if err != nil || int(y) < vpartyDepuis {
			continue
		}
		abbr := tab.Column("v2pashname")[i]
		if abbr == "" {
			continue
		}
		orgID, err := upsertVPartyParty(ctx, pool, abbr, tab.Column("v2paid")[i])
		if err != nil {
			return err
		}
		nParties[abbr] = true
		for _, dim := range vpartyDimensions {
			v := tab.Column(dim)[i]
			if v == "" {
				continue // non codé : jamais converti en zéro
			}
			val, err := strconv.ParseFloat(v, 64)
			if err != nil {
				continue
			}
			if _, err := pool.Exec(ctx, `
				INSERT INTO core.party_classification
				  (classification_set_id, party_id, dimension, value, period_year)
				VALUES ($1,$2,$3,$4,$5)
				ON CONFLICT DO NOTHING`, setID, orgID, dim, val, int(y)); err != nil {
				return err
			}
			nRows++
		}
	}
	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"partis_france": len(nParties), "scores": nRows}, "")
	logs.Notice(fmt.Sprintf("V-Party v2: %s, %s (source RESTRICTED)",
		logs.Plural(len(nParties), "French party"), logs.Plural(nRows, "score")))
	return nil
}

// V-Party ne publie que l'abréviation du parti (et son nom anglais). Le
// rapprochement avec une organisation française est une DÉCISION, prise dans
// data/organisations.csv (colonne vparty_nom) ; ici on ne crée que l'entrée du
// référentiel, nommée comme lui la nomme.
func upsertVPartyParty(ctx context.Context, pool *pgxpool.Pool, abbr, id string) (int64, error) {
	var orgID int64
	err := pool.QueryRow(ctx, `
		INSERT INTO core.organization (slug, kind, name, short_name)
		VALUES ($1,'PARTY',$2,$2)
		ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
		RETURNING id`, "vparty-"+Slugify(abbr)+"-"+id, abbr).Scan(&orgID)
	if err != nil {
		return 0, err
	}
	if id != "" {
		_, _ = pool.Exec(ctx, `
			INSERT INTO core.organization_identifier (organization_id, scheme, value)
			VALUES ($1,'VPARTY',$2) ON CONFLICT (scheme, value) DO NOTHING`, orgID, id)
	}
	return orgID, nil
}
