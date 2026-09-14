package aides

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Les aides financières de l'ADEME, publiées au format SCDL (décret 2017-779)
// sans seuil de montant, avec le SIRET du bénéficiaire.
var SourceADEME = archive.Source{
	Slug: "ademe-aides-financieres", Label: "ADEME — aides financières attribuées (format SCDL)",
	Publisher: "ADEME", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Licence Ouverte v2.0",
	ReuseClass:  "OPEN",
	Attribution: "Source : ADEME, les aides financières de l'ADEME (data.ademe.fr)",
	Cadence:     "quotidienne",
	Notes: "Montants ENGAGÉS (conventions), pas versés. Des intermédiaires publics reçoivent des fonds " +
		"qu'ils reversent (ASP : plusieurs centaines de millions en deux dossiers) : les isoler par la " +
		"catégorie juridique avant toute répartition. Référence de décision non unique : la ligne est " +
		"identifiée par son identifiant data-fair.",
}

// Le registre public des aides de minimis tenu par la DGE depuis le 1er
// janvier 2026 (décret 2025-1361), toutes autorités confondues.
var SourceMinimis = archive.Source{
	Slug: "dge-registre-minimis", Label: "DGE — registre public des aides de minimis",
	Publisher: "Direction générale des entreprises", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Licence non renseignée dans les métadonnées ; publication prévue par le décret 2025-1361",
	ReuseClass:  "RESTRICTED",
	Attribution: "Source : DGE, registre public des aides de minimis (data.economie.gouv.fr)",
	Cadence:     "quotidienne",
	Notes: "Aides accordées depuis le 1er janvier 2026 (2027 pour l'agriculture), plafonnées à 300 k€ " +
		"sur trois ans : le registre mesure le nombre de bénéficiaires, pas la concentration des " +
		"montants. Montants en équivalent-subvention brut. Nom masqué (« - ») pour une partie des " +
		"lignes. Licence à confirmer avant tout export ouvert.",
}

func dateISO(s string) *time.Time {
	s = strings.TrimSpace(s)
	if len(s) >= 10 {
		if t, err := time.Parse("2006-01-02", s[:10]); err == nil {
			return &t
		}
	}
	return nil
}

func chaine(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return fmt.Sprint(v)
}

func montantJSON(v any) (*float64, error) {
	switch x := v.(type) {
	case nil:
		return nil, nil
	case float64:
		return &x, nil
	case string:
		m, ok := montantPoint(x)
		if !ok {
			return nil, fmt.Errorf("montant illisible %q", x)
		}
		return m, nil
	}
	return nil, fmt.Errorf("montant de type inattendu %T", v)
}

const ademeLignes = "https://data.ademe.fr/data-fair/api/v1/datasets/les-aides-financieres-de-l'ademe/lines?size=10000"

func IngestADEME(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	return executerAides(ctx, pool, arch, SourceADEME, "ADEME", func(srcID, runID int64) ([]aide, map[string]any, error) {
		var out []aide
		total, pages := -1, 0
		for u := ademeLignes; u != ""; {
			f, err := arch.Fetch(ctx, srcID, runID, u, ".json")
			if err != nil {
				return nil, nil, err
			}
			b, err := os.ReadFile(f.Path)
			if err != nil {
				return nil, nil, err
			}
			var page struct {
				Total   int              `json:"total"`
				Next    string           `json:"next"`
				Results []map[string]any `json:"results"`
			}
			if err := json.Unmarshal(b, &page); err != nil {
				return nil, nil, fmt.Errorf("page %d : %w", pages+1, err)
			}
			// La page vide qui suit la dernière annonce un total de 0 : seul
			// celui de la première page fait foi.
			if pages == 0 {
				total = page.Total
			}
			pages++
			for _, r := range page.Results {
				m, err := montantJSON(r["montant"])
				if err != nil {
					return nil, nil, fmt.Errorf("%s : %w", chaine(r["_id"]), err)
				}
				out = append(out, aide{
					reference: chaine(r["_id"]), identifiant: chaine(r["idBeneficiaire"]), nom: chaine(r["nomBeneficiaire"]),
					regime: chaine(r["dispositifAide"]), intitule: chaine(r["objet"]), instrument: chaine(r["nature"]),
					secteur:  chaine(r["_siret_infos.activitePrincipaleEtablissementNAFRev2Libelle"]),
					region:   chaine(r["_siret_infos._infos_commune.nom_departement"]),
					autorite: chaine(r["Nom_de_l_attribuant"]), nominal: m, dateOctroi: dateISO(chaine(r["dateConvention"])),
					document: f.DocumentID,
				})
			}
			if len(page.Results) == 0 {
				break
			}
			u = page.Next
		}
		if len(out) != total {
			return nil, nil, fmt.Errorf("%d lignes lues pour %d annoncées", len(out), total)
		}
		return out, map[string]any{"pages": pages}, nil
	})
}

const minimisExport = "https://data.economie.gouv.fr/api/explore/v2.1/catalog/datasets/aides_minimis/exports/json?order_by=index"

func IngestMinimis(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	return executerAides(ctx, pool, arch, SourceMinimis, "MINIMIS", func(srcID, runID int64) ([]aide, map[string]any, error) {
		f, err := arch.Fetch(ctx, srcID, runID, minimisExport, ".json")
		if err != nil {
			return nil, nil, err
		}
		b, err := os.ReadFile(f.Path)
		if err != nil {
			return nil, nil, err
		}
		var recs []map[string]any
		if err := json.Unmarshal(b, &recs); err != nil {
			return nil, nil, err
		}
		var out []aide
		for _, r := range recs {
			m, err := montantJSON(r["montant_esb"])
			if err != nil {
				return nil, nil, fmt.Errorf("ligne %s : %w", chaine(r["index"]), err)
			}
			out = append(out, aide{
				reference: chaine(r["index"]), identifiant: chaine(r["identifiant_beneficiaire"]), nom: chaine(r["nom_beneficiaire"]),
				regime: chaine(r["regime"]), instrument: chaine(r["instrument_aide"]), secteur: chaine(r["secteur_aide"]),
				region: chaine(r["commune_beneficiaire"]), autorite: chaine(r["autorite"]), operateur: chaine(r["operateur"]),
				esb: m, dateOctroi: dateISO(chaine(r["date_octroi"])), datePublication: dateISO(chaine(r["date_publication"])),
				document: f.DocumentID,
			})
		}
		return out, nil, nil
	})
}
