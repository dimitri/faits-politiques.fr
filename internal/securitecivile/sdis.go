// Package securitecivile charge le financement des services départementaux
// d'incendie et de secours (SDIS) — qui paie les pompiers, et combien.
package securitecivile

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/bulkload"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Même producteur et même licence que les comptes communaux et les comptes
// des régions/départements/groupements (internal/communes) : l'OFGL
// consolide les comptes de TOUTES les collectivités, SDIS compris, avec les
// mêmes conventions d'agrégat — ce qui rend les niveaux lisibles les uns à
// côté des autres, jamais additionnés (un euro départemental versé au SDIS
// est déjà compté dans le budget du département).
var SourceSDIS = archive.Source{
	Slug: "ofgl-sdis", Label: "OFGL — comptes des SDIS",
	Publisher:  "Observatoire des finances et de la gestion publique locales",
	Tier:       "PRIMARY_OFFICIAL",
	License:    "Licence Ouverte v2.0",
	ReuseClass: "OPEN",
	Attribution: "Source : OFGL (Observatoire des finances et de la gestion publique locales), " +
		"d'après les comptes de gestion de la DGFiP",
	Cadence: "annuelle",
	Notes: "Budget principal du SDIS uniquement (cbudg=1) : les budgets annexes (écoles " +
		"départementales, restaurants, téléassistance…) ne sont pas chargés, ce ne sont pas des " +
		"masses de financement du secours. Paris et Marseille ne sont pas des SDIS — la Brigade de " +
		"sapeurs-pompiers de Paris et le Bataillon de marins-pompiers de Marseille sont des unités " +
		"militaires, financées et commandées autrement (voir docs/securite-civile-donnees.md) : " +
		"absentes de ce jeu de données par construction, pas par omission.",
}

const (
	sdisFirstFiscalYear = 2012
	sdisLastFiscalYear  = 2025
	sdisDataset         = "ofgl-base-sdis"
)

// Agrégat publié par l'OFGL -> code d'indicateur de ref.indicator (voir
// db/migrations/0203_sdis_budget.sql). Une liste à part de
// internal/communes.ofglAggregates : les deux parcourent la même source,
// mais pas les mêmes agrégats (un SDIS ne touche pas de DGF, par exemple ;
// à l'inverse, "Participations des départements" n'existe pour aucun autre
// niveau suivi aujourd'hui).
var sdisAggregates = map[string]string{
	"Participations des départements":                   "sdis.participation_departements",
	"Participations des communes":                       "sdis.participation_communes",
	"Participations des groupements à fiscalité propre": "sdis.participation_groupements",
	"Participations des régions":                        "sdis.participation_regions",
	"Concours de l'Etat":                                "sdis.concours_etat",
	"Frais de personnel":                                "sdis.frais_personnel",
	"Dépenses totales":                                  "sdis.depenses_totales",
	"Recettes totales":                                  "sdis.recettes_totales",
	"Dépenses d'équipement":                             "sdis.depenses_equipement",
	"Encours de dette":                                  "sdis.encours_dette",
}

func sdisURL(fiscalYear int) string {
	names := make([]string, 0, len(sdisAggregates))
	for a := range sdisAggregates {
		names = append(names, `"`+a+`"`)
	}
	// ODSQL : budget principal seulement (cbudg=1) — les budgets annexes
	// partagent le même Siren mais un identifiant de budget (ident/lbudg)
	// différent, voir SourceSDIS.Notes.
	where := fmt.Sprintf(`exer=date'%d' AND cbudg="1" AND agregat IN (%s)`,
		fiscalYear, strings.Join(names, ","))
	return "https://data.ofgl.fr/api/explore/v2.1/catalog/datasets/" + sdisDataset +
		"/exports/csv?delimiter=%3B&select=" +
		url.QueryEscape("siren,lbudg,code_dep,agregat,montant,euros_par_habitant,population_totale,outre_mer,dep_rural") +
		"&where=" + url.QueryEscape(where)
}

