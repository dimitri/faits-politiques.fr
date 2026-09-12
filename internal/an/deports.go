package an

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Les déports sont déjà scellés dans raw.record : rien à télécharger.
//
// Un déport n'est pas un manquement, c'est sa prévention — le député signale
// lui-même un lien qui pourrait créer un conflit et s'abstient de participer.
// L'explication est transcrite dans ses mots, jamais résumée.
var reBaliseHTML = regexp.MustCompile(`(?s)<[^>]+>`)

func NormalizeDeports(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT ON (natural_key) payload FROM raw.record
		 WHERE record_type = 'an.deport'
		 ORDER BY natural_key, extracted_at DESC, id DESC`)
	if err != nil {
		return err
	}
	type deport struct {
		UID   string `json:"uid"`
		Cible struct {
			Type struct {
				Code    string `json:"code"`
				Libelle string `json:"libelle"`
			} `json:"type"`
			ReferenceTextuelle string `json:"referenceTextuelle"`
		} `json:"cible"`
		Portee struct {
			Code    string `json:"code"`
			Libelle string `json:"libelle"`
		} `json:"portee"`
		Instance struct {
			Libelle string `json:"libelle"`
		} `json:"instance"`
		RefActeur       string `json:"refActeur"`
		Explication     string `json:"explication"`
		Legislature     string `json:"legislature"`
		DateCreation    string `json:"dateCreation"`
		DatePublication string `json:"datePublication"`
	}
	var ds []deport
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return err
		}
		var d deport
		if err := json.Unmarshal(raw, &d); err == nil && d.UID != "" {
			ds = append(ds, d)
		}
	}
	rows.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM core.deport`); err != nil {
		return err
	}

	var n, sansActeur int
	for _, d := range ds {
		var pid int64
		err := tx.QueryRow(ctx, `
			SELECT person_id FROM core.person_identifier
			 WHERE scheme = 'AN_ACTEUR' AND value = $1`, d.RefActeur).Scan(&pid)
		if err != nil {
			// Un déport d'une législature antérieure peut viser un acteur que
			// nous n'avons pas. Il est compté, pas rattaché de force.
			sansActeur++
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.deport
			  (source_uid, person_id, legislature, cible_type, cible_libelle,
			   portee_code, portee_libelle, instance, explication,
			   date_creation, date_publication)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,
			        nullif($10,'')::timestamptz, nullif($11,'')::timestamptz)
			ON CONFLICT (source_uid) DO NOTHING`,
			d.UID, pid, nul(d.Legislature), nul(d.Cible.Type.Libelle),
			nul(texteBrut(d.Cible.ReferenceTextuelle)), nul(d.Portee.Code),
			nul(d.Portee.Libelle), nul(d.Instance.Libelle),
			nul(texteBrut(d.Explication)), d.DateCreation, d.DatePublication,
		); err != nil {
			return fmt.Errorf("déport %s : %w", d.UID, err)
		}
		n++
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	fmt.Printf("  déports        %d (%d sans acteur connu)\n", n, sansActeur)
	return nil
}

// texteBrut retire le balisage HTML que l'Assemblée met dans les explications,
// et rend les entités. Le texte reste celui du député, mot pour mot.
func texteBrut(s string) string {
	s = reBaliseHTML.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	return strings.Join(strings.Fields(s), " ")
}

func nul(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
