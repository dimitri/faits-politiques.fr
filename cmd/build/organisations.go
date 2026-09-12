package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PosteLigne struct {
	Label, Caveat, Categorie string
	Montants                 []string
}

type ClassifLigne struct {
	Referentiel, Vocabulaire, Categorie, Periode string
	SetSlug, LibelleFr                           string
	Valeur                                       string // score continu, le cas échéant
}

type Organisation struct {
	Slug, Libelle, CodeCNCCFP, PopuListNom, Justification string
	GroupeANUID, CHESNom                                  string
	OrgID                                                 int64
	Logo                                                  *Media
	Groupe                                                *Groupe
	Exercices                                             []int
	Postes                                                []PosteLigne
	Classifications                                       []ClassifLigne
	Candidats                                             []*Candidat
	HasComptes                                            bool
}

// loadOrganisations lit la décision éditoriale de rattachement puis charge, pour
// chaque organisation, ses comptes publics et les classifications tierces qui la
// concernent. Le rapprochement passe par un IDENTIFIANT (code CNCCFP, libellé
// exact publié par le référentiel), jamais par une correspondance approchée de
// noms : une telle correspondance serait un jugement déguisé en calcul.
func loadOrganisations(ctx context.Context, pool *pgxpool.Pool, path string, tags map[string]Tag) (map[string]*Organisation, error) {
	f, err := os.Open(path)
	if err != nil {
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
	if len(recs) < 2 {
		return nil, fmt.Errorf("%s : aucune organisation", path)
	}
	idx := map[string]int{}
	for i, h := range recs[0] {
		idx[strings.TrimSpace(h)] = i
	}
	get := func(rec []string, k string) string {
		if i, ok := idx[k]; ok && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}

	out := map[string]*Organisation{}
	for _, rec := range recs[1:] {
		o := &Organisation{
			Slug: get(rec, "slug"), Libelle: get(rec, "libelle"),
			CodeCNCCFP: get(rec, "code_cnccfp"), PopuListNom: get(rec, "populist_nom"),
			GroupeANUID:   get(rec, "groupe_an_uid"),
			CHESNom:       get(rec, "ches_nom"),
			Justification: get(rec, "justification"),
		}
		if o.Slug == "" {
			continue
		}
		if err := loadComptes(ctx, pool, o); err != nil {
			return nil, fmt.Errorf("%s : %w", o.Slug, err)
		}
		if err := loadClassifications(ctx, pool, o, tags); err != nil {
			return nil, fmt.Errorf("%s : %w", o.Slug, err)
		}
		out[o.Slug] = o
	}
	return out, nil
}

func loadComptes(ctx context.Context, pool *pgxpool.Pool, o *Organisation) error {
	if o.CodeCNCCFP == "" {
		return nil
	}
	_ = pool.QueryRow(ctx, `
		SELECT organization_id FROM core.organization_identifier
		WHERE scheme = 'CNCCFP' AND value = $1`, o.CodeCNCCFP).Scan(&o.OrgID)
	rows, err := pool.Query(ctx, `
		SELECT p.code, p.label, coalesce(p.caveat,''), p.categorie, l.exercice, l.montant
		FROM core.party_account_line l
		JOIN ref.party_account_poste p ON p.code = l.poste
		JOIN core.organization_identifier i
		  ON i.organization_id = l.organization_id AND i.scheme = 'CNCCFP' AND i.value = $1
		ORDER BY p.ordre, l.exercice`, o.CodeCNCCFP)
	if err != nil {
		return err
	}
	defer rows.Close()

	type key struct{ code string }
	byPoste := map[string]map[int]float64{}
	meta := map[string]PosteLigne{}
	var ordre []string
	years := map[int]bool{}

	for rows.Next() {
		var code, label, caveat, categorie string
		var exercice int
		var montant float64
		if err := rows.Scan(&code, &label, &caveat, &categorie, &exercice, &montant); err != nil {
			return err
		}
		if _, ok := byPoste[code]; !ok {
			byPoste[code] = map[int]float64{}
			meta[code] = PosteLigne{Label: label, Caveat: caveat, Categorie: categorie}
			ordre = append(ordre, code)
		}
		byPoste[code][exercice] = montant
		years[exercice] = true
	}
	if len(ordre) == 0 {
		return nil
	}

	for y := 2021; y <= 2024; y++ {
		if years[y] {
			o.Exercices = append(o.Exercices, y)
		}
	}
	for _, code := range ordre {
		p := meta[code]
		for _, y := range o.Exercices {
			if v, ok := byPoste[code][y]; ok {
				p.Montants = append(p.Montants, euros(v))
			} else {
				p.Montants = append(p.Montants, "")
			}
		}
		o.Postes = append(o.Postes, p)
	}
	o.HasComptes = true
	return nil
}

func loadClassifications(ctx context.Context, pool *pgxpool.Pool, o *Organisation, tags map[string]Tag) error {
	add := func(nomDansReferentiel string) error {
		rows, err := pool.Query(ctx, `
			SELECT s.slug, s.label, s.vocabulary,
			       coalesce(c.category,''), coalesce(c.dimension,''),
			       coalesce(c.value::text,''),
			       coalesce(c.periode_debut::text,''), coalesce(c.periode_fin::text,'')
			FROM core.party_classification c
			JOIN ref.classification_set s ON s.id = c.classification_set_id
			JOIN core.organization org ON org.id = c.party_id
			WHERE org.name = $1
			ORDER BY s.label, c.category, c.dimension`, nomDansReferentiel)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c ClassifLigne
			var cat, dim, val, debut, fin string
			if err := rows.Scan(&c.SetSlug, &c.Referentiel, &c.Vocabulaire,
				&cat, &dim, &val, &debut, &fin); err != nil {
				return err
			}
			c.Categorie = cat
			if dim != "" {
				// Une moyenne d'appréciations d'experts n'a pas sept décimales
				// significatives : les afficher donnerait une fausse précision.
				c.Categorie, c.Valeur = dim, arrondi(val)
			}
			if t, ok := tags[c.SetSlug+"/"+c.Categorie]; ok {
				c.LibelleFr = t.LibelleFr
			}
			switch {
			case debut == "1900" && fin == "2100":
				c.Periode = "sur toute la période couverte par le référentiel"
			case debut == "1900":
				c.Periode = "jusqu'en " + fin
			case fin == "2100":
				c.Periode = "depuis " + debut
			case debut != "" && fin != "":
				c.Periode = "de " + debut + " à " + fin
			case debut != "":
				c.Periode = "depuis " + debut
			default:
				c.Periode = "vague 2024"
			}
			o.Classifications = append(o.Classifications, c)
		}
		return rows.Err()
	}
	if o.PopuListNom != "" {
		if err := add(o.PopuListNom); err != nil {
			return err
		}
	}
	if o.CHESNom != "" {
		if err := add(o.CHESNom); err != nil {
			return err
		}
	}
	return nil
}

// arrondi ramène un score à une décimale, avec la virgule française.
func arrondi(v string) string {
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return v
	}
	return strings.Replace(fmt.Sprintf("%.1f", f), ".", ",", 1)
}

// euros formate un montant avec des espaces insécables fins comme séparateur de
// milliers, sans décimale : les comptes sont publiés à l'euro près et la
// précision décimale n'apporterait rien à la lecture.
func euros(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	s := fmt.Sprintf("%.0f", v)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteString(" ")
		}
		b.WriteRune(r)
	}
	out := b.String() + " €"
	if neg {
		return "−" + out
	}
	return out
}
