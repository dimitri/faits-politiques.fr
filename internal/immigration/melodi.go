// Package immigration charge la population immigrée et la population
// étrangère — deux notions distinctes, voir le commentaire de
// db/migrations/0071_immigration.sql — ainsi que leur comparaison
// européenne et les titres de séjour délivrés.
package immigration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/faits-politiques/faits-politiques/internal/archive"
)

const ConnectorVersion = "immigration-v1"

var SourceMelodiImmigration = archive.Source{
	Slug: "insee-rp-immigration-nationalite", Label: "Insee — recensement, immigration et nationalité",
	Publisher: "INSEE", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Insee, recensement de la population, diffusion Melodi",
	Cadence:     "annuelle",
	Notes: "Immigré (né étranger à l'étranger, quelle que soit la nationalité actuelle) et " +
		"étranger (nationalité étrangère actuelle, quel que soit le lieu de naissance) " +
		"sont deux classifications distinctes qui se recoupent partiellement : 34 % des " +
		"immigrés ont la nationalité française en 2023 (Insee).",
}

type melodiObs struct {
	Dimensions map[string]string `json:"dimensions"`
	Measures   struct {
		OBSVALUENIVEAU struct {
			Value float64 `json:"value"`
		} `json:"OBS_VALUE_NIVEAU"`
	} `json:"measures"`
}

type melodiReponse struct {
	Observations []melodiObs `json:"observations"`
}

// melodiLire télécharge et parse une réponse Melodi. Partagé avec le paquet
// macro (internal/macro/menages_effectif.go) sans dépendance croisée : deux
// implémentations d'une trentaine de lignes valent mieux qu'un paquet utilitaire
// pour ce seul appel HTTP.
func melodiLire(ctx context.Context, arch *archive.Archive, srcID, runID int64, url string) ([]melodiObs, error) {
	f, err := arch.Fetch(ctx, srcID, runID, url, ".json")
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(f.Path)
	if err != nil {
		return nil, err
	}
	var r melodiReponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("réponse Melodi illisible : %w", err)
	}
	if len(r.Observations) == 0 {
		return nil, fmt.Errorf("réponse Melodi vide (%s)", url)
	}
	return r.Observations, nil
}

var sexeLib = map[string]string{"F": "FEMME", "M": "HOMME", "_T": "TOTAL"}

var empstaLib = map[string]string{
	"1": "ACTIF_OCCUPE", "2": "CHOMEUR", "31": "RETRAITE", "33": "ETUDIANT",
	"35": "AU_FOYER", "36": "AUTRE_INACTIF", "_T": "TOTAL",
}

var ageLib = map[string]string{
	"Y_LT15": "MOINS_15", "Y15T24": "15_24", "Y25T54": "25_54", "Y_GE55": "55_PLUS",
	"Y_GE15": "15_PLUS", "_T": "TOUS_AGES",
}
