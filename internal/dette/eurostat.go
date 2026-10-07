package dette

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Eurostat donne la comparaison européenne dans des définitions harmonisées
// (SEC 2010, notification de déficit excessif) : la dette « Maastricht » de
// la France y est celle de l'INSEE, et celle de l'Italie est mesurée de la
// même façon. C'est aussi la seule source qui ventile la dette de chaque pays
// par détenteur et par échéance résiduelle, et qui publie la charge
// d'intérêts de la Suisse aux côtés de celle des pays de l'Union.
var SourceEurostatDette = archive.Source{
	Slug: "eurostat-dette", Label: "Eurostat — dette, déficit, intérêts et taux longs des États européens",
	Publisher: "Eurostat", Tier: "PRIMARY_OFFICIAL",
	Licence: "Creative Commons Attribution 4.0 (CC BY 4.0)", ReuseClass: "ATTRIBUTION",
	Attribution: "Source : Eurostat (gov_10dd_edpt1, gov_10a_main, gov_10dd_ggd, irt_lt_mcby)",
	Cadence:     "annuelle (notifications d'avril et octobre), mensuelle pour les taux",
	Notes: "Échéances de gov_10dd_ggd en durée RÉSIDUELLE, à ne pas rapprocher du court/long " +
		"terme de l'INSEE (durée initiale). Taux irt_lt_mcby : critère de convergence de " +
		"Maastricht, rendement des emprunts d'État à environ 10 ans — pas le taux payé sur la " +
		"dette. La Suisse n'est présente que dans gov_10a_main (hors notification de déficit).",
}

const eurostatBase = "https://ec.europa.eu/eurostat/api/dissemination/statistics/1.0/data/"

// Les pays de comparaison : les grandes économies de la zone euro, les pays
// « frugaux » souvent cités en contre-exemple, les pays du Sud exposés à la
// crise de 2010-2012, et deux agrégats. Pas les 27 : ce projet ne synthétise
// pas un inventaire pays par pays.
var comparisonCountries = []string{
	"FR", "DE", "IT", "ES", "NL", "BE", "AT", "PT", "EL", "IE", "FI", "SE", "DK", "PL",
}

type eurostatRequest struct {
	jeu      string
	params   string
	geo      []string
	classify func(dims map[string]string) (*Series, error)
}

func eurostatRequests() []eurostatRequest {
	aggregates := append(append([]string{}, comparisonCountries...), "EU27_2020", "EA20")
	return []eurostatRequest{
		{
			jeu:    "gov_10dd_edpt1",
			params: "na_item=GD&sector=S13&sector=S1311&sector=S1312&sector=S1313&sector=S1314&unit=MIO_EUR&unit=PC_GDP",
			geo:    aggregates,
			classify: func(d map[string]string) (*Series, error) {
				return &Series{Concept: "DETTE_MAASTRICHT", Mesure: "ENCOURS", SecteurEmetteur: d["sector"]}, nil
			},
		},
		{
			jeu:      "gov_10a_main",
			params:   "na_item=D41PAY&na_item=B9&na_item=TR&na_item=TE&sector=S13&unit=MIO_EUR&unit=PC_GDP",
			geo:      append(append([]string{}, aggregates...), "CH"),
			classify: classifyAccounts,
		},
		{
			// Les montants de la Suisse en francs : convertis en euros par
			// Eurostat, ils varieraient avec le change autant qu'avec la dette.
			jeu:      "gov_10a_main",
			params:   "na_item=D41PAY&na_item=B9&na_item=TR&na_item=TE&sector=S13&unit=MIO_NAC",
			geo:      []string{"CH"},
			classify: classifyAccounts,
		},
		// Le compte des administrations françaises, opération par opération :
		// la dépense par nature (dont la somme redonne TE au million près) et
		// le compte de capital (épargne brute, investissement, transferts en
		// capital), qui décompose exactement le besoin de financement.
		// B9, TE, TR et D41PAY du secteur S13 sont déjà dans la requête
		// précédente : les redemander chargerait deux fois les mêmes séries.
		{
			jeu:      "gov_10a_main",
			params:   "sector=S13&unit=MIO_EUR&unit=PC_GDP" + queryParams("na_item", operationsAPU),
			geo:      []string{"FR"},
			classify: classifyOperation,
		},
		{
			jeu: "gov_10a_main",
			params: "sector=S1311&sector=S1313&sector=S1314&unit=MIO_EUR&unit=PC_GDP" +
				queryParams("na_item", append([]string{"B9", "TE", "TR", "D41PAY"}, operationsAPU...)),
			geo:      []string{"FR"},
			classify: classifyOperation,
		},
		// Pour comparer la « règle d'or » d'un pays à l'autre : le compte de
		// capital seul, au niveau de l'ensemble des administrations.
		{
			jeu:      "gov_10a_main",
			params:   "sector=S13&unit=MIO_EUR&unit=PC_GDP" + queryParams("na_item", compteCapital),
			geo:      append(without(aggregates, "FR"), "CH"),
			classify: classifyOperation,
		},
		{
			jeu:      "gov_10dd_ggd",
			params:   "na_item=GD&sector=S13&maturity=TOTAL&unit=MIO_EUR&unit=PC_GDP",
			geo:      comparisonCountries,
			classify: classifyHolding,
		},
		{
			jeu: "gov_10dd_ggd",
			// L'échéance TOTAL est déjà dans la requête précédente : la
			// redemander chargerait deux fois les mêmes séries.
			params: "na_item=GD&sector=S13&sector2=S1_S2&maturity=Y_LE1&maturity=Y1-5&maturity=Y_GT1" +
				"&maturity=Y5-10&maturity=Y10-30&maturity=Y_GT30&unit=MIO_EUR&unit=PC_GDP",
			geo:      comparisonCountries,
			classify: classifyHolding,
		},
		{
			jeu:      "irt_lt_mcby_a",
			params:   "int_rt=MCBY",
			geo:      append(append([]string{}, comparisonCountries...), "EU27_2020"),
			classify: classifyRate,
		},
		{
			jeu:      "irt_lt_mcby_m",
			params:   "int_rt=MCBY",
			geo:      append(append([]string{}, comparisonCountries...), "EU27_2020"),
			classify: classifyRate,
		},
	}
}

