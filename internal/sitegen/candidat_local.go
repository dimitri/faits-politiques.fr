package sitegen

import (
	"context"
	"encoding/csv"
	"os"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Rapprocher un candidat de sa fiche au répertoire national des élus.
//
// Les deux sources n'ont pas d'identifiant commun, et un homonyme unique n'est
// PAS une preuve : « Nathalie ARTHAUD » n'a qu'un homonyme au RNE, et c'est une
// conseillère municipale de Limey-Remenauville. Le rapprochement est donc écrit
// à la main, ligne à ligne, dans data/candidats-mandats-locaux.csv, avec sa
// vérification et sa source — les refus compris.
type TermLocal struct {
	Type, Role, Place, Depuis        string
	TypeCode, Municipality, District string
	Places                           []Place
}

type ReconciliationRNE struct {
	Status       string
	Verification string
	Source       string
	Terms        []TermLocal
}

func loadTermsPremises(ctx context.Context, pool *pgxpool.Pool, path string) (
	map[string]*ReconciliationRNE, error) {

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]*ReconciliationRNE{}, nil
		}
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comment = '#'
	r.FieldsPerRecord = -1
	recs, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	out := map[string]*ReconciliationRNE{}
	var ids []int64
	for i, rec := range recs {
		if i == 0 || len(rec) < 4 {
			continue
		}
		slug := strings.TrimSpace(rec[0])
		id, _ := strconv.ParseInt(strings.TrimSpace(rec[1]), 10, 64)
		rp := &ReconciliationRNE{
			Status: strings.TrimSpace(rec[2]), Verification: strings.TrimSpace(rec[3]),
		}
		if len(rec) > 4 {
			rp.Source = strings.TrimSpace(rec[4])
		}
		out[slug] = rp
		if rp.Status == "RETENU" && id != 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return out, nil
	}

	// Une requête pour tous les id retenus, plutôt qu'une par ligne du CSV —
	// une vingtaine de rapprochements aujourd'hui, mais le même patron N+1
	// qu'ailleurs dans ce fichier.
	rows, err := pool.Query(ctx, `
		SELECT m.person_id, m.mandate_type::text, coalesce(m.role,''),
		       coalesce(c.nom_clair, m.constituency, ''),
		       to_char(lower(m.validity),'DD/MM/YYYY'),
		       coalesce(m.commune_code,''), coalesce(m.constituency,'')
		FROM core.mandate m
		LEFT JOIN ref.commune c ON c.code_insee=m.commune_code
		 AND c.cog_millesime=(SELECT max(cog_millesime) FROM ref.commune)
		WHERE m.person_id = ANY($1) AND upper(m.validity) IS NULL
		ORDER BY m.person_id, m.mandate_type`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	termsPerID := map[int64][]TermLocal{}
	for rows.Next() {
		var pid int64
		var m TermLocal
		if err := rows.Scan(&pid, &m.Type, &m.Role, &m.Place, &m.Depuis, &m.Municipality, &m.District); err != nil {
			return nil, err
		}
		m.TypeCode = m.Type
		m.Type = labelTerm(m.Type)
		termsPerID[pid] = append(termsPerID[pid], m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i, rec := range recs {
		if i == 0 || len(rec) < 4 {
			continue
		}
		slug := strings.TrimSpace(rec[0])
		id, _ := strconv.ParseInt(strings.TrimSpace(rec[1]), 10, 64)
		out[slug].Terms = termsPerID[id]
	}
	return out, nil
}

var labelsTerm = map[string]string{
	"MAIRE": "Maire", "CONSEILLER_MUNICIPAL": "Conseiller municipal",
	"CONSEILLER_COMMUNAUTAIRE": "Conseiller communautaire",
	"CONSEILLER_DEPARTEMENTAL": "Conseiller départemental",
	"CONSEILLER_REGIONAL":      "Conseiller régional",
	"DEPUTE":                   "Député", "SENATEUR": "Sénateur",
	"DEPUTE_EUROPEEN": "Député européen", "MINISTRE": "Ministre",
	"PRESIDENT_REPUBLIQUE": "Président de la République",
}

func labelTerm(t string) string {
	if l := labelsTerm[t]; l != "" {
		return l
	}
	return t
}

// locateTermsPremises pose la chaîne de lieux sur les mandats des candidats,
// une fois le résolveur construit.
func locateTermsPremises(premises map[string]*ReconciliationRNE, r *Resolver) {
	for _, rp := range premises {
		for i := range rp.Terms {
			m := &rp.Terms[i]
			m.Places = r.Term(m.TypeCode, m.Municipality, m.District)
		}
	}
}
