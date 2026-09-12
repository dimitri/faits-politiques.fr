// Package entreprises charge les comptes déposés des grandes sociétés
// françaises.
//
// UNE RÈGLE, PAS UNE LISTE. La première version suivait « le CAC 40 ». Cet
// indice est un produit propriétaire d'Euronext : sa composition est arrêtée
// par un comité d'indice, révisée trimestriellement, publiée sous les
// conditions d'Euronext, et republiée par aucune autorité publique. Une liste
// écrite à la main était donc soit une copie d'un produit commercial, soit une
// reconstitution de mémoire — et la reconstitution contenait des erreurs.
//
// La sélection se fait désormais par une requête sur la base ouverte de l'INPI :
// les sociétés ayant déposé un compte CONSOLIDÉ dont le chiffre d'affaires
// dépasse un seuil, pour un exercice donné. Le lecteur peut la rejouer.
//
// Ce qui reste indisponible en open data : les dividendes versés (seulement
// dans les rapports annuels, en PDF, société par société), l'impôt sur les
// sociétés payé (secret fiscal — la ligne figure dans la liasse déposée, dont
// l'accès en masse passe par un compte INPI) et la masse salariale (même
// raison ; l'open data ne publie que des tranches d'effectifs).
package entreprises

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ConnectorVersion = "entreprises-v2"

var Source = archive.Source{
	Slug: "inpi-ratios", Label: "Ratios financiers des entreprises (INPI / BCE)",
	Publisher: "INPI, traitement DNUM du ministère du travail", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Licence Ouverte v2.0",
	ReuseClass:  "OPEN",
	Attribution: "Source : INPI (base RNCS), traitement BCE — ministères économiques et financiers",
	Cadence:     "annuelle",
	Notes: "Ne contient ni les dividendes, ni l'impôt sur les sociétés payé, ni " +
		"la masse salariale. Mêle comptes sociaux et consolidés : voir type_bilan.",
}

var SourceGLEIF = archive.Source{
	Slug: "gleif", Label: "GLEIF — identifiants d'entités juridiques",
	Publisher: "Global Legal Entity Identifier Foundation", Tier: "PRIMARY_OFFICIAL",
	Licence:     "CC0 1.0",
	ReuseClass:  "OPEN",
	Attribution: "Source : GLEIF, Global LEI Index",
	Cadence:     "quotidienne",
	Notes: "Le champ registeredAs porte l'identifiant de registre national déclaré " +
		"par l'entité — le SIREN pour la France. Pont par identifiant, jamais par nom.",
}

const (
	ratios = "https://data.economie.gouv.fr/api/explore/v2.1/catalog/datasets/ratios_inpi_bce/records"
	gleif  = "https://api.gleif.org/api/v1/lei-records"

	// L'exercice de référence pour la sélection. Un exercice fixe est
	// nécessaire : classer sur « le dernier exercice de chacun » comparerait
	// des années différentes.
	ExerciceSelection = 2024
	// Le seuil, en euros de chiffre d'affaires consolidé. 20 milliards retient
	// une quarantaine de sociétés — un ordre de grandeur comparable à celui du
	// CAC 40, mais obtenu par une règle publique et non par un comité privé.
	SeuilCA = 20_000_000_000
)

