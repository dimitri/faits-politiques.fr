package dette

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Qui détient la dette de l'État : la Banque de France publie, chaque
// trimestre depuis 2008, la détention des titres négociables (OAT, BTF) par
// secteur résident et par les non-résidents (jeu DET2). C'est la seule source
// qui répond à « à qui emprunte-t-on ? » ; l'AFT n'en publie qu'une synthèse.
var SourceBanqueDeFrance = archive.Source{
	Slug: "bdf-webstat-det2", Label: "Banque de France — détention des titres négociables de l'État (DET2)",
	Publisher: "Banque de France", Tier: "PRIMARY_OFFICIAL",
	License:     "Conditions d'utilisation Webstat : réutilisation libre avec mention de la source",
	ReuseClass:  "ATTRIBUTION",
	Attribution: "Source : Banque de France, Webstat, détention des titres de l'État (DET2)",
	Cadence:     "trimestrielle",
	Notes: "Encours en VALEUR DE MARCHÉ, pas en nominal : ils s'écartent de la dette négociable " +
		"de l'AFT dès que les taux bougent (au-dessus en 2019-2021, taux bas ; en dessous " +
		"depuis 2022). Comparer des PARTS, jamais des montants, avec l'INSEE/AFT. " +
		"« Non-résident » = domicile du détenteur, pas sa nationalité : une banque française " +
		"logée au Luxembourg est non résidente. L'OAT inclut les OATi et OAT€i. " +
		"Accès par clé d'API (en-tête HTTP, jamais dans l'URL).",
}

const webstatBase = "https://webstat.banque-france.fr/api/explore/v2.1/catalog/datasets/"

// Positions dans la clé SDMX DET2 (DET2.Q.N.FR.W1.S13111.S1.N.L.LE.F3.L._Z.XDC._T.M.V.N._T).
const (
	det2Freq        = 1
	det2Zone        = 4
	det2Emetteur    = 5
	det2Detenteur   = 6
	det2Instr       = 10
	det2Maturite    = 11
	det2Unite       = 13
	det2Ventilation = 18
)

var det2Secteurs = map[string]bool{
	"S1": true, "S11": true, "S12": true, "S121": true, "S122": true, "S123": true, "S124": true,
	"S125": true, "S126": true, "S127": true, "S128": true, "S129": true, "S12K": true, "S12P": true,
	"S12Q": true, "S13": true, "S14": true, "S15": true, "S1M": true, "_Z": true,
}

type webstatSeries struct {
	SeriesKey string          `json:"series_key"`
	Info      json.RawMessage `json:"series_info"`
}

type webstatObs struct {
	SeriesKey string   `json:"series_key"`
	Periode   string   `json:"time_period"`
	Valeur    *float64 `json:"obs_value"`
	Statut    string   `json:"obs_status"`
	Supprime  *string  `json:"deleted_at"`
}

func IngestBanqueDeFrance(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	key := os.Getenv("WEBSTAT_API_KEY")
	if key == "" {
		fmt.Println("  bdf-webstat-det2 : WEBSTAT_API_KEY absente, détention non chargée")
		return nil
	}
	headers := http.Header{"Authorization": {"Apikey " + key}}

	return run(ctx, pool, arch, SourceBanqueDeFrance, func(srcID, runID int64) (*batch, error) {
		// Un export trié : à données inchangées, mêmes octets, même document.
		q := url.Values{"where": {`dataset_id="DET2"`}, "order_by": {"series_key"}}
		urlSeries := webstatBase + "series/exports/json?" + q.Encode()
		f, err := arch.FetchEntetes(ctx, srcID, runID, urlSeries, ".json", headers)
		if err != nil {
			return nil, err
		}
		var series []webstatSeries
		if err := readJSON(f.Path, &series); err != nil {
			return nil, fmt.Errorf("séries : %w", err)
		}

		q = url.Values{
			"where":    {`dataset_id="DET2"`},
			"select":   {"series_key,time_period,obs_value,obs_status,deleted_at"},
			"order_by": {"series_key,time_period"},
		}
		urlObs := webstatBase + "observations/exports/json?" + q.Encode()
		fo, err := arch.FetchEntetes(ctx, srcID, runID, urlObs, ".json", headers)
		if err != nil {
			return nil, err
		}
		var observations []webstatObs
		if err := readJSON(fo.Path, &observations); err != nil {
			return nil, fmt.Errorf("observations : %w", err)
		}
		bySeries := map[string][]Obs{}
		for _, o := range observations {
			if o.Valeur == nil || o.Supprime != nil {
				continue
			}
			bySeries[o.SeriesKey] = append(bySeries[o.SeriesKey],
				Obs{Periode: o.Periode, Valeur: *o.Valeur, Statut: o.Statut, DocumentID: fo.DocumentID})
		}

		l := newBatch()
		for _, ws := range series {
			var info map[string]string
			if err := decodeInfo(ws.Info, &info); err != nil {
				return nil, fmt.Errorf("%s : %w", ws.SeriesKey, err)
			}
			s, mult, keep, err := classifyDET2(ws.SeriesKey, info)
			if err != nil {
				return nil, err
			}
			if !keep {
				continue
			}
			s.URL = urlObs
			obs := bySeries[ws.SeriesKey]
			for i := range obs {
				obs[i].Valeur *= mult
			}
			l.add(s, obs)
		}
		return l, nil
	})
}