// Les opérations du SEC retenues. Nature de la dépense : consommation
// intermédiaire (P2), rémunérations (D1), impôts sur la production (D29),
// subventions (D3), revenus de la propriété dont intérêts (D4), impôts
// courants (D5), prestations en espèces (D62) et en nature (D632), autres
// transferts courants (D7), ajustement des droits à pension (D8), transferts
// en capital (D9), formation de capital (P5) et acquisitions nettes d'actifs
// non produits (NP). Plus l'épargne, la consommation de capital fixe, les
// aides à l'investissement et les crédits d'impôt à payer.
var compteCapital = []string{"B8G", "P5", "NP", "D9PAY", "D9REC", "P51G", "P51C"}

var operationsAPU = append([]string{"B8N", "D92PAY", "P2", "D1PAY", "D29PAY", "D3PAY", "D4PAY",
	"D5PAY", "D62PAY", "D632PAY", "D7PAY", "D8", "PTC"}, compteCapital...)

func queryParams(name string, values []string) string {
	var b strings.Builder
	for _, v := range values {
		b.WriteString("&" + name + "=" + v)
	}
	return b.String()
}

func without(list []string, excluded string) []string {
	var out []string
	for _, x := range list {
		if x != excluded {
			out = append(out, x)
		}
	}
	return out
}

// classifyOperation range les opérations qui ont déjà un concept (solde,
// recettes, dépenses, intérêts) sous ce concept, et les autres sous
// OPERATION_APU, le code de l'opération dans instrument.
func classifyOperation(d map[string]string) (*Series, error) {
	if s, err := classifyAccounts(d); err == nil {
		return s, nil
	}
	for _, op := range operationsAPU {
		if d["na_item"] == op {
			return &Series{Concept: "OPERATION_APU", Mesure: "FLUX", SecteurEmetteur: d["sector"], Instrument: op}, nil
		}
	}
	return nil, fmt.Errorf("na_item %q inattendu", d["na_item"])
}

func classifyAccounts(d map[string]string) (*Series, error) {
	concepts := map[string]string{
		"D41PAY": "INTERETS_VERSES", "B9": "SOLDE_PUBLIC", "TR": "RECETTES_PUBLIQUES", "TE": "DEPENSES_PUBLIQUES",
	}
	c, ok := concepts[d["na_item"]]
	if !ok {
		return nil, fmt.Errorf("na_item %q inattendu", d["na_item"])
	}
	return &Series{Concept: c, Mesure: "FLUX", SecteurEmetteur: d["sector"]}, nil
}