func IngestSDIS(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceSDIS)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "sdis-v1")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	type dim struct {
		nom, codeDep       string
		outreMer, depRural bool
		depRuralConnu      bool
		populationTotale   any
	}
	dims := map[string]dim{} // siren -> dimension la plus récente rencontrée

	var budget [][]any
	for year := sdisFirstFiscalYear; year <= sdisLastFiscalYear; year++ {
		f, err := arch.Fetch(ctx, srcID, runID, sdisURL(year), ".csv")
		if err != nil {
			return fail(fmt.Errorf("exercice %d : %w", year, err))
		}
		recs, err := readCSV(f.Path, ';')
		if err != nil {
			return fail(fmt.Errorf("exercice %d : %w", year, err))
		}
		vus := map[string]bool{}
		for _, r := range recs {
			siren := r["siren"]
			ind, ok := sdisAggregates[r["agregat"]]
			if siren == "" || !ok {
				continue
			}
			k := siren + "|" + ind
			if vus[k] {
				continue
			}
			vus[k] = true
			budget = append(budget, []any{
				"SDIS", siren, r["lbudg"], year, ind,
				parseFloatOrNull(r["montant"]), parseFloatOrNull(r["euros_par_habitant"]),
				parseIntOrNull(r["population_totale"]), srcID,
			})
			dims[siren] = dim{
				nom: r["lbudg"], codeDep: r["code_dep"],
				outreMer: r["outre_mer"] == "Oui",
				depRural: r["dep_rural"] == "Oui", depRuralConnu: r["dep_rural"] != "",
				populationTotale: parseIntOrNull(r["population_totale"]),
			}
		}
		fmt.Printf("    exercice %d : %d lignes\n", year, len(recs))
	}
	if len(dims) == 0 {
		return fail(fmt.Errorf("aucun SDIS trouvé : vérifier le schéma de %s", sdisDataset))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	// core.sdis : la dimension (un SDIS, un Siren, un département), écrasée
	// à chaque ingestion par la vue la plus récente rencontrée — jamais
	// additionnée à l'historique, il n'y a qu'un département de
	// rattachement par SDIS à la fois.
	var dimRows [][]any
	for siren, d := range dims {
		var depRural any
		if d.depRuralConnu {
			depRural = d.depRural
		}
		dimRows = append(dimRows, []any{siren, d.nom, d.codeDep, d.outreMer, depRural, d.populationTotale, srcID})
	}
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_sdis (
			siren text, nom text, code_departement text, outre_mer boolean,
			departement_rural boolean, population_totale int, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_sdis"},
		[]string{"siren", "nom", "code_departement", "outre_mer", "departement_rural", "population_totale", "source_id"},
		pgx.CopyFromRows(dimRows)); err != nil {
		return fail(fmt.Errorf("dimension SDIS : %w", err))
	}
	if _, err := tx.Exec(ctx, `
		MERGE INTO core.sdis AS tgt
		USING tmp_sdis AS src
		ON tgt.siren = src.siren
		WHEN MATCHED AND (tgt.nom, tgt.code_departement, tgt.outre_mer, tgt.departement_rural,
		                   tgt.population_totale, tgt.source_id)
		                  IS DISTINCT FROM
		                  (src.nom, src.code_departement, src.outre_mer, src.departement_rural,
		                   src.population_totale, src.source_id) THEN
		    UPDATE SET nom = src.nom, code_departement = src.code_departement,
		               outre_mer = src.outre_mer, departement_rural = src.departement_rural,
		               population_totale = src.population_totale, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (siren, nom, code_departement, outre_mer, departement_rural, population_totale, source_id)
		    VALUES (src.siren, src.nom, src.code_departement, src.outre_mer, src.departement_rural,
		            src.population_totale, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`); err != nil {
		return fail(fmt.Errorf("fusion dimension SDIS : %w", err))
	}

	// core.collectivite_budget (niveau='SDIS') : les comptes. Fusion SCOPÉE
	// à niveau='SDIS' par une vue temporaire, exactement comme
	// commune_indicator_ofgl (internal/communes/ofgl.go) scope sur un
	// préfixe d'indicator_code — la table est partagée avec
	// internal/communes.IngestCollectivites (REGION/DEPARTEMENT/GROUPEMENT),
	// qui fusionne elle aussi par un WHEN NOT MATCHED BY SOURCE THEN
	// DELETE : sans ce scope, chaque connecteur effacerait les lignes de
	// l'autre à son tour.
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_collectivite_budget_sdis (
			niveau text, code text, nom text, exercice int, indicator_code text,
			montant numeric, euros_par_hab numeric, population int, source_id bigint
		) ON COMMIT DROP;
		CREATE OR REPLACE TEMPORARY VIEW collectivite_budget_sdis AS
		  SELECT * FROM core.collectivite_budget WHERE niveau = 'SDIS'
		  WITH LOCAL CHECK OPTION`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_collectivite_budget_sdis"},
		[]string{"niveau", "code", "nom", "exercice", "indicator_code",
			"montant", "euros_par_hab", "population", "source_id"},
		pgx.CopyFromRows(budget)); err != nil {
		return fail(fmt.Errorf("comptes SDIS : %w", err))
	}
	var touchees int64
	err = bulkload.WithoutFKConstraints(ctx, tx, "core.collectivite_budget", func() error {
		ct, err := tx.Exec(ctx, `
			MERGE INTO collectivite_budget_sdis AS tgt
			USING tmp_collectivite_budget_sdis AS src
			ON tgt.niveau = src.niveau AND tgt.code = src.code
			   AND tgt.exercice = src.exercice AND tgt.indicator_code = src.indicator_code
			WHEN MATCHED AND (tgt.nom, tgt.montant, tgt.euros_par_hab, tgt.population, tgt.source_id)
			                  IS DISTINCT FROM
			                  (src.nom, src.montant, src.euros_par_hab, src.population, src.source_id) THEN
			    UPDATE SET nom = src.nom, montant = src.montant, euros_par_hab = src.euros_par_hab,
			               population = src.population, source_id = src.source_id
			WHEN NOT MATCHED BY TARGET THEN
			    INSERT (niveau, code, nom, exercice, indicator_code, montant, euros_par_hab,
			            population, source_id)
			    VALUES (src.niveau, src.code, src.nom, src.exercice, src.indicator_code, src.montant,
			            src.euros_par_hab, src.population, src.source_id)
			WHEN NOT MATCHED BY SOURCE THEN DELETE`)
		if err != nil {
			return err
		}
		touchees = ct.RowsAffected()
		return nil
	})
	if err != nil {
		return fail(fmt.Errorf("fusion : %w", err))
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"sdis": len(dims), "valeurs": len(budget), "touchees": touchees}, "")
	fmt.Printf("  SDIS : %d établissements, %d valeurs sur %d-%d (%d touchées par la fusion)\n",
		len(dims), len(budget), sdisFirstFiscalYear, sdisLastFiscalYear, touchees)
	return nil
}

// readCSV renvoie les lignes sous forme de maps colonne -> valeur — même
// forme que internal/communes.readCSV, dupliquée plutôt que partagée : un
// aller simple (lire un CSV ; ; vers une map) ne vaut pas une dépendance
// inter-paquets pour si peu.
func readCSV(path string, sep rune) ([]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comma = sep
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	recs, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%s : %w", path, err)
	}
	if len(recs) < 2 {
		return nil, fmt.Errorf("%s : fichier vide", path)
	}
	head := make([]string, len(recs[0]))
	for i, h := range recs[0] {
		head[i] = strings.TrimSpace(strings.TrimPrefix(h, "\xef\xbb\xbf"))
	}
	out := make([]map[string]string, 0, len(recs)-1)
	for _, rec := range recs[1:] {
		m := make(map[string]string, len(head))
		for i, h := range head {
			if i < len(rec) {
				m[h] = strings.TrimSpace(rec[i])
			}
		}
		out = append(out, m)
	}
	return out, nil
}

func parseFloatOrNull(s string) any {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", ".")
	if s == "" || s == "NA" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return v
}

func parseIntOrNull(s string) any {
	s = strings.TrimSpace(s)
	if s == "" || s == "NA" {
		return nil
	}
	if i := strings.IndexAny(s, ",."); i >= 0 {
		s = s[:i]
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return n
}
