package numerique

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var SourceSILL = archive.Source{
	Slug: "sill-code-gouv", Label: "Socle interministériel de logiciels libres (SILL)",
	Publisher:   "Direction interministérielle du numérique (DINUM), code.gouv.fr",
	Tier:        "PRIMARY_OFFICIAL",
	Licence:     "Données publiques de la DINUM, citées avec lien",
	ReuseClass:  "ATTRIBUTION",
	Attribution: "Source : DINUM, socle interministériel de logiciels libres (code.gouv.fr/sill)",
	Cadence:     "mise à jour continue",
	Notes: "Catalogue des logiciels libres recommandés par des agents publics, avec les organisations qui s'en " +
		"déclarent utilisatrices ou référentes et les prestataires de service déclarés. Usage déclaré, pas " +
		"inventaire des déploiements.",
}

const urlSILL = "https://code.gouv.fr/sill/api/sill.json"

func IngestSILL(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	return executer(ctx, arch, SourceSILL, func(srcID, runID int64) (map[string]any, error) {
		f, err := arch.Fetch(ctx, srcID, runID, urlSILL, ".json")
		if err != nil {
			return nil, err
		}
		b, err := os.ReadFile(f.Path)
		if err != nil {
			return nil, err
		}
		var logiciels []struct {
			ID                   int      `json:"id"`
			Name                 string   `json:"name"`
			License              string   `json:"license"`
			ReferencedSinceTime  int64    `json:"referencedSinceTime"`
			IsStillInObservation bool     `json:"isStillInObservation"`
			Categories           []string `json:"categories"`
			CustomAttributes     struct {
				IsFromFrenchPublicService  bool `json:"isFromFrenchPublicService"`
				IsPresentInSupportContract bool `json:"isPresentInSupportContract"`
			} `json:"customAttributes"`
			ServiceProviders []json.RawMessage `json:"serviceProviders"`
			ParOrganisation  map[string]struct {
				ReferentCount int `json:"referentCount"`
				UserCount     int `json:"userCount"`
			} `json:"userAndReferentCountByOrganization"`
		}
		if err := json.Unmarshal(b, &logiciels); err != nil {
			return nil, err
		}
		// Plus de 600 logiciels en 2026 : moins de 300 signalerait un format
		// changé ou une réponse tronquée.
		if len(logiciels) < 300 {
			return nil, fmt.Errorf("%d logiciels seulement", len(logiciels))
		}
		var lignes [][]any
		publics, support := 0, 0
		for _, l := range logiciels {
			util, ref := 0, 0
			for _, o := range l.ParOrganisation {
				util += o.UserCount
				ref += o.ReferentCount
			}
			var depuis any
			if l.ReferencedSinceTime > 0 {
				depuis = time.UnixMilli(l.ReferencedSinceTime).UTC()
			}
			if l.CustomAttributes.IsFromFrenchPublicService {
				publics++
			}
			if l.CustomAttributes.IsPresentInSupportContract {
				support++
			}
			lignes = append(lignes, []any{l.ID, l.Name, nul(l.License), depuis, l.IsStillInObservation,
				l.CustomAttributes.IsFromFrenchPublicService, l.CustomAttributes.IsPresentInSupportContract,
				len(l.ParOrganisation), util, ref, len(l.ServiceProviders), l.Categories, f.DocumentID})
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, `DELETE FROM core.sill_logiciel`); err != nil {
			return nil, err
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "sill_logiciel"},
			[]string{"id", "nom", "licence", "reference_depuis", "en_observation", "issu_service_public", "contrat_support",
				"organisations", "utilisateurs", "referents", "prestataires", "categories", "document_id"},
			pgx.CopyFromRows(lignes)); err != nil {
			return nil, err
		}
		return map[string]any{"logiciels": len(lignes), "issus_service_public": publics, "contrat_support": support}, tx.Commit(ctx)
	})
}
