package communes

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Qui paie, via quel mécanisme fiscal nommé — pas seulement quel niveau de
// collectivité reçoit (ofgl.impots_taxes_par_hab, déjà chargé, ne dit pas
// « ménages » ou « entreprises »). REI (Registre des Éléments d'Imposition,
// DGFiP), diffusé par l'OFGL sur le même portail Opendatasoft que
// core.commune_indicator. Voir docs/collectivites-donnees.md.
var SourceFiscaliteLocale = archive.Source{
	Slug: "ofgl-fiscalite-directe-locale", Label: "OFGL — fiscalité directe locale (REI/DGFiP)",
	Publisher: "Observatoire des finances et de la gestion publique locales (REI, DGFiP)",
	Tier:      "PRIMARY_OFFICIAL",
	Licence:   "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : DGFiP (Registre des éléments d'imposition), diffusion OFGL",
	Cadence:     "annuelle",
	Notes: "Produit réel du foncier bâti, du foncier non bâti, de la CFE et de la TASCOM, " +
		"pour les seuls destinataires commune et intercommunalité (bloc communal) — pas " +
		"département/région, qui n'en reçoivent plus depuis les réformes 2020-2021, pas " +
		"l'État (frais de gestion), pas les syndicats ni les chambres consulaires. Un seul " +
		"var REI par dispositif × destinataire retenu : le REI publie aussi des sous-totaux " +
		"imbriqués sous le même dispositif et la même catégorie (ex. la CFE intercommunale " +
		"P33 se décompose en P33_1+P33_2, eux-mêmes sous-décomposés en P33_2U/P33_2Z/P33_3) " +
		"qui NE S'ADDITIONNENT PAS au total dont ils sont extraits — un premier calcul naïf " +
		"(somme de tous les var d'un même dispositif/destinataire) avait donné 21,9 Md€ de " +
		"CFE intercommunale contre 7,3 Md€ réels, confirmés par la seule variable de tête " +
		"(P33). IFER, taxe d'habitation résiduelle, TEOM et les surtaxes GEMAPI/TSE/CHAMBRE " +
		"ne sont pas chargés ici : IFER en particulier répète la même valeur régionale sur " +
		"CHAQUE commune membre de la région (vérifié directement — un agrégat national naïf " +
		"y serait faux de plusieurs ordres de grandeur, pas seulement imprécis) ; les autres " +
		"sortent du strict périmètre ménages/entreprises visé par ce chargement.",
}

const urlFiscaliteLocale = "https://data.ofgl.fr/api/explore/v2.1/catalog/datasets/rei/records?" +
	"select=annee,dispositif_fiscal,destinataire,var,sum(valeur)%20as%20total" +
	"&where=var%20in(%22E13%22,%22E33%22,%22B13%22,%22B33%22,%22P13%22,%22P33%22,%22TASCOMcom%22,%22TASCOMgfp%22)" +
	"&group_by=annee,dispositif_fiscal,destinataire,var&limit=50"

// categoriePayeur : foncier bâti et non bâti sont assis sur la propriété
// (ménages, très majoritairement) ; CFE et TASCOM sont assis sur l'activité
// économique (entreprises) — la même distinction que docs/collectivites-donnees.md.
var categoriePayeur = map[string]string{
	"FB": "MENAGES", "FNB": "MENAGES",
	"CFE": "ENTREPRISES", "TASCOM": "ENTREPRISES",
}

var destinataireCode = map[string]string{"Commune": "COMMUNE", "GFP": "GFP"}

func IngestFiscaliteDirecteLocale(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceFiscaliteLocale)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersion)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, urlFiscaliteLocale, ".json")
	if err != nil {
		return fail(err)
	}
	b, err := os.ReadFile(f.Path)
	if err != nil {
		return fail(err)
	}
	var rep struct {
		Results []struct {
			Annee            string  `json:"annee"`
			DispositifFiscal string  `json:"dispositif_fiscal"`
			Destinataire     string  `json:"destinataire"`
			Var              string  `json:"var"`
			Total            float64 `json:"total"`
		} `json:"results"`
	}
	if err := json.Unmarshal(b, &rep); err != nil {
		return fail(err)
	}
	if len(rep.Results) == 0 {
		return fail(fmt.Errorf("fiscalité directe locale : réponse vide"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM core.fiscalite_directe_locale`); err != nil {
		return fail(err)
	}

	var rows [][]any
	for _, r := range rep.Results {
		cat, ok := categoriePayeur[r.DispositifFiscal]
		if !ok {
			return fail(fmt.Errorf("dispositif fiscal inattendu : %q (var %q) — le format REI a peut-être changé",
				r.DispositifFiscal, r.Var))
		}
		dest, ok := destinataireCode[r.Destinataire]
		if !ok {
			return fail(fmt.Errorf("destinataire inattendu : %q (var %q) — le format REI a peut-être changé",
				r.Destinataire, r.Var))
		}
		annee, err := strconv.Atoi(r.Annee)
		if err != nil {
			return fail(fmt.Errorf("année %q illisible : %w", r.Annee, err))
		}
		rows = append(rows, []any{annee, r.DispositifFiscal, cat, dest, r.Total, srcID})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "fiscalite_directe_locale"},
		[]string{"annee", "dispositif", "categorie_payeur", "destinataire", "montant_eur", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(fmt.Errorf("fiscalite_directe_locale : %w", err))
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": len(rows)}, "")
	fmt.Printf("  fiscalité directe locale (OFGL/REI) : %d lignes\n", len(rows))
	return nil
}
