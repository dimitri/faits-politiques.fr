package sitegen

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
	var codesCNCCFP []string
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
		if o.CodeCNCCFP != "" {
			codesCNCCFP = append(codesCNCCFP, o.CodeCNCCFP)
		}
		out[o.Slug] = o
	}

	// Deux requêtes pour TOUTES les organisations (comptes, classifications)
	// plutôt que jusqu'à quatre PAR organisation — une cinquantaine de
	// lignes dans data/organisations.csv aujourd'hui, mais le même patron
	// N+1 qu'ailleurs dans ce fichier.
	orgIDParCode, err := chargerOrgIDParCode(ctx, pool, codesCNCCFP)
	if err != nil {
		return nil, err
	}
	comptesParCode, err := chargerComptesParCode(ctx, pool, codesCNCCFP)
	if err != nil {
		return nil, err
	}
	for _, o := range out {
		if o.CodeCNCCFP != "" {
			o.OrgID = orgIDParCode[o.CodeCNCCFP]
			appliquerComptes(o, comptesParCode[o.CodeCNCCFP])
		}
	}

	var nomsClassif []string
	for _, o := range out {
		if o.PopuListNom != "" {
			nomsClassif = append(nomsClassif, o.PopuListNom)
		}
		if o.CHESNom != "" {
			nomsClassif = append(nomsClassif, o.CHESNom)
		}
	}
	classifParNom, err := chargerClassificationsParNom(ctx, pool, nomsClassif, tags)
	if err != nil {
		return nil, err
	}
	for _, o := range out {
		if o.PopuListNom != "" {
			o.Classifications = append(o.Classifications, classifParNom[o.PopuListNom]...)
		}
		if o.CHESNom != "" {
			o.Classifications = append(o.Classifications, classifParNom[o.CHESNom]...)
		}
	}
	return out, nil
}

// chargerOrgIDParCode résout tous les codes CNCCFP en organization_id d'un
// coup — voir loadOrganisations, qui répartit ensuite par code.
func chargerOrgIDParCode(ctx context.Context, pool *pgxpool.Pool, codes []string) (map[string]int64, error) {
	out := map[string]int64{}
	if len(codes) == 0 {
		return out, nil
	}
	rows, err := pool.Query(ctx, `
		SELECT value, organization_id FROM core.organization_identifier
		WHERE scheme = 'CNCCFP' AND value = ANY($1)`, codes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var code string
		var id int64
		if err := rows.Scan(&code, &id); err != nil {
			return nil, err
		}
		out[code] = id
	}
	return out, rows.Err()
}

// chargerComptesParCode charge les comptes CNCCFP de tous les codes d'un
// coup — voir loadOrganisations, qui répartit ensuite par code.
func chargerComptesParCode(ctx context.Context, pool *pgxpool.Pool, codes []string) (map[string][]ligneCompte, error) {
	out := map[string][]ligneCompte{}
	if len(codes) == 0 {
		return out, nil
	}
	rows, err := pool.Query(ctx, `
		SELECT i.value, p.code, p.label, coalesce(p.caveat,''), p.categorie, l.exercice, l.montant
		FROM core.party_account_line l
		JOIN ref.party_account_poste p ON p.code = l.poste
		JOIN core.organization_identifier i
		  ON i.organization_id = l.organization_id AND i.scheme = 'CNCCFP' AND i.value = ANY($1)
		ORDER BY i.value, p.ordre, l.exercice`, codes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var code string
		var l ligneCompte
		if err := rows.Scan(&code, &l.code, &l.label, &l.caveat, &l.categorie, &l.exercice, &l.montant); err != nil {
			return nil, err
		}
		out[code] = append(out[code], l)
	}
	return out, rows.Err()
}

type ligneCompte struct {
	code, label, caveat, categorie string
	exercice                       int
	montant                        float64
}

// appliquerComptes reproduit la mise en forme de l'ancienne loadComptes (une
// requête par organisation) à partir des lignes déjà chargées en bloc.
func appliquerComptes(o *Organisation, lignes []ligneCompte) {
	if len(lignes) == 0 {
		return
	}
	byPoste := map[string]map[int]float64{}
	meta := map[string]PosteLigne{}
	var ordre []string
	years := map[int]bool{}
	for _, l := range lignes {
		if _, ok := byPoste[l.code]; !ok {
			byPoste[l.code] = map[int]float64{}
			meta[l.code] = PosteLigne{Label: l.label, Caveat: l.caveat, Categorie: l.categorie}
			ordre = append(ordre, l.code)
		}
		byPoste[l.code][l.exercice] = l.montant
		years[l.exercice] = true
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
}

// chargerClassificationsParNom charge les classifications de tous les noms
// (PopuList et CHES confondus, une même requête suffit) en une fois — voir
// loadOrganisations, qui répartit ensuite par nom.
func chargerClassificationsParNom(ctx context.Context, pool *pgxpool.Pool, noms []string, tags map[string]Tag) (map[string][]ClassifLigne, error) {
	out := map[string][]ClassifLigne{}
	if len(noms) == 0 {
		return out, nil
	}
	rows, err := pool.Query(ctx, `
		SELECT org.name, s.slug, s.label, s.vocabulary,
		       coalesce(c.category,''), coalesce(c.dimension,''),
		       coalesce(c.value::text,''),
		       coalesce(c.periode_debut::text,''), coalesce(c.periode_fin::text,'')
		FROM core.party_classification c
		JOIN ref.classification_set s ON s.id = c.classification_set_id
		JOIN core.organization org ON org.id = c.party_id
		WHERE org.name = ANY($1)
		ORDER BY org.name, s.label, c.category, c.dimension`, noms)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var nom string
		var c ClassifLigne
		var cat, dim, val, debut, fin string
		if err := rows.Scan(&nom, &c.SetSlug, &c.Referentiel, &c.Vocabulaire,
			&cat, &dim, &val, &debut, &fin); err != nil {
			return nil, err
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
		out[nom] = append(out[nom], c)
	}
	return out, rows.Err()
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
