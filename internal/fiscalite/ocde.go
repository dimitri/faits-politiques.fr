package fiscalite

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const sdmxOCDE = "https://sdmx.oecd.org/public/rest/data/"

// urlOCDE compose une requête SDMX : une clé par dimension, vide pour « toutes ».
func urlOCDE(flux string, cle []string, debut int) string {
	return fmt.Sprintf("%s%s/%s?startPeriod=%d&format=csvfilewithlabels", sdmxOCDE, flux, strings.Join(cle, "."), debut)
}

// Juridictions de contrepartie retenues pour le CbCR : la France, ses grands
// voisins, et les juridictions que la littérature et les listes désignent
// comme centres de profits. Toutes les juridictions pour tous les sièges
// dépasseraient la centaine de mégaoctets pour un usage qui n'en demande
// qu'une vingtaine. WXD = reste du monde (tout l'étranger du siège, total
// qui permet les parts) ; STLS = entités apatrides (sans résidence fiscale).
var contrepartiesCbCR = []string{
	"FRA", "DEU", "ITA", "ESP", "GBR", "USA", "JPN", "CHN",
	"IRL", "LUX", "NLD", "BEL", "CHE", "MLT", "CYP", "HUN", "SGP", "HKG", "PRI",
	"BMU", "CYM", "VGB", "BHS", "JEY", "GGY", "IMN", "CAN", "MEX", "IND", "BRA", "KOR", "AUS", "POL",
	"WXD", "STLS",
}

// Mesures CbCR conservées : les montants et effectifs. Les distributions de
// taux effectifs (percentiles) et les comptages d'activités sont écartés.
var mesuresCbCR = map[string]bool{
	"PROFIT": true, "PROFIT_ADJ": true, "TAX_PAID": true, "TAX_ACCRUED": true, "EMPLOYEES": true,
	"TOT_REV": true, "RPR": true, "UPR": true, "ASSETS": true, "EARNINGS": true,
	"STATED_CAPITAL": true, "ENTITIES_COUNT": true, "CBCR_COUNT": true, "ACT_HOLDING": true, "ACT_IP": true,
}

// valeurOCDE lit OBS_VALUE et applique UNIT_MULT quand la colonne existe.
// Chaîne vide : valeur absente, jamais zéro.
func valeurOCDE(r map[string]string) (float64, bool, error) {
	s := strings.TrimSpace(r["OBS_VALUE"])
	if s == "" {
		return 0, false, nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false, fmt.Errorf("OBS_VALUE %q", s)
	}
	if m := r["UNIT_MULT"]; m != "" && m != "0" {
		n, err := strconv.Atoi(m)
		if err != nil {
			return 0, false, fmt.Errorf("UNIT_MULT %q", m)
		}
		v *= math.Pow10(n)
	}
	return v, true, nil
}

