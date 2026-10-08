package dette

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Le FMI est la seule source qui mesure la dette de la Suisse, du Royaume-Uni,
// des États-Unis ou du Japon avec la MÊME définition que celle de la France :
// Eurostat s'arrête à l'Espace économique européen et ne publie pas la dette
// suisse, et l'Administration fédérale des finances suit sa propre
// nomenclature. Pour comparer, c'est donc le FMI ; pour expliquer la Suisse
// de l'intérieur, l'AFF (suisse.go).
//
// Point d'accès : l'API SDMX du FMI, pas le DataMapper. Ce dernier renvoie
// 403 à tout client qui s'identifie honnêtement (règle du pare-feu sur le
// User-Agent) ; on ne se fait pas passer pour un navigateur. L'API SDMX a en
// outre ce que le DataMapper n'a pas : la dernière année OBSERVÉE de chaque
// pays, qui sépare les données des projections.
var SourceFMI = archive.Source{
	Slug: "fmi-weo-dette", Label: "FMI — World Economic Outlook, dette et solde des administrations publiques",
	Publisher: "Fonds monétaire international", Tier: "PRIMARY_OFFICIAL",
	License:     "Conditions d'utilisation du FMI (imf.org/external/terms.htm) : données réutilisables avec mention de la source ; usage commercial à confirmer",
	ReuseClass:  "ATTRIBUTION",
	Attribution: "Source : FMI, World Economic Outlook",
	Cadence:     "semestrielle (avril, octobre)",
	Notes: "Dette brute au sens du FMI (GFSM) : plus large que Maastricht, et en valeur faciale pour " +
		"la France. Seules les années jusqu'à LATEST_ACTUAL_ANNUAL_DATA (publiée pays par pays) " +
		"sont chargées : les projections du FMI n'entrent pas en base. La Suisse suit le GFSM " +
		"2001, la France le GFSM 2014 : écart de méthode mineur, signalé par le FMI lui-même. " +
		"Chine et Arabie saoudite ont un écart plus net, signalé de la même façon : la Chine " +
		"soumet sous une méthodologie que le FMI classe lui-même « Other », pas GFSM ; l'Arabie " +
		"saoudite sous un périmètre d'administration CENTRALE, pas d'administrations publiques " +
		"comme les autres pays de la série — deux chiffres à ne pas lire comme strictement " +
		"comparables aux douze autres, chargés quand même faute d'alternative ouverte.",
}

const fmiURL = "https://api.imf.org/external/sdmx/2.1/data/IMF.RES,WEO,/"

var fmiIndicators = map[string]string{
	"GGXWDG_NGDP":  "DETTE_BRUTE_FMI",
	"GGXWDN_NGDP":  "DETTE_NETTE_FMI",
	"GGXCNL_NGDP":  "SOLDE_PUBLIC",
	"GGXONLB_NGDP": "SOLDE_PRIMAIRE",
}

var fmiCountries = map[string]string{
	"CHE": "CH", "FRA": "FR", "DEU": "DE", "ITA": "IT", "ESP": "ES", "NLD": "NL", "BEL": "BE",
	"AUT": "AT", "SWE": "SE", "DNK": "DK", "NOR": "NO", "GBR": "GB", "USA": "US", "JPN": "JP",
	// Chine et Arabie saoudite ne suivent pas la même méthodologie que les
	// onze pays ci-dessus (docs/international-donnees.md § 6) : la Chine
	// soumet sous METHODOLOGY="Other" (pas GFSM 2014), l'Arabie saoudite sous
	// un périmètre d'ADMINISTRATION CENTRALE, pas d'administrations
	// publiques — chargées quand même, avec la réserve explicite plutôt que
	// tues, parce qu'aucune autre source ouverte ne les couvre du tout.
	"CHN": "CN", "RUS": "RU", "SAU": "SA",
}

