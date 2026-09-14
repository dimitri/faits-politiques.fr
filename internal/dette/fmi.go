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
	Licence:     "Conditions d'utilisation du FMI (imf.org/external/terms.htm) : données réutilisables avec mention de la source ; usage commercial à confirmer",
	ReuseClass:  "ATTRIBUTION",
	Attribution: "Source : FMI, World Economic Outlook",
	Cadence:     "semestrielle (avril, octobre)",
	Notes: "Dette brute au sens du FMI (GFSM) : plus large que Maastricht, et en valeur faciale pour " +
		"la France. Seules les années jusqu'à LATEST_ACTUAL_ANNUAL_DATA (publiée pays par pays) " +
		"sont chargées : les projections du FMI n'entrent pas en base. La Suisse suit le GFSM " +
		"2001, la France le GFSM 2014 : écart de méthode mineur, signalé par le FMI lui-même.",
}

const fmiURL = "https://api.imf.org/external/sdmx/2.1/data/IMF.RES,WEO,/"

var fmiIndicateurs = map[string]string{
	"GGXWDG_NGDP":  "DETTE_BRUTE_FMI",
	"GGXWDN_NGDP":  "DETTE_NETTE_FMI",
	"GGXCNL_NGDP":  "SOLDE_PUBLIC",
	"GGXONLB_NGDP": "SOLDE_PRIMAIRE",
}

var fmiPays = map[string]string{
	"CHE": "CH", "FRA": "FR", "DEU": "DE", "ITA": "IT", "ESP": "ES", "NLD": "NL", "BEL": "BE",
	"AUT": "AT", "SWE": "SE", "DNK": "DK", "NOR": "NO", "GBR": "GB", "USA": "US", "JPN": "JP",
}

type fmiMessage struct {
	Groupes []struct {
		Pays       string `xml:"COUNTRY,attr"`
		Indicateur string `xml:"INDICATOR,attr"`
		Derniere   string `xml:"LATEST_ACTUAL_ANNUAL_DATA,attr"`
	} `xml:"DataSet>Group"`
	Series []struct {
		Pays       string `xml:"COUNTRY,attr"`
		Indicateur string `xml:"INDICATOR,attr"`
		Frequence  string `xml:"FREQUENCY,attr"`
		Echelle    string `xml:"SCALE,attr"`
		Obs        []struct {
			Periode string `xml:"TIME_PERIOD,attr"`
			Valeur  string `xml:"OBS_VALUE,attr"`
		} `xml:"Obs"`
	} `xml:"DataSet>Series"`
}

func IngestFMI(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	return executer(ctx, pool, arch, SourceFMI, func(srcID, runID int64) (*lot, error) {
		pays := cles(fmiPays)
		inds := cles(fmiIndicateurs)
		url := fmiURL + strings.Join(pays, "+") + "." + strings.Join(inds, "+") + ".A"
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
		derniere := map[string]int{}
		for _, g := range m.Groupes {
			if g.Pays == "" || g.Derniere == "" {
				continue
			}
			a, err := strconv.Atoi(g.Derniere)
			if err != nil {
				return nil, fmt.Errorf("%s/%s : dernière année %q illisible", g.Pays, g.Indicateur, g.Derniere)
			}
			derniere[g.Pays+"|"+g.Indicateur] = a
		}
		l := nouveauLot()
		for _, s := range m.Series {
			concept, ok := fmiIndicateurs[s.Indicateur]
			if !ok || fmiPays[s.Pays] == "" {
				return nil, fmt.Errorf("série inattendue : %s/%s", s.Pays, s.Indicateur)
			}
			if s.Frequence != "A" || (s.Echelle != "" && s.Echelle != "0") {
				return nil, fmt.Errorf("%s/%s : fréquence %q ou échelle %q inattendue", s.Pays, s.Indicateur, s.Frequence, s.Echelle)
			}
			// Sans dernière année observée, impossible de séparer données et
			// projections : la série est écartée plutôt que chargée en bloc.
			fin, ok := derniere[s.Pays+"|"+s.Indicateur]
			if !ok {
				return nil, fmt.Errorf("%s/%s : LATEST_ACTUAL_ANNUAL_DATA absente", s.Pays, s.Indicateur)
			}
			serie := &Serie{
				Code: "fmi:" + s.Indicateur + ":" + s.Pays, CodeSource: "WEO/" + s.Pays + "." + s.Indicateur + ".A",
				Libelle: s.Indicateur + " (" + s.Pays + ")", Pays: fmiPays[s.Pays], Frequence: "A",
				Unite: "PCT_PIB", Concept: concept, Mesure: "ENCOURS", SecteurEmetteur: "S13", URL: url,
				Notes: fmt.Sprintf("Dernière année observée selon le FMI : %d", fin),
			}
			if concept == "SOLDE_PUBLIC" || concept == "SOLDE_PRIMAIRE" {
				serie.Mesure = "FLUX"
			}
			var obs []Obs
			for _, o := range s.Obs {
				a, err := strconv.Atoi(o.Periode)
				if err != nil || a > fin {
					continue
				}
				v, err := strconv.ParseFloat(o.Valeur, 64)
				if err != nil {
					continue
				}
				obs = append(obs, Obs{Periode: o.Periode, Valeur: v, DocumentID: f.DocumentID})
			}
			l.ajouter(serie, obs)
		}
		return l, nil
	})
}

func cles[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
