package dette

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"strconv"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Le site de l'Agence France Trésor (aft.gouv.fr) refuse tout accès
// automatisé derrière une protection anti-robot, qu'on ne contourne pas. Ses
// chiffres mensuels de dette négociable sont republiés, à l'identique et sous
// licence ouverte, par l'INSEE dans la BDM (jeu DETTE-NEGOCIABLE-ETAT, source
// déclarée : AFT). C'est la voie retenue : même donnée, producteur primaire
// cité, accès stable.
var SourceINSEE = archive.Source{
	Slug: "insee-dette", Label: "INSEE — dette négociable de l'État (AFT) et dette trimestrielle des APU",
	Publisher: "INSEE, d'après l'Agence France Trésor", Tier: "PRIMARY_OFFICIAL",
	License:     "Licence Ouverte v2.0",
	ReuseClass:  "OPEN",
	Attribution: "Source : INSEE, BDM ; Agence France Trésor pour la dette négociable de l'État",
	Cadence:     "mensuelle (dette négociable), trimestrielle (dette Maastricht)",
	Notes: "Dette négociable : valeur NOMINALE, échéance à l'émission (un titre à 10 ans reste " +
		"« long terme » jusqu'à son remboursement). OAT indexées : nominal revalorisé de " +
		"l'inflation. Dette trimestrielle base 2020 : révisée à chaque compte trimestriel.",
}

const inseeBDM = "https://bdm.insee.fr/series/sdmx/data/"

// Chaque IDBANK attendu est décrit explicitement. Un identifiant inconnu fait
// échouer le chargement : l'INSEE change de base (2014, puis 2020) en créant
// de nouveaux identifiants, et une série qu'on ne sait pas qualifier ne doit
// pas entrer silencieusement dans des sommes.
var inseeNegociable = map[string]Series{
	"001739081": {Mesure: "ENCOURS"},
	"001711531": {Mesure: "ENCOURS", MonnaieEmission: "EUR"},
	"001711532": {Mesure: "ENCOURS", MonnaieEmission: "EUR", Echeance: "CT", BaseEcheance: "INITIALE"},
	"001711533": {Mesure: "ENCOURS", MonnaieEmission: "EUR", Echeance: "LT", BaseEcheance: "INITIALE"},
	"001719708": {Mesure: "ENCOURS", MonnaieEmission: "DEVISES"},
	"001719709": {Mesure: "ENCOURS", MonnaieEmission: "DEVISES", Echeance: "CT", BaseEcheance: "INITIALE"},
	"001719710": {Mesure: "ENCOURS", MonnaieEmission: "DEVISES", Echeance: "LT", BaseEcheance: "INITIALE"},
	"001738853": {Mesure: "ENCOURS", Instrument: "TAUX_FIXE"},
	"001738854": {Mesure: "ENCOURS", Instrument: "INDEXE_INFLATION"},
	"001738855": {Mesure: "VARIATION_CUMULEE", MonnaieEmission: "EUR"},
	"001738856": {Mesure: "VARIATION_CUMULEE", MonnaieEmission: "EUR", Echeance: "CT", BaseEcheance: "INITIALE"},
	"001738857": {Mesure: "VARIATION_CUMULEE", MonnaieEmission: "EUR", Echeance: "LT", BaseEcheance: "INITIALE"},
	"001738858": {Mesure: "VARIATION_CUMULEE", MonnaieEmission: "DEVISES"},
	"001738859": {Mesure: "VARIATION_CUMULEE", MonnaieEmission: "DEVISES", Echeance: "CT", BaseEcheance: "INITIALE"},
	"001738860": {Mesure: "VARIATION_CUMULEE", MonnaieEmission: "DEVISES", Echeance: "LT", BaseEcheance: "INITIALE"},
	"001738861": {Mesure: "VARIATION_CUMULEE", Instrument: "TAUX_FIXE"},
	"001738862": {Mesure: "VARIATION_CUMULEE", Instrument: "INDEXE_INFLATION"},
}

// Dette trimestrielle des APU. Les contributions des sous-secteurs sont
// CONSOLIDÉES : un titre de l'État détenu par la Sécurité sociale n'est
// compté nulle part, si bien que les quatre sous-secteurs se somment au total.
// Les instruments suivent la nomenclature SEC 2010 : F2 dépôts, F3 titres,
// F4 crédits, F6 réserves d'assurance, F8 autres comptes à payer, F12 DTS.
var inseeTrimestrielle = map[string]Series{
	"010777616": {Concept: "DETTE_MAASTRICHT", SecteurEmetteur: "S13"},
	"010777608": {Concept: "DETTE_MAASTRICHT", SecteurEmetteur: "S13", Unite: "PCT_PIB"},
	"010777606": {Concept: "DETTE_MAASTRICHT", SecteurEmetteur: "S13", Instrument: "F2"},
	"010777624": {Concept: "DETTE_MAASTRICHT", SecteurEmetteur: "S13", Instrument: "F3"},
	"010777609": {Concept: "DETTE_MAASTRICHT", SecteurEmetteur: "S13", Instrument: "F3", Echeance: "CT", BaseEcheance: "INITIALE"},
	"010777619": {Concept: "DETTE_MAASTRICHT", SecteurEmetteur: "S13", Instrument: "F3", Echeance: "LT", BaseEcheance: "INITIALE"},
	"010777607": {Concept: "DETTE_MAASTRICHT", SecteurEmetteur: "S13", Instrument: "F4"},
	"010777618": {Concept: "DETTE_MAASTRICHT", SecteurEmetteur: "S13", Instrument: "F4", Echeance: "CT", BaseEcheance: "INITIALE"},
	"010777617": {Concept: "DETTE_MAASTRICHT", SecteurEmetteur: "S13", Instrument: "F4", Echeance: "LT", BaseEcheance: "INITIALE"},
	"010777610": {Concept: "DETTE_MAASTRICHT", SecteurEmetteur: "S13111"}, // État
	"010777613": {Concept: "DETTE_MAASTRICHT", SecteurEmetteur: "S13112"}, // organismes divers d'administration centrale
	"010777626": {Concept: "DETTE_MAASTRICHT", SecteurEmetteur: "S1313"},  // administrations publiques locales
	"010777625": {Concept: "DETTE_MAASTRICHT", SecteurEmetteur: "S1314"},  // administrations de sécurité sociale
	"010777611": {Concept: "DETTE_NETTE_APU", SecteurEmetteur: "S13"},
	"010777621": {Concept: "ACTIFS_COTES_APU", SecteurEmetteur: "S13"},
	"010777622": {Concept: "DETTE_BRUTE_FMI", SecteurEmetteur: "S13"},
	"010777614": {Concept: "DETTE_BRUTE_FMI", SecteurEmetteur: "S13", Instrument: "F8"},
	"010777612": {Concept: "DETTE_BRUTE_FMI", SecteurEmetteur: "S13", Instrument: "F6"},
	"010777615": {Concept: "DETTE_BRUTE_FMI", SecteurEmetteur: "S13", Instrument: "F12"},
	"010777623": {Concept: "DETTE_BRUTE_FMI", SecteurEmetteur: "S13", MonnaieEmission: "EUR"},
	"010777620": {Concept: "DETTE_BRUTE_FMI", SecteurEmetteur: "S13", MonnaieEmission: "DEVISES"},
}