// classifyHolding range la contrepartie (sector2) de gov_10dd_ggd dans
// les dimensions zone et secteur détenteur du modèle.
func classifyHolding(d map[string]string) (*Series, error) {
	s := &Series{Concept: "DETTE_MAASTRICHT", Mesure: "ENCOURS", SecteurEmetteur: d["sector"]}
	switch c := d["sector2"]; c {
	case "S1_S2":
		s.ZoneDetenteur, s.SecteurDetenteur = "W0", "_T"
	case "S2":
		s.ZoneDetenteur, s.SecteurDetenteur = "W1", "_T"
	case "S1":
		s.ZoneDetenteur, s.SecteurDetenteur = "W2", "_T"
	case "S11", "S12", "S121", "S122_S123", "S124-S129", "S11_S14_S15", "S14_S15",
		"S1311", "S1312", "S1313", "S1314":
		s.ZoneDetenteur, s.SecteurDetenteur = "W2", c
	default:
		return nil, fmt.Errorf("sector2 %q inattendu", c)
	}
	switch m := d["maturity"]; m {
	case "TOTAL":
	case "Y_LE1", "Y1-5", "Y_GT1", "Y5-10", "Y10-30", "Y_GT30":
		s.Echeance, s.BaseEcheance = m, "RESIDUELLE"
	default:
		return nil, fmt.Errorf("maturity %q inattendue", m)
	}
	return s, nil
}

func classifyRate(d map[string]string) (*Series, error) {
	return &Series{Concept: "TAUX_LONG_TERME", Mesure: "TAUX", SecteurEmetteur: "S1311", Unite: "PCT"}, nil
}

// jsonStat est la réponse JSON-stat 2.0 d'Eurostat. Les valeurs sont rangées
// dans un tableau à plat, indexé en ordre ligne-majeur sur les dimensions
// listées dans id (la dernière varie le plus vite) : l'index 0 n'est pas
// « la première année » mais la première combinaison de toutes les dimensions.
type jsonStat struct {
	Error []struct {
		Label string `json:"label"`
	} `json:"error"`
	ID        []string                     `json:"id"`
	Size      []int                        `json:"size"`
	Value     map[string]*float64          `json:"value"`
	Status    map[string]string            `json:"status"`
	Label     string                       `json:"label"`
	Dimension map[string]jsonStatDimension `json:"dimension"`
}

type jsonStatDimension struct {
	Label    string `json:"label"`
	Category struct {
		Index map[string]int    `json:"index"`
		Label map[string]string `json:"label"`
	} `json:"category"`
}

type cell struct {
	dims   map[string]string
	value  float64
	status string
}

// cells déplie le tableau à plat en combinaisons de dimensions nommées.
func (js *jsonStat) cells() ([]cell, error) {
	if len(js.ID) != len(js.Size) {
		return nil, fmt.Errorf("JSON-stat : %d dimensions pour %d tailles", len(js.ID), len(js.Size))
	}
	codes := make([][]string, len(js.ID))
	for i, id := range js.ID {
		dim, ok := js.Dimension[id]
		if !ok {
			return nil, fmt.Errorf("JSON-stat : dimension %q non décrite", id)
		}
		codes[i] = make([]string, js.Size[i])
		for code, pos := range dim.Category.Index {
			if pos < 0 || pos >= js.Size[i] {
				return nil, fmt.Errorf("JSON-stat : %s=%s hors bornes", id, code)
			}
			codes[i][pos] = code
		}
	}
	out := make([]cell, 0, len(js.Value))
	for key, v := range js.Value {
		// Un null décodé dans un float64 donnerait 0 : une valeur absente
		// deviendrait une valeur nulle. On l'écarte explicitement.
		if v == nil {
			continue
		}
		n, err := strconv.Atoi(key)
		if err != nil {
			return nil, fmt.Errorf("JSON-stat : index %q", key)
		}
		dims := make(map[string]string, len(js.ID))
		remainder := n
		for i := len(js.ID) - 1; i >= 0; i-- {
			dims[js.ID[i]] = codes[i][remainder%js.Size[i]]
			remainder /= js.Size[i]
		}
		if remainder != 0 {
			return nil, fmt.Errorf("JSON-stat : index %d hors du cube", n)
		}
		out = append(out, cell{dims: dims, value: *v, status: js.Status[key]})
	}
	return out, nil
}