func Ingest(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, Source)
	if err != nil {
		return err
	}
	leiSrcID, err := arch.EnsureSource(ctx, SourceGLEIF)
	if err != nil {
		return err
	}
	_ = leiSrcID
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersion)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	// 1. La sélection, par requête. Le résultat EST la liste ; il n'y a pas de
	//    fichier à maintenir.
	where := fmt.Sprintf(
		`date_cloture_exercice>=date'%d-01-01' AND date_cloture_exercice<=date'%d-12-31' `+
			`AND type_bilan="K" AND chiffre_d_affaires>%d`,
		ExerciceSelection, ExerciceSelection, SeuilCA)
	sel := ratios + "?limit=100&order_by=chiffre_d_affaires%20desc" +
		"&select=siren,chiffre_d_affaires,resultat_net,type_bilan,date_cloture_exercice" +
		"&where=" + url.QueryEscape(where)

	f, err := arch.Fetch(ctx, srcID, runID, sel, ".json")
	if err != nil {
		return fail(err)
	}
	var selection struct {
		Total   int `json:"total_count"`
		Results []struct {
			Siren string   `json:"siren"`
			CA    *float64 `json:"chiffre_d_affaires"`
			Date  string   `json:"date_cloture_exercice"`
		} `json:"results"`
	}
	if err := lireJSON(f.Path, &selection); err != nil {
		return fail(err)
	}
	if len(selection.Results) == 0 {
		return fail(fmt.Errorf("la règle de sélection ne retient aucune société : seuil ou exercice à revoir"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	for _, q := range []string{
		`DELETE FROM core.entreprise_compte`, `DELETE FROM core.entreprise`,
	} {
		if _, err := tx.Exec(ctx, q); err != nil {
			return fail(err)
		}
	}

	critere := fmt.Sprintf("compte consolidé (type_bilan=K) de l'exercice %d, chiffre d'affaires supérieur à %d Md€",
		ExerciceSelection, SeuilCA/1_000_000_000)

	var nSoc, nEx, nLEI int
	for _, s := range selection.Results {
		nom, err := raisonSociale(ctx, arch, srcID, runID, s.Siren)
		if err != nil {
			return fail(err)
		}
		lei := leiDe(ctx, arch, srcID, runID, s.Siren)

		var ca float64
		if s.CA != nil {
			ca = *s.CA
		}
		verif := fmt.Sprintf("retenue sur son compte consolidé clos le %s : %.1f Md€ de chiffre d'affaires",
			s.Date, ca/1e9)

		if _, err := tx.Exec(ctx, `
			INSERT INTO core.entreprise (siren, nom, portee, verification, critere, lei)
			VALUES ($1,$2,'GROUPE',$3,$4,$5)
			ON CONFLICT (siren) DO NOTHING`,
			s.Siren, nom, verif, critere, nul(lei)); err != nil {
			return fail(fmt.Errorf("%s : %w", s.Siren, err))
		}
		nSoc++
		if lei != "" {
			nLEI++
		}

		// Tous les exercices publiés, consolidés comme sociaux : c'est la série
		// qui permet de situer une société dans le temps. La portée de chaque
		// ligne reste lisible par type_bilan.
		q := ratios + "?limit=100&order_by=date_cloture_exercice%20desc&where=" +
			url.QueryEscape(`siren="`+s.Siren+`"`)
		fx, err := arch.Fetch(ctx, srcID, runID, q, ".json")
		if err != nil {
			return fail(err)
		}
		var doc struct {
			Results []struct {
				Date            string   `json:"date_cloture_exercice"`
				TypeBilan       string   `json:"type_bilan"`
				Confidentiality string   `json:"confidentiality"`
				CA              *float64 `json:"chiffre_d_affaires"`
				Marge           *float64 `json:"marge_brute"`
				EBE             *float64 `json:"ebe"`
				EBIT            *float64 `json:"ebit"`
				RN              *float64 `json:"resultat_net"`
			} `json:"results"`
		}
		if err := lireJSON(fx.Path, &doc); err != nil {
			return fail(err)
		}
		for _, r := range doc.Results {
			if len(r.Date) < 10 {
				continue
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO core.entreprise_compte
				  (siren, date_cloture, type_bilan, chiffre_affaires, marge_brute,
				   ebe, ebit, resultat_net, confidentialite, source_id)
				VALUES ($1,$2::date,$3,$4,$5,$6,$7,$8,$9,$10)
				ON CONFLICT (siren, date_cloture) DO NOTHING`,
				s.Siren, r.Date[:10], nul(r.TypeBilan), r.CA, r.Marge, r.EBE, r.EBIT,
				r.RN, nul(r.Confidentiality), srcID); err != nil {
				return fail(fmt.Errorf("%s %s : %w", s.Siren, r.Date, err))
			}
			nEx++
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"societes": nSoc, "exercices": nEx, "lei": nLEI}, "")
	fmt.Printf("  Entreprises : %d sociétés retenues par la règle, %d exercices, %d LEI appariés\n",
		nSoc, nEx, nLEI)
	fmt.Printf("  Règle : %s\n", critere)
	return nil
}

// raisonSociale interroge le répertoire public des entreprises PAR SIREN. Le
// sens de la requête compte : du SIREN vers le nom, jamais du nom vers le
// SIREN — c'est la recherche par nom qui produisait de mauvaises entités.
func raisonSociale(ctx context.Context, arch *archive.Archive, srcID, runID int64, siren string) (string, error) {
	f, err := arch.Fetch(ctx, srcID, runID,
		"https://recherche-entreprises.api.gouv.fr/search?per_page=1&q="+siren, ".json")
	if err != nil {
		return "", err
	}
	var doc struct {
		Results []struct {
			Siren   string `json:"siren"`
			Nom     string `json:"nom_raison_sociale"`
			Complet string `json:"nom_complet"`
		} `json:"results"`
	}
	if err := lireJSON(f.Path, &doc); err != nil {
		return "", err
	}
	for _, r := range doc.Results {
		if r.Siren == siren {
			if r.Nom != "" {
				return r.Nom, nil
			}
			return r.Complet, nil
		}
	}
	return "SIREN " + siren, nil
}

// leiDe cherche le LEI dont GLEIF déclare qu'il est enregistré sous ce SIREN.
// Appariement par identifiant : le champ registeredAs est renseigné par
// l'entité elle-même et validé par l'émetteur du LEI. Un LEI absent n'est pas
// une erreur — toutes les sociétés n'en ont pas.
func leiDe(ctx context.Context, arch *archive.Archive, srcID, runID int64, siren string) string {
	// GLEIF écrit le SIREN tantôt compact, tantôt par groupes de trois.
	for _, forme := range []string{siren, siren[:3] + " " + siren[3:6] + " " + siren[6:]} {
		f, err := arch.Fetch(ctx, srcID, runID,
			gleif+"?page%5Bsize%5D=5&filter%5Bentity.registeredAs%5D="+url.QueryEscape(forme), ".json")
		if err != nil {
			continue
		}
		var doc struct {
			Data []struct {
				Attributes struct {
					LEI    string `json:"lei"`
					Entity struct {
						RegisteredAs string `json:"registeredAs"`
						LegalAddress struct {
							Country string `json:"country"`
						} `json:"legalAddress"`
					} `json:"entity"`
				} `json:"attributes"`
			} `json:"data"`
		}
		if err := lireJSON(f.Path, &doc); err != nil {
			continue
		}
		for _, d := range doc.Data {
			if d.Attributes.Entity.LegalAddress.Country == "FR" {
				return d.Attributes.LEI
			}
		}
	}
	return ""
}

func lireJSON(path string, v any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func nul(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
