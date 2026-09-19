package macro

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

var SourceTraficPortuaireEurope = archive.Source{
	Slug: "eurostat-mar-go-aa", Label: "Eurostat, mar_go_aa — trafic portuaire, six ports nommés",
	Publisher: "Eurostat", Tier: "PRIMARY_OFFICIAL",
	Licence: "Eurostat (réutilisation libre avec attribution)", ReuseClass: "OPEN",
	Attribution: "Source : Eurostat, mar_go_aa",
	Cadence:     "annuelle",
	Notes: "Six ports nommés (France, rang nord-européen), pas tous les ports du continent — " +
		"même logique de sélection nommée que pour SIPRI ou le PIB des blocs déjà chargés.",
}

// portEurope : code Eurostat (rep_mar, avec son préfixe pays), libellé,
// pays — vérifié directement dans le référentiel Eurostat REP_MAR (les
// codes ont un préfixe pays qui fait partie de la valeur de dimension,
// pas un simple identifiant de port).
var portsEurope = []struct{ code, label, pays string }{
	{"FR_1FR001", "HAROPA (Le Havre, Rouen)", "FR"},
	{"FR_1FRLEH", "Le Havre (avant fusion HAROPA)", "FR"},
	{"FR_2FRMRS", "Marseille", "FR"},
	{"NL_0NLRTM", "Rotterdam", "NL"},
	{"BE_0BEANR", "Antwerpen (Anvers)", "BE"},
	{"DE_1DEHAM", "Hamburg", "DE"},
}

const urlMarGoAA = "https://ec.europa.eu/eurostat/api/dissemination/statistics/1.0/data/mar_go_aa?format=JSON&lang=EN&direct=TOTAL&unit=THS_T" +
	"&rep_mar=FR_1FR001&rep_mar=FR_1FRLEH&rep_mar=FR_2FRMRS&rep_mar=NL_0NLRTM&rep_mar=BE_0BEANR&rep_mar=DE_1DEHAM"

type jsonStatMar struct {
	Value     map[string]float64 `json:"value"`
	Dimension struct {
		RepMar struct {
			Category struct {
				Index map[string]int `json:"index"`
			} `json:"category"`
		} `json:"rep_mar"`
		Time struct {
			Category struct {
				Index map[string]int `json:"index"`
			} `json:"category"`
		} `json:"time"`
	} `json:"dimension"`
	Size []int `json:"size"`
}

// IngestTraficPortuaireEurope charge, pour six ports nommés (France, Pays-Bas,
// Belgique, Allemagne), le trafic total annuel (Eurostat mar_go_aa). Voir
// docs/ports-donnees.md.
func IngestTraficPortuaireEurope(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceTraficPortuaireEurope)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "trafic-portuaire-europe-v1")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, urlMarGoAA, ".json")
	if err != nil {
		return fail(err)
	}
	raw, err := os.ReadFile(f.Path)
	if err != nil {
		return fail(err)
	}
	var doc jsonStatMar
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fail(fmt.Errorf("JSON-stat illisible : %w", err))
	}
	if len(doc.Size) != 5 {
		return fail(fmt.Errorf("forme inattendue (%d dimensions, 5 attendues)", len(doc.Size)))
	}
	nTime := doc.Size[4]

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM core.trafic_portuaire_europe`); err != nil {
		return fail(err)
	}

	var n int
	for _, p := range portsEurope {
		iPort, ok := doc.Dimension.RepMar.Category.Index[p.code]
		if !ok {
			return fail(fmt.Errorf("%s : code absent de la réponse Eurostat", p.code))
		}
		for anneeStr, iTime := range doc.Dimension.Time.Category.Index {
			// JSON-stat : indice à plat = ((0*1 + 0)*1 + 0)*1*nPortsIndex... —
			// avec un seul filtre par dimension sauf rep_mar (6) et time (nTime),
			// l'indice à plat est iPort*nTime + iTime (les trois dimensions
			// freq/direct/unit ont chacune une seule valeur ici).
			idx := iPort*nTime + iTime
			v, ok := doc.Value[fmt.Sprint(idx)]
			if !ok {
				continue // valeur manquante pour ce port/cette année, pas une erreur
			}
			var annee int
			if _, err := fmt.Sscanf(anneeStr, "%d", &annee); err != nil {
				return fail(fmt.Errorf("année illisible : %q", anneeStr))
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO core.trafic_portuaire_europe (code_port, port_label, pays_code, annee, tonnage_milliers, source_id)
				VALUES ($1,$2,$3,$4,$5,$6)`, p.code, p.label, p.pays, annee, v, srcID); err != nil {
				return fail(fmt.Errorf("%s %d : insertion : %w", p.code, annee, err))
			}
			n++
		}
	}
	if n == 0 {
		return fail(fmt.Errorf("aucune valeur chargée"))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": n}, "")
	fmt.Printf("  Trafic portuaire européen (Eurostat) : %d lignes, %d ports\n", n, len(portsEurope))
	return nil
}
