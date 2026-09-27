package entreprises

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"strconv"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Investissement productif et dividendes versés par les sociétés non
// financières (comptes nationaux), et la répartition par catégorie
// d'entreprise (Ésane) — le point de départ d'un chantier sur ce que
// l'argent des entreprises finance réellement, et où, pas une démonstration :
// les chiffres peuvent aussi bien confirmer qu'infirmer une thèse donnée en
// amont ; seule la mesure compte. Voir docs/investissement-entreprises-donnees.md.

const ConnectorVersionInvestissement = "entreprises-investissement-v1"

var SourceComptesSNF = archive.Source{
	Slug: "insee-bdm-comptes-snf", Label: "Insee — comptes des sociétés non financières (BDM)",
	Publisher: "INSEE", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Licence Ouverte v2.0",
	ReuseClass:  "OPEN",
	Attribution: "Source : Insee, comptes nationaux trimestriels, sociétés non financières",
	Cadence:     "trimestrielle",
	Notes: "Séries CVS(-CJO) de la Banque de données macro-économiques (BDM), valeur aux prix " +
		"courants, France entière, secteur S11 (sociétés non financières) toutes tailles et tous " +
		"secteurs confondus. Dividendes (idbank 011794592) et formation brute de capital fixe — " +
		"l'investissement productif (idbank 011794792), même champ, même unité, directement " +
		"comparables terme à terme.",
}

var SourceEsaneCategorie = archive.Source{
	Slug: "insee-esane-categorie-entreprise", Label: "Insee — Ésane, le tissu productif par catégorie d'entreprise",
	Publisher: "INSEE", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Licence Ouverte v2.0",
	ReuseClass:  "OPEN",
	Attribution: "Source : Insee, Ésane, Insee Focus « Le tissu productif français par catégorie d'entreprises »",
	Cadence:     "annuelle",
	Notes: "Catégories au sens de la loi de modernisation de l'économie de 2008 (seuils d'effectif, " +
		"chiffre d'affaires et total de bilan) : micro-entreprises (MIC), PME hors MIC, entreprises " +
		"de taille intermédiaire (ETI), grandes entreprises (GE). Champ : secteurs principalement " +
		"marchands non agricoles et non financiers. Millésime 2022, le seul publié sous cette forme " +
		"au moment du chargement — pas une série : une photographie, à rejouer quand l'édition " +
		"suivante paraît.",
}

const (
	bdmDividendesIdbank = "011794592"
	bdmFBCFIdbank       = "011794792"
	bdmURL              = "https://www.bdm.insee.fr/series/sdmx/data/SERIES_BDM/"
	esaneFocus343URL    = "https://www.insee.fr/fr/statistiques/fichier/8290682/IF343.xlsx"
	esaneMillesime      = 2022
)

type sdmxDataSet struct {
	Series struct {
		IDBank string    `xml:"IDBANK,attr"`
		Obs    []sdmxObs `xml:"Obs"`
	} `xml:"DataSet>Series"`
}

type sdmxObs struct {
	Periode string `xml:"TIME_PERIOD,attr"`
	Valeur  string `xml:"OBS_VALUE,attr"`
}

