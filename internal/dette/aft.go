package dette

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Ce que l'AFT rend compte au Parlement, faute de pouvoir lire son site : les
// indicateurs de performance du programme 117 « Charge de la dette et
// trésorerie de l'État », publiés dans les rapports annuels de performance
// sur data.economie.gouv.fr. On n'en retient que la demande des investisseurs
// aux adjudications — combien d'offres pour chaque euro emprunté, et combien
// d'adjudications n'ont pas trouvé preneur —, la seule mesure publique et
// officielle de l'appétit du marché pour la dette française.
var SourceAFT = archive.Source{
	Slug: "aft-p117-performance", Label: "Programme 117 (AFT) — indicateurs de performance des adjudications",
	Publisher: "Direction du budget, d'après l'Agence France Trésor", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Licence Ouverte v2.0",
	ReuseClass:  "OPEN",
	Attribution: "Source : rapport annuel de performance, programme 117, data.economie.gouv.fr",
	Cadence:     "annuelle (RAP, juin)",
	Notes: "Taux de couverture en % : 297 signifie 2,97 € d'offres pour 1 € adjugé. Un « - » du " +
		"RAP n'est pas chargé : il peut signifier zéro comme « sans objet », et le tableau ne " +
		"permet pas de trancher. Colonnes par année figées dans le schéma du jeu : un nouveau " +
		"millésime demande un nouveau jeu et un nouveau mappage.",
}

const aftJeu = "performance-de-la-depense-rap-2025"

var aftIndicateurs = map[string]bool{"P117-1-1": true, "P117-1-2": true}

func IngestAFT(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	return executer(ctx, pool, arch, SourceAFT, func(srcID, runID int64) (*lot, error) {
		q := url.Values{"where": {"code_programme=117"}, "order_by": {"code_externe_indicateur,ordre_ssi"}}
		u := "https://data.economie.gouv.fr/api/explore/v2.1/catalog/datasets/" + aftJeu + "/exports/json?" + q.Encode()
		f, err := arch.Fetch(ctx, srcID, runID, u, ".json")
		if err != nil {
			return nil, err
		}
		var lignes []map[string]any
		if err := lireJSON(f.Path, &lignes); err != nil {
			return nil, err
		}
		l := nouveauLot()
		for _, ln := range lignes {
			code, _ := ln["code_externe_indicateur"].(string)
			if !aftIndicateurs[code] {
				continue
			}
			sous, _ := ln["libelle_sous_indicateur"].(string)
			unite, _ := ln["unite"].(string)
			s := &Serie{
				Code: "aft-p117:" + code + ":" + slug(sous), CodeSource: aftJeu + "#" + code,
				Libelle: sous, Pays: "FR", Frequence: "A", Concept: "ADJUDICATIONS_AFT",
				SecteurEmetteur: "S13111", URL: u,
			}
			switch unite {
			case "%":
				s.Unite, s.Mesure = "PCT", "TAUX"
			case "Nb":
				s.Unite, s.Mesure = "NOMBRE", "NOMBRE"
			default:
				return nil, fmt.Errorf("%s : unité %q inattendue", code, unite)
			}
			var obs []Obs
			for _, annee := range []string{"2023", "2024", "2025"} {
				brut, _ := ln["exec_"+annee].(string)
				v, ok := nombreRAP(brut)
				if !ok {
					continue
				}
				obs = append(obs, Obs{Periode: annee, Valeur: v, DocumentID: f.DocumentID})
			}
			l.ajouter(s, obs)
		}
		if len(l.series) == 0 {
			return nil, fmt.Errorf("aucun indicateur d'adjudication dans %s", aftJeu)
		}
		return l, nil
	})
}

// nombreRAP lit les nombres des RAP : espaces insécables comme séparateurs de
// milliers, virgule décimale, « - » pour une case vide.
func nombreRAP(s string) (float64, bool) {
	s = strings.NewReplacer("\u202f", "", "\u00a0", "", " ", "", ",", ".").Replace(strings.TrimSpace(s))
	if s == "" || s == "-" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}