func IngestOCDEImpotSocietes(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	return executer(ctx, arch, SourceOCDEImpotSocietes, func(srcID, runID int64) (map[string]any, error) {
		stats := map[string]any{}

		// 1. Déclarations pays par pays : tous les sièges, juridictions retenues.
		u := urlOCDE("OECD.CTP.TPS,DSD_CBCR@DF_CBCRI,1.1",
			[]string{"", strings.Join(contrepartiesCbCR, "+"), "A", "", "", "_Z", "TOTAL", "", "", ""}, 2016)
		f, err := arch.Fetch(ctx, srcID, runID, u, ".csv")
		if err != nil {
			return nil, err
		}
		rows, err := lireCSV(f.Path)
		if err != nil {
			return nil, err
		}
		type cleCbCR struct{ annee, siege, jur, mesure, groupe string }
		vus := map[cleCbCR]bool{}
		var cbcr [][]any
		for _, r := range rows {
			if !mesuresCbCR[r["MEASURE"]] || r["STATISTICAL_OPERATION"] != "_Z" {
				continue
			}
			v, ok, err := valeurOCDE(r)
			if err != nil {
				return nil, fmt.Errorf("CbCR : %w", err)
			}
			if !ok {
				continue
			}
			k := cleCbCR{r["TIME_PERIOD"], r["REF_AREA"], r["COUNTERPART_AREA"], r["MEASURE"], r["PROFIT_GROUPING"]}
			if vus[k] {
				return nil, fmt.Errorf("CbCR : doublon %v (dimension non filtrée)", k)
			}
			vus[k] = true
			annee, err := strconv.Atoi(k.annee)
			if err != nil {
				return nil, fmt.Errorf("CbCR : période %q", k.annee)
			}
			cbcr = append(cbcr, []any{annee, k.siege, k.jur, k.mesure, k.groupe, r["UNIT_MEASURE"], v, f.DocumentID})
		}
		stats["cbcr"] = len(cbcr)

		// 2. Indicateurs par pays : taux légaux, taux effectifs, régimes de
		// propriété intellectuelle. Tous les pays couverts.
		var pays [][]any
		type clePays struct{ pays, annee, ind, var_ string }
		vusPays := map[clePays]bool{}
		ajoutePays := func(r map[string]string, ind, variante string, doc int64, texteAdmis bool) error {
			annee, err := strconv.Atoi(r["TIME_PERIOD"])
			if err != nil {
				return fmt.Errorf("période %q", r["TIME_PERIOD"])
			}
			k := clePays{r["REF_AREA"], r["TIME_PERIOD"], ind, variante}
			if vusPays[k] {
				return fmt.Errorf("doublon %v", k)
			}
			brut := strings.TrimSpace(r["OBS_VALUE"])
			if brut == "" {
				return nil
			}
			var valeur, texte any
			unite := r["UNIT_MEASURE"]
			if n, err := strconv.ParseFloat(strings.TrimSuffix(brut, "%"), 64); err == nil {
				valeur = n
				if strings.HasSuffix(brut, "%") {
					unite = "PT"
				}
			} else if texteAdmis {
				texte = brut
			} else {
				return fmt.Errorf("%s : valeur non numérique %q", ind, brut)
			}
			vusPays[k] = true
			pays = append(pays, []any{k.pays, annee, ind, variante, valeur, texte, nul(unite), doc})
			return nil
		}

		requetes := []struct {
			nom  string
			url  string
			lire func(r map[string]string, doc int64) error
		}{
			{"taux légaux", urlOCDE("OECD.CTP.TPS,DSD_TAX_CIT@DF_CIT,", make([]string, 9), 2000),
				func(r map[string]string, doc int64) error {
					return ajoutePays(r, "CIT."+r["MEASURE"], r["TARGETING"]+"."+r["SECTOR"], doc, false)
				}},
			{"taux effectifs", urlOCDE("OECD.CTP.TPS,DSD_ETR@DF_ETR_BASELINE,", []string{"", "A", "EATR+EMTR", "", "BASELINE", "", "", ""}, 2017),
				func(r map[string]string, doc int64) error {
					return ajoutePays(r, "ETR."+r["MEASURE"], r["ETR_SCENARIO"]+"."+r["ETR_TAX_TYPE"]+"."+r["REGIME"], doc, false)
				}},
			{"régimes PI", urlOCDE("OECD.CTP.TPS,DSD_QDD_IPR@DF_QDD_IPR,", make([]string, 4), 2017),
				func(r map[string]string, doc int64) error {
					return ajoutePays(r, "IPR."+r["MEASURE"], r["REGIME"], doc, true)
				}},
		}
		for _, q := range requetes {
			f, err := arch.Fetch(ctx, srcID, runID, q.url, ".csv")
			if err != nil {
				return nil, fmt.Errorf("%s : %w", q.nom, err)
			}
			rows, err := lireCSV(f.Path)
			if err != nil {
				return nil, fmt.Errorf("%s : %w", q.nom, err)
			}
			if len(rows) == 0 {
				return nil, fmt.Errorf("%s : réponse vide", q.nom)
			}
			for _, r := range rows {
				if err := q.lire(r, f.DocumentID); err != nil {
					return nil, fmt.Errorf("%s : %w", q.nom, err)
				}
			}
		}
		stats["indicateurs_pays"] = len(pays)

		tx, err := pool.Begin(ctx)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, `DELETE FROM core.cbcr_agregat; DELETE FROM core.fiscalite_pays`); err != nil {
			return nil, err
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "cbcr_agregat"},
			[]string{"annee", "siege", "juridiction", "mesure", "groupe_profit", "unite", "valeur", "document_id"},
			pgx.CopyFromRows(cbcr)); err != nil {
			return nil, err
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "fiscalite_pays"},
			[]string{"pays", "annee", "indicateur", "variante", "valeur", "valeur_texte", "unite", "document_id"},
			pgx.CopyFromRows(pays)); err != nil {
			return nil, err
		}
		return stats, tx.Commit(ctx)
	})
}