func IngestEurostat(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	return run(ctx, pool, arch, SourceEurostatDette, func(srcID, runID int64) (*batch, error) {
		l := newBatch()
		series := map[string]*Series{}
		for _, r := range eurostatRequests() {
			url := eurostatBase + r.jeu + "?format=JSON&lang=FR&" + r.params
			for _, g := range r.geo {
				url += "&geo=" + g
			}
			f, err := arch.Fetch(ctx, srcID, runID, url, ".json")
			if err != nil {
				return nil, err
			}
			var js jsonStat
			if err := readJSON(f.Path, &js); err != nil {
				return nil, fmt.Errorf("%s : %w", r.jeu, err)
			}
			if len(js.Error) > 0 {
				return nil, fmt.Errorf("%s : Eurostat : %s", r.jeu, js.Error[0].Label)
			}
			cells, err := js.cells()
			if err != nil {
				return nil, fmt.Errorf("%s : %w", r.jeu, err)
			}
			if len(cells) == 0 {
				return nil, fmt.Errorf("%s : aucune valeur — requête ou dimension à revoir", r.jeu)
			}
			// Un ordre stable des séries et des observations, pour des
			// chargements comparables d'une exécution à l'autre.
			sort.Slice(cells, func(i, j int) bool { return cells[i].dims["time"] < cells[j].dims["time"] })
			obsBySeries := map[string][]Obs{}
			var order []string
			for _, c := range cells {
				code, libelle := eurostatCode(r.jeu, &js, c.dims)
				mult, err := eurostatFactor(c.dims)
				if err != nil {
					return nil, fmt.Errorf("%s : %w", code, err)
				}
				if _, ok := series[code]; !ok {
					s, err := r.classify(c.dims)
					if err != nil {
						return nil, fmt.Errorf("%s : %w", r.jeu, err)
					}
					s.Code, s.CodeSource, s.Libelle = code, strings.TrimPrefix(code, "eurostat:"), libelle
					s.Pays = c.dims["geo"]
					s.Frequence = c.dims["freq"]
					s.URL = url
					if err := eurostatUnit(s, c.dims); err != nil {
						return nil, fmt.Errorf("%s : %w", code, err)
					}
					series[code] = s
					order = append(order, code)
				}
				periode := eurostatPeriod(c.dims["time"])
				obsBySeries[code] = append(obsBySeries[code],
					Obs{Periode: periode, Valeur: c.value * mult, Statut: c.status, DocumentID: f.DocumentID})
			}
			sort.Strings(order)
			for _, code := range order {
				l.add(series[code], obsBySeries[code])
			}
		}
		return l, nil
	})
}

// eurostatCode nomme une série par son jeu, son pays et ses autres
// dimensions dans l'ordre de la réponse : 'eurostat:gov_10a_main:FR:MIO_EUR:S13:D41PAY'.
func eurostatCode(jeu string, js *jsonStat, dims map[string]string) (string, string) {
	parts := []string{"eurostat", jeu, dims["geo"]}
	var labels []string
	for _, id := range js.ID {
		if id == "freq" || id == "geo" || id == "time" {
			continue
		}
		parts = append(parts, dims[id])
		if l := js.Dimension[id].Category.Label[dims[id]]; l != "" {
			labels = append(labels, l)
		}
	}
	return strings.Join(parts, ":"), js.Label + " — " + strings.Join(labels, " — ")
}

func eurostatUnit(s *Series, dims map[string]string) error {
	u, ok := dims["unit"]
	if !ok {
		if s.Unite == "" {
			return fmt.Errorf("unité absente")
		}
		return nil
	}
	switch u {
	case "MIO_EUR":
		s.Unite = "EUR"
	case "MIO_NAC":
		// Seule la Suisse est demandée en monnaie nationale.
		if s.Pays != "CH" {
			return fmt.Errorf("monnaie nationale demandée hors Suisse (%s)", s.Pays)
		}
		s.Unite = "CHF"
	case "PC_GDP":
		s.Unite = "PCT_PIB"
	default:
		return fmt.Errorf("unité %q inattendue", u)
	}
	return nil
}

// eurostatFactor : les montants sont publiés en millions.
func eurostatFactor(dims map[string]string) (float64, error) {
	switch dims["unit"] {
	case "MIO_EUR", "MIO_NAC":
		return 1e6, nil
	case "PC_GDP", "":
		return 1, nil
	}
	return 0, fmt.Errorf("unité %q inattendue", dims["unit"])
}

// eurostatPeriod ramène '2025M01' ou '2025-01' à '2025-01', '2025Q1' à '2025-Q1'.
func eurostatPeriod(t string) string {
	switch {
	case len(t) == 7 && t[4] == 'M':
		return t[:4] + "-" + t[5:]
	case len(t) == 6 && t[4] == 'Q':
		return t[:4] + "-" + t[4:]
	}
	return t
}