type fmiMessage struct {
	Groups []struct {
		Country   string `xml:"COUNTRY,attr"`
		Indicator string `xml:"INDICATOR,attr"`
		Latest    string `xml:"LATEST_ACTUAL_ANNUAL_DATA,attr"`
	} `xml:"DataSet>Group"`
	Series []struct {
		Country   string `xml:"COUNTRY,attr"`
		Indicator string `xml:"INDICATOR,attr"`
		Frequency string `xml:"FREQUENCY,attr"`
		Scale     string `xml:"SCALE,attr"`
		Obs       []struct {
			Period string `xml:"TIME_PERIOD,attr"`
			Value  string `xml:"OBS_VALUE,attr"`
		} `xml:"Obs"`
	} `xml:"DataSet>Series"`
}

func IngestFMI(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	return run(ctx, pool, arch, SourceFMI, func(srcID, runID int64) (*batch, error) {
		countries := keys(fmiCountries)
		inds := keys(fmiIndicators)
		url := fmiURL + strings.Join(countries, "+") + "." + strings.Join(inds, "+") + ".A"
		f, err := arch.Fetch(ctx, srcID, runID, url, ".xml")
		if err != nil {
			return nil, err
		}
		b, err := os.ReadFile(f.Path)
		if err != nil {
			return nil, err
		}
		var m fmiMessage
		if err := xml.Unmarshal(b, &m); err != nil {
			return nil, fmt.Errorf("réponse SDMX illisible : %w", err)
		}
		latest := map[string]int{}
		for _, g := range m.Groups {
			if g.Country == "" || g.Latest == "" {
				continue
			}
			a, err := strconv.Atoi(g.Latest)
			if err != nil {
				return nil, fmt.Errorf("%s/%s : dernière année %q illisible", g.Country, g.Indicator, g.Latest)
			}
			latest[g.Country+"|"+g.Indicator] = a
		}
		l := newBatch()
		for _, s := range m.Series {
			concept, ok := fmiIndicators[s.Indicator]
			if !ok || fmiCountries[s.Country] == "" {
				return nil, fmt.Errorf("série inattendue : %s/%s", s.Country, s.Indicator)
			}
			if s.Frequency != "A" || (s.Scale != "" && s.Scale != "0") {
				return nil, fmt.Errorf("%s/%s : fréquence %q ou échelle %q inattendue", s.Country, s.Indicator, s.Frequency, s.Scale)
			}
			// Sans dernière année observée, impossible de séparer données et
			// projections : la série est écartée plutôt que chargée en bloc.
			latestYear, ok := latest[s.Country+"|"+s.Indicator]
			if !ok {
				return nil, fmt.Errorf("%s/%s : LATEST_ACTUAL_ANNUAL_DATA absente", s.Country, s.Indicator)
			}
			series := &Series{
				Code: "fmi:" + s.Indicator + ":" + s.Country, CodeSource: "WEO/" + s.Country + "." + s.Indicator + ".A",
				Libelle: s.Indicator + " (" + s.Country + ")", Pays: fmiCountries[s.Country], Frequence: "A",
				Unite: "PCT_PIB", Concept: concept, Mesure: "ENCOURS", SecteurEmetteur: "S13", URL: url,
				Notes: fmt.Sprintf("Dernière année observée selon le FMI : %d", latestYear),
			}
			if concept == "SOLDE_PUBLIC" || concept == "SOLDE_PRIMAIRE" {
				series.Mesure = "FLUX"
			}
			var obs []Obs
			for _, o := range s.Obs {
				a, err := strconv.Atoi(o.Period)
				if err != nil || a > latestYear {
					continue
				}
				v, err := strconv.ParseFloat(o.Value, 64)
				if err != nil {
					continue
				}
				obs = append(obs, Obs{Periode: o.Period, Valeur: v, DocumentID: f.DocumentID})
			}
			l.add(series, obs)
		}
		return l, nil
	})
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