// classifyDET2 traduit une clé SDMX en dimensions du modèle. Toute valeur
// de dimension non prévue fait échouer : mieux vaut un chargement en échec
// qu'une série mal rangée qui fausse une part.
func classifyDET2(key string, info map[string]string) (*Series, float64, bool, error) {
	p := strings.Split(key, ".")
	if len(p) != 19 || p[0] != "DET2" {
		return nil, 0, false, fmt.Errorf("clé DET2 inattendue : %s", key)
	}
	// Une seule série hors sujet dans le jeu : la part du capital du CAC 40
	// détenue par les non-résidents. Rien à voir avec la dette.
	if p[det2Ventilation] == "FR_CAC" {
		return nil, 0, false, nil
	}
	if p[det2Freq] != "Q" || p[det2Emetteur] != "S13111" || p[det2Instr] != "F3" {
		return nil, 0, false, fmt.Errorf("%s : fréquence, émetteur ou instrument inattendu", key)
	}
	s := &Series{
		Code: "bdf:" + strings.TrimPrefix(key, "DET2."), CodeSource: key,
		Libelle: info["TITLE"], Pays: "FR", Frequence: "Q",
		Concept: "DETENTION_TITRES_ETAT", SecteurEmetteur: "S13111",
	}
	zone := p[det2Zone]
	switch zone {
	case "W0", "W1", "W2":
		s.ZoneDetenteur = zone
	default:
		return nil, 0, false, fmt.Errorf("%s : zone %q inattendue", key, zone)
	}
	sector := p[det2Detenteur]
	if !det2Secteurs[sector] {
		return nil, 0, false, fmt.Errorf("%s : secteur détenteur %q inattendu", key, sector)
	}
	// S1 est ici « tous secteurs » de la zone : on le ramène à la convention
	// du modèle, où le total d'une dimension vaut '_T'.
	if sector == "S1" {
		sector = "_T"
	}
	s.SecteurDetenteur = sector

	switch p[det2Maturite] {
	case "T":
	case "L":
		s.Echeance, s.BaseEcheance = "LT", "INITIALE"
	case "S":
		s.Echeance, s.BaseEcheance = "CT", "INITIALE"
	case "Y25":
		// Les BTAN (2 à 5 ans), émis jusqu'en 2013 puis fondus dans les OAT.
		s.Echeance, s.BaseEcheance = "2A5", "INITIALE"
	default:
		return nil, 0, false, fmt.Errorf("%s : maturité %q inattendue", key, p[det2Maturite])
	}
	switch p[det2Ventilation] {
	case "_T":
	case "FR_OAT":
		s.Instrument = "OAT"
	case "FR_OATI":
		s.Instrument = "OATI"
	case "FR_OATIE":
		s.Instrument = "OATEI"
	case "FR_BT":
		if s.Echeance == "2A5" {
			s.Instrument = "BTAN"
		} else {
			s.Instrument = "BTF"
		}
	default:
		return nil, 0, false, fmt.Errorf("%s : ventilation %q inattendue", key, p[det2Ventilation])
	}
	mult := 1.0
	switch p[det2Unite] {
	case "XDC":
		if info["UNIT"] != "EUR" {
			return nil, 0, false, fmt.Errorf("%s : unité %q, attendu EUR", key, info["UNIT"])
		}
		m, err := multiplier(info["UNIT_MULT"])
		if err != nil {
			return nil, 0, false, fmt.Errorf("%s : %w", key, err)
		}
		s.Unite, s.Mesure, mult = "EUR", "ENCOURS", m
	case "PT":
		s.Unite, s.Mesure = "PCT", "PART"
	default:
		return nil, 0, false, fmt.Errorf("%s : unité %q inattendue", key, p[det2Unite])
	}
	return s, mult, true, nil
}

// decodeInfo accepte series_info sous forme d'objet ou de chaîne JSON :
// l'API renvoie l'un ou l'autre selon le point d'accès.
func decodeInfo(raw json.RawMessage, dst *map[string]string) error {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return json.Unmarshal([]byte(s), dst)
	}
	return json.Unmarshal(raw, dst)
}

func readJSON(path string, dst any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}
