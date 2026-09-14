// Package fiscalite charge ce qui documente l'évasion fiscale des
// multinationales et ce que la France en perçoit — ou non : les estimations du
// transfert de bénéfices, les déclarations pays par pays agrégées, les revenus
// des investissements directs, les comptes des filiales françaises de groupes
// étrangers, les marchés publics et les faits établis (contrats, règlements
// fiscaux, enquêtes parlementaires) qui les concernent ; et, pour situer la
// France, les grilles officielles des paradis fiscaux.
// Voir docs/evasion-fiscale-multinationales.md.
package fiscalite

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ConnectorVersion = "fiscalite-v1"

var SourceOCDEImpotSocietes = archive.Source{
	Slug: "ocde-statistiques-impot-societes", Label: "OCDE — statistiques de l'impôt sur les sociétés (taux, taux effectifs, régimes PI, CbCR agrégé)",
	Publisher: "OCDE, Centre de politique et d'administration fiscales", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Creative Commons Attribution 4.0 (données de l'OCDE)",
	ReuseClass:  "ATTRIBUTION",
	Attribution: "Source : OCDE, Corporate Tax Statistics",
	Cadence:     "annuelle",
	Notes: "Déclarations pays par pays : agrégats anonymisés transmis par les administrations, groupes " +
		"de plus de 750 M€ de chiffre d'affaires. Tous groupes confondus, pertes comprises : le rapport " +
		"impôt / bénéfice d'une juridiction mêle groupes bénéficiaires et déficitaires. Certains pays " +
		"transmettent des données incomplètes ou non consolidées (doubles comptes intragroupe). Taux " +
		"légaux combinés : contributions exceptionnelles comprises (France 2025 : 36,13 %).",
}

var SourceOCDEIDE = archive.Source{
	Slug: "ocde-investissements-directs", Label: "OCDE — revenus des investissements directs par pays de contrepartie (BMD4)",
	Publisher: "OCDE, Direction des affaires financières et des entreprises", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Creative Commons Attribution 4.0 (données de l'OCDE)",
	ReuseClass:  "ATTRIBUTION",
	Attribution: "Source : OCDE, statistiques d'investissement direct international",
	Cadence:     "annuelle",
	Notes: "Pays de contrepartie immédiat, pas le pays de la mère ultime : un dividende versé par une filiale " +
		"française à une holding luxembourgeoise d'un groupe américain est compté vers le Luxembourg. " +
		"Valeurs confidentielles non publiées : absentes, jamais nulles.",
}

var SourceEurostatFATS = archive.Source{
	Slug: "eurostat-filiales-etrangeres", Label: "Eurostat — filiales sous contrôle étranger (FATS entrantes)",
	Publisher: "Eurostat, d'après l'INSEE", Tier: "PRIMARY_OFFICIAL",
	Licence: "Creative Commons Attribution 4.0 (CC BY 4.0)", ReuseClass: "ATTRIBUTION",
	Attribution: "Source : Eurostat, fats_g1b_08 et fats_ctrl",
	Cadence:     "annuelle",
	Notes: "Pays de contrôle = pays de l'unité institutionnelle contrôlante ultime. Rupture de série en 2021 " +
		"(fats_g1b_08 jusqu'à 2020, fats_ctrl ensuite, nomenclature d'indicateurs différente). Hors " +
		"secteur financier.",
}

var SourceGLEIF = archive.Source{
	Slug: "gleif-lei-niveau2", Label: "GLEIF — répertoire mondial des LEI et relations de contrôle (niveau 2)",
	Publisher: "Global Legal Entity Identifier Foundation", Tier: "PRIMARY_OFFICIAL",
	Licence: "CC0 1.0", ReuseClass: "OPEN",
	Attribution: "Source : GLEIF, fichiers Golden Copy LEI et Relationship Records",
	Cadence:     "trois publications par jour",
	Notes: "Relations déclarées par les entités elles-mêmes, souvent incomplètes : 1 904 sociétés françaises " +
		"actives identifiées par leur SIREN déclaraient une mère ultime étrangère en septembre 2026 (Apple " +
		"France, Microsoft France ou Amazon France n'en font pas partie). Le pays est celui de la société " +
		"de tête : Stellantis et Airbus y sont néerlandais. Repérage systématique, pas recensement.",
}

var SourceRatiosINPI = archive.Source{
	Slug: "inpi-bce-ratios-financiers", Label: "INPI / BCE — ratios financiers des comptes annuels déposés",
	Publisher: "INPI et Banque centrale européenne (data.economie.gouv.fr)", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Ratios financiers BCE / INPI, data.economie.gouv.fr",
	Cadence:     "continue",
	Notes: "Pas de ligne « impôt sur les bénéfices » : le résultat courant avant impôt est reconstitué " +
		"(ratio publié × chiffre d'affaires), et son écart au résultat net mêle impôt, résultat " +
		"exceptionnel et participation. Comptes confidentiels (option des petites entreprises) absents.",
}

// executer encadre un connecteur.
func executer(ctx context.Context, arch *archive.Archive, src archive.Source,
	f func(srcID, runID int64) (map[string]any, error)) error {
	srcID, err := arch.EnsureSource(ctx, src)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersion)
	if err != nil {
		return err
	}
	stats, err := f(srcID, runID)
	if err != nil {
		err = fmt.Errorf("%s : %w", src.Slug, err)
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}
	arch.EndRun(ctx, runID, "SUCCESS", stats, "")
	fmt.Printf("  %-34s %v\n", src.Slug, stats)
	return nil
}

// lireCSV lit un CSV avec en-tête en tableaux de colonnes nommées.
func lireCSV(path string) ([]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cr := csv.NewReader(f)
	cr.FieldsPerRecord = -1
	entete, err := cr.Read()
	if err != nil {
		return nil, err
	}
	for i := range entete {
		entete[i] = strings.TrimPrefix(entete[i], "\uFEFF")
	}
	var out []map[string]string
	for {
		rec, err := cr.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		m := make(map[string]string, len(entete))
		for i, k := range entete {
			if i < len(rec) {
				m[k] = rec[i]
			}
		}
		out = append(out, m)
	}
	return out, nil
}

// Ingest charge tout le bloc. SIRENE (ref.unite_legale) doit être chargé
// pour que les comptes des filiales portent leur dénomination.
func Ingest(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	for _, e := range []func(context.Context, *pgxpool.Pool, *archive.Archive) error{
		IngestListes, IngestOCDEImpotSocietes, IngestOCDEIDE, IngestFATS, IngestTWZ, IngestFiliales,
		IngestMarches, IngestFaits,
	} {
		if err := e(ctx, pool, arch); err != nil {
			return err
		}
	}
	return nil
}