type inseeMessage struct {
	Series []struct {
		IDBank   string `xml:"IDBANK,attr"`
		Title    string `xml:"TITLE_FR,attr"`
		Freq     string `xml:"FREQ,attr"`
		UnitMult string `xml:"UNIT_MULT,attr"`
		Unit     string `xml:"UNIT_MEASURE,attr"`
		Obs      []struct {
			Period string `xml:"TIME_PERIOD,attr"`
			Value  string `xml:"OBS_VALUE,attr"`
			Status string `xml:"OBS_STATUS,attr"`
		} `xml:"Obs"`
	} `xml:"DataSet>Series"`
}

func IngestINSEE(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	return run(ctx, pool, arch, SourceINSEE, func(srcID, runID int64) (*batch, error) {
		l := newBatch()
		datasets := []struct {
			dataset  string
			expected map[string]Series
			base     Series
		}{
			{"DETTE-NEGOCIABLE-ETAT", inseeNegociable,
				Series{Concept: "DETTE_NEGOCIABLE_ETAT", SecteurEmetteur: "S13111"}},
			{"DETTE-TRIM-APU-2020", inseeTrimestrielle, Series{Mesure: "ENCOURS"}},
		}
		for _, j := range datasets {
			url := inseeBDM + j.dataset
			f, err := arch.Fetch(ctx, srcID, runID, url, ".xml")
			if err != nil {
				return nil, err
			}
			b, err := os.ReadFile(f.Path)
			if err != nil {
				return nil, err
			}
			var m inseeMessage
			if err := xml.Unmarshal(b, &m); err != nil {
				return nil, fmt.Errorf("%s : réponse SDMX illisible : %w", j.dataset, err)
			}
			seen := map[string]bool{}
			for _, s := range m.Series {
				d, ok := j.expected[s.IDBank]
				if !ok {
					return nil, fmt.Errorf("%s : série %s inconnue (%s) — à qualifier avant chargement",
						j.dataset, s.IDBank, s.Title)
				}
				seen[s.IDBank] = true
				mult, err := multiplier(s.UnitMult)
				if err != nil {
					return nil, fmt.Errorf("%s : %w", s.IDBank, err)
				}
				series := d
				series.Code = "insee:" + s.IDBank
				series.CodeSource = s.IDBank
				series.Libelle = s.Title
				series.Pays = "FR"
				// L'INSEE code le trimestre « T », SDMX et le reste du modèle « Q ».
				switch s.Freq {
				case "M", "A":
					series.Frequence = s.Freq
				case "T":
					series.Frequence = "Q"
				default:
					return nil, fmt.Errorf("%s : fréquence %q inattendue", s.IDBank, s.Freq)
				}
				series.URL = url
				series.Concept = orDefault(series.Concept, j.base.Concept)
				series.Mesure = orDefault(series.Mesure, j.base.Mesure)
				series.SecteurEmetteur = orDefault(series.SecteurEmetteur, j.base.SecteurEmetteur)
				switch {
				case series.Unite == "PCT_PIB":
					if s.Unit != "POURCENT" {
						return nil, fmt.Errorf("%s : unité %q, attendu POURCENT", s.IDBank, s.Unit)
					}
					mult = 1
				case s.Unit == "EUROS":
					series.Unite = "EUR"
				default:
					return nil, fmt.Errorf("%s : unité %q inattendue", s.IDBank, s.Unit)
				}
				var obs []Obs
				for _, o := range s.Obs {
					v, err := strconv.ParseFloat(o.Value, 64)
					if err != nil {
						continue // valeur non publiée (NaN) : la période reste absente
					}
					obs = append(obs, Obs{Periode: o.Period, Valeur: v * mult, Statut: o.Status, DocumentID: f.DocumentID})
				}
				l.add(&series, obs)
			}
			for id := range j.expected {
				if !seen[id] {
					return nil, fmt.Errorf("%s : série %s attendue mais absente", j.dataset, id)
				}
			}
		}
		return l, nil
	})
}