var composantesIDE = map[string]string{
	"T_D4P_F":   "TOTAL",
	"T_D42S_F5": "DIVIDENDES",
	"T_D43S_F5": "BENEFICES_REINVESTIS",
	"T_D4S_F5":  "REVENUS_ACTIONS",
	"T_D4Q_FL":  "INTERETS",
}

var entitesIDE = map[string]string{"ALL": "TOUTES", "RSP": "SPE", "ROU": "HORS_SPE"}

// IngestOCDEIDE charge les revenus des investissements directs de la France,
// pays de contrepartie par pays de contrepartie, entrants (versés par les
// filiales en France à leurs investisseurs étrangers) et sortants.
func IngestOCDEIDE(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	return executer(ctx, arch, SourceOCDEIDE, func(srcID, runID int64) (map[string]any, error) {
		u := urlOCDE("OECD.DAF.INV,DSD_FDI@DF_FDI_INC_CTRY,1.0",
			[]string{"FRA", "", "", "", "NET_FDI", "", "D", "S1", "", "IMC", "_T", "A", ""}, 2013)
		f, err := arch.Fetch(ctx, srcID, runID, u, ".csv")
		if err != nil {
			return nil, err
		}
		rows, err := lireCSV(f.Path)
		if err != nil {
			return nil, err
		}
		type cle struct{ annee, cp, dir, comp, ent, unite string }
		vus := map[cle]bool{}
		var lignes [][]any
		for _, r := range rows {
			comp, ok := composantesIDE[r["MEASURE"]]
			if !ok {
				return nil, fmt.Errorf("composante inconnue %q", r["MEASURE"])
			}
			ent, ok := entitesIDE[r["TYPE_ENTITY"]]
			if !ok {
				return nil, fmt.Errorf("type d'entité inconnu %q", r["TYPE_ENTITY"])
			}
			dir := map[string]string{"DI": "ENTRANT", "DO": "SORTANT"}[r["MEASURE_PRINCIPLE"]]
			if dir == "" {
				return nil, fmt.Errorf("principe %q", r["MEASURE_PRINCIPLE"])
			}
			// USD_EXC : converti en dollars ; RC : monnaie déclarée (euros
			// pour la France), qu'on nomme par son code.
			unite := r["CURRENCY"]
			if unite == "" {
				return nil, fmt.Errorf("devise absente")
			}
			v, ok, err := valeurOCDE(r)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			annee, err := strconv.Atoi(r["TIME_PERIOD"])
			if err != nil {
				return nil, fmt.Errorf("période %q", r["TIME_PERIOD"])
			}
			k := cle{r["TIME_PERIOD"], r["COUNTERPART_AREA"], dir, comp, ent, unite}
			if vus[k] {
				return nil, fmt.Errorf("doublon %v", k)
			}
			vus[k] = true
			lignes = append(lignes, []any{"FRA", annee, k.cp, dir, comp, ent, unite, v, f.DocumentID})
		}
		if len(lignes) < 10000 {
			return nil, fmt.Errorf("%d lignes seulement", len(lignes))
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, `DELETE FROM core.ide_revenu WHERE pays_declarant = 'FRA'`); err != nil {
			return nil, err
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "ide_revenu"},
			[]string{"pays_declarant", "annee", "contrepartie", "direction", "composante", "type_entite", "unite", "valeur", "document_id"},
			pgx.CopyFromRows(lignes)); err != nil {
			return nil, err
		}
		return map[string]any{"lignes": len(lignes)}, tx.Commit(ctx)
	})
}