// IngestFluxFinancierSNF charge les deux séries trimestrielles BDM
// (dividendes versés, formation brute de capital fixe) des sociétés non
// financières. Les deux idbank partagent le même champ et la même unité :
// aucun retraitement n'est nécessaire pour les comparer terme à terme.
func IngestFluxFinancierSNF(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceComptesSNF)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersionInvestissement)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	series := []struct{ nom, idbank string }{
		{"dividendes", bdmDividendesIdbank},
		{"fbcf", bdmFBCFIdbank},
	}
	var rows [][]any
	compte := map[string]int{}
	for _, s := range series {
		f, err := arch.Fetch(ctx, srcID, runID, bdmURL+s.idbank, ".xml")
		if err != nil {
			return fail(fmt.Errorf("%s (idbank %s) : %w", s.nom, s.idbank, err))
		}
		raw, err := os.ReadFile(f.Path)
		if err != nil {
			return fail(err)
		}
		var ds sdmxDataSet
		if err := xml.Unmarshal(raw, &ds); err != nil {
			return fail(fmt.Errorf("%s : réponse SDMX illisible : %w", s.nom, err))
		}
		if len(ds.Series.Obs) == 0 {
			return fail(fmt.Errorf("%s (idbank %s) : aucune observation", s.nom, s.idbank))
		}
		for _, o := range ds.Series.Obs {
			v, err := strconv.ParseFloat(o.Valeur, 64)
			if err != nil {
				continue // une observation manquante ('OBS_VALUE=""') n'invalide pas les autres
			}
			rows = append(rows, []any{s.nom, o.Periode, v, srcID})
			compte[s.nom]++
		}
	}
	if compte["dividendes"] == 0 || compte["fbcf"] == 0 {
		return fail(fmt.Errorf("série incomplète : %d dividendes, %d fbcf", compte["dividendes"], compte["fbcf"]))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_flux_financier_snf (
			serie text, trimestre text, valeur_meur numeric, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_flux_financier_snf"},
		[]string{"serie", "trimestre", "valeur_meur", "source_id"}, pgx.CopyFromRows(rows)); err != nil {
		return fail(err)
	}
	if _, err := tx.Exec(ctx, `
		MERGE INTO core.flux_financier_snf AS tgt
		USING tmp_flux_financier_snf AS src ON tgt.serie = src.serie AND tgt.trimestre = src.trimestre
		WHEN MATCHED AND (tgt.valeur_meur, tgt.source_id) IS DISTINCT FROM (src.valeur_meur, src.source_id)
		THEN UPDATE SET valeur_meur = src.valeur_meur, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		     INSERT (serie, trimestre, valeur_meur, source_id)
		     VALUES (src.serie, src.trimestre, src.valeur_meur, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`); err != nil {
		return fail(fmt.Errorf("fusion flux_financier_snf : %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"trimestres_dividendes": compte["dividendes"], "trimestres_fbcf": compte["fbcf"]}, "")
	fmt.Printf("  flux financiers des sociétés non financières : %d trimestres de dividendes, %d de FBCF\n",
		compte["dividendes"], compte["fbcf"])
	return nil
}

// IngestEntrepriseCategorie charge la photographie 2022 du tissu productif
// par catégorie d'entreprise (Ésane, via le fichier de données de l'Insee
// Focus n°343), feuille « Figure 1 ».
func IngestEntrepriseCategorie(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceEsaneCategorie)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersionInvestissement)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, esaneFocus343URL, ".xlsx")
	if err != nil {
		return fail(err)
	}
	x, err := openXLSXEnt(f.Path)
	if err != nil {
		return fail(err)
	}
	defer x.Close()
	lignes, err := x.rows("Figure 1")
	if err != nil {
		return fail(err)
	}

	// La feuille répète le même libellé de ligne dans les deux champs
	// (« secteurs marchands non agricoles et non financiers » puis, plus
	// bas, « secteurs principalement marchands ») : on retient le second
	// bloc, celui que documente le texte de la publication et qui porte le
	// taux d'investissement.
	categories := []string{"MIC", "PME", "ETI", "GE"}
	champs := map[string]int{} // libellé de ligne -> index de ligne (dernière occurrence)
	for i, l := range lignes {
		lib := l["A"]
		if lib != "" {
			champs[lib] = i
		}
	}
	get := func(libelle string, col string) (float64, bool) {
		i, ok := champs[libelle]
		if !ok {
			return 0, false
		}
		v, err := strconv.ParseFloat(lignes[i][col], 64)
		return v, err == nil
	}
	cols := map[string]string{"MIC": "B", "PME": "C", "ETI": "D", "GE": "E"}

	var rows [][]any
	for _, cat := range categories {
		col := cols[cat]
		row := []any{esaneMillesime, cat, nil, nil, nil, nil, nil, nil, srcID}
		if v, ok := get("Nombre d'entreprises", col); ok {
			n := int(v)
			row[2] = n
		}
		if v, ok := get("Effectif salarié en ETP (en milliers)", col); ok {
			row[3] = v
		}
		if v, ok := get("Chiffre d'affaires (en milliards d'euros)", col); ok {
			row[4] = v * 1000 // Md€ -> M€
		}
		if v, ok := get("Valeur ajoutée hors taxes (en milliards d'euros)", col); ok {
			row[5] = v * 1000
		}
		if v, ok := get("Taux d'investissement (investissement corporel/VA) (en %)", col); ok {
			row[6] = v
		}
		if v, ok := get("Immobilisations corporelles par salarié en ETP (en milliers d'euros)", col); ok {
			row[7] = v * 1000 // k€ -> €
		}
		if row[6] == nil {
			return fail(fmt.Errorf("catégorie %s : taux d'investissement introuvable — la mise en page de la feuille a dû changer", cat))
		}
		rows = append(rows, row)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	// Seul le millésime esaneMillesime est chargé par ce connecteur ; un
	// autre millésime, une fois chargé, ne doit pas être touché ici.
	if _, err := tx.Exec(ctx, fmt.Sprintf(`
		CREATE OR REPLACE TEMPORARY VIEW entreprise_categorie_scope AS
		SELECT * FROM core.entreprise_categorie WHERE annee = %d
		WITH LOCAL CHECK OPTION`, esaneMillesime)); err != nil {
		return fail(err)
	}
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_entreprise_categorie (
			annee integer, categorie text, nb_entreprises integer, effectif_etp_milliers numeric,
			chiffre_affaires_meur numeric, valeur_ajoutee_meur numeric, taux_investissement_pct numeric,
			immobilisations_par_salarie_eur numeric, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_entreprise_categorie"},
		[]string{"annee", "categorie", "nb_entreprises", "effectif_etp_milliers",
			"chiffre_affaires_meur", "valeur_ajoutee_meur", "taux_investissement_pct",
			"immobilisations_par_salarie_eur", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(err)
	}
	if _, err := tx.Exec(ctx, `
		MERGE INTO entreprise_categorie_scope AS tgt
		USING tmp_entreprise_categorie AS src ON tgt.annee = src.annee AND tgt.categorie = src.categorie
		WHEN MATCHED AND (tgt.nb_entreprises, tgt.effectif_etp_milliers, tgt.chiffre_affaires_meur,
		                   tgt.valeur_ajoutee_meur, tgt.taux_investissement_pct,
		                   tgt.immobilisations_par_salarie_eur, tgt.source_id)
		     IS DISTINCT FROM (src.nb_entreprises, src.effectif_etp_milliers, src.chiffre_affaires_meur,
		                        src.valeur_ajoutee_meur, src.taux_investissement_pct,
		                        src.immobilisations_par_salarie_eur, src.source_id)
		THEN UPDATE SET nb_entreprises = src.nb_entreprises, effectif_etp_milliers = src.effectif_etp_milliers,
		     chiffre_affaires_meur = src.chiffre_affaires_meur, valeur_ajoutee_meur = src.valeur_ajoutee_meur,
		     taux_investissement_pct = src.taux_investissement_pct,
		     immobilisations_par_salarie_eur = src.immobilisations_par_salarie_eur, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		     INSERT (annee, categorie, nb_entreprises, effectif_etp_milliers, chiffre_affaires_meur,
		             valeur_ajoutee_meur, taux_investissement_pct, immobilisations_par_salarie_eur, source_id)
		     VALUES (src.annee, src.categorie, src.nb_entreprises, src.effectif_etp_milliers,
		             src.chiffre_affaires_meur, src.valeur_ajoutee_meur, src.taux_investissement_pct,
		             src.immobilisations_par_salarie_eur, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`); err != nil {
		return fail(fmt.Errorf("fusion entreprise_categorie : %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"categories": len(rows), "millesime": esaneMillesime}, "")
	fmt.Printf("  entreprises par catégorie (Ésane %d) : %d catégories\n", esaneMillesime, len(rows))
	return nil
}

func IngestInvestissement(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	if err := IngestFluxFinancierSNF(ctx, pool, arch); err != nil {
		return err
	}
	return IngestEntrepriseCategorie(ctx, pool, arch)
}
