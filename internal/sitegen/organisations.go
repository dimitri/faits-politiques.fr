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

type PositionLine struct {
	Label, Caveat, Category string
	Amounts                 []string
}

type ClassificationLine struct {
	Reference, Vocabulary, Category, Period string
	SetSlug, LabelFr                        string
	Value                                   string // score continu, le cas échéant
}

type Organization struct {
	Slug, Label, CodeCNCCFP, PopulationListName, Justification string
	GroupANUID, CHESName                                       string
	OrgID                                                      int64
	Logo                                                       *Media
	Group                                                      *Group
	FiscalYears                                                []int
	Positions                                                  []PositionLine
	Classifications                                            []ClassificationLine
	Candidates                                                 []*Candidate
	HasAccounts                                                bool
}

// loadOrganizations lit la décision éditoriale de rattachement puis charge, pour
// chaque organisation, ses comptes publics et les classifications tierces qui la
// concernent. Le rapprochement passe par un IDENTIFIANT (code CNCCFP, libellé
// exact publié par le référentiel), jamais par une correspondance approchée de
// noms : une telle correspondance serait un jugement déguisé en calcul.
func loadOrganizations(ctx context.Context, pool *pgxpool.Pool, path string, tags map[string]Tag) (map[string]*Organization, error) {
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

	out := map[string]*Organization{}
	var codesCNCCFP []string
	for _, rec := range recs[1:] {
		o := &Organization{
			Slug: get(rec, "slug"), Label: get(rec, "libelle"),
			CodeCNCCFP: get(rec, "code_cnccfp"), PopulationListName: get(rec, "populist_nom"),
			GroupANUID:    get(rec, "groupe_an_uid"),
			CHESName:      get(rec, "ches_nom"),
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
	orgIDPerCode, err := loadOrgIDPerCode(ctx, pool, codesCNCCFP)
	if err != nil {
		return nil, err
	}
	accountsPerCode, err := loadAccountsPerCode(ctx, pool, codesCNCCFP)
	if err != nil {
		return nil, err
	}
	for _, o := range out {
		if o.CodeCNCCFP != "" {
			o.OrgID = orgIDPerCode[o.CodeCNCCFP]
			applyAccounts(o, accountsPerCode[o.CodeCNCCFP])
		}
	}

	var nomsClassification []string
	for _, o := range out {
		if o.PopulationListName != "" {
			nomsClassification = append(nomsClassification, o.PopulationListName)
		}
		if o.CHESName != "" {
			nomsClassification = append(nomsClassification, o.CHESName)
		}
	}
	classificationPerName, err := loadClassificationsPerName(ctx, pool, nomsClassification, tags)
	if err != nil {
		return nil, err
	}
	for _, o := range out {
		if o.PopulationListName != "" {
			o.Classifications = append(o.Classifications, classificationPerName[o.PopulationListName]...)
		}
		if o.CHESName != "" {
			o.Classifications = append(o.Classifications, classificationPerName[o.CHESName]...)
		}
	}
	return out, nil
}

// loadOrgIDPerCode résout tous les codes CNCCFP en organization_id d'un
// coup — voir loadOrganisations, qui répartit ensuite par code.
func loadOrgIDPerCode(ctx context.Context, pool *pgxpool.Pool, codes []string) (map[string]int64, error) {
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

// loadAccountsPerCode charge les comptes CNCCFP de tous les codes d'un
// coup — voir loadOrganisations, qui répartit ensuite par code.
func loadAccountsPerCode(ctx context.Context, pool *pgxpool.Pool, codes []string) (map[string][]lineAccount, error) {
	out := map[string][]lineAccount{}
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
		var l lineAccount
		if err := rows.Scan(&code, &l.code, &l.label, &l.caveat, &l.category, &l.fiscalYear, &l.amount); err != nil {
			return nil, err
		}
		out[code] = append(out[code], l)
	}
	return out, rows.Err()
}

type lineAccount struct {
	code, label, caveat, category string
	fiscalYear                    int
	amount                        float64
}

// applyAccounts reproduit la mise en forme de l'ancienne loadComptes (une
// requête par organisation) à partir des lignes déjà chargées en bloc.
func applyAccounts(o *Organization, lines []lineAccount) {
	if len(lines) == 0 {
		return
	}
	byPosition := map[string]map[int]float64{}
	meta := map[string]PositionLine{}
	var order []string
	years := map[int]bool{}
	for _, l := range lines {
		if _, ok := byPosition[l.code]; !ok {
			byPosition[l.code] = map[int]float64{}
			meta[l.code] = PositionLine{Label: l.label, Caveat: l.caveat, Category: l.category}
			order = append(order, l.code)
		}
		byPosition[l.code][l.fiscalYear] = l.amount
		years[l.fiscalYear] = true
	}
	for y := 2021; y <= 2024; y++ {
		if years[y] {
			o.FiscalYears = append(o.FiscalYears, y)
		}
	}
	for _, code := range order {
		p := meta[code]
		for _, y := range o.FiscalYears {
			if v, ok := byPosition[code][y]; ok {
				p.Amounts = append(p.Amounts, euros(v))
			} else {
				p.Amounts = append(p.Amounts, "")
			}
		}
		o.Positions = append(o.Positions, p)
	}
	o.HasAccounts = true
}

// loadClassificationsPerName charge les classifications de tous les noms
// (PopuList et CHES confondus, une même requête suffit) en une fois — voir
// loadOrganisations, qui répartit ensuite par nom.
func loadClassificationsPerName(ctx context.Context, pool *pgxpool.Pool, noms []string, tags map[string]Tag) (map[string][]ClassificationLine, error) {
	out := map[string][]ClassificationLine{}
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
		var name string
		var c ClassificationLine
		var cat, dim, val, start, end string
		if err := rows.Scan(&name, &c.SetSlug, &c.Reference, &c.Vocabulary,
			&cat, &dim, &val, &start, &end); err != nil {
			return nil, err
		}
		c.Category = cat
		if dim != "" {
			// Une moyenne d'appréciations d'experts n'a pas sept décimales
			// significatives : les afficher donnerait une fausse précision.
			c.Category, c.Value = dim, arrondi(val)
		}
		if t, ok := tags[c.SetSlug+"/"+c.Category]; ok {
			c.LabelFr = t.LabelFr
		}
		switch {
		case start == "1900" && end == "2100":
			c.Period = "sur toute la période couverte par le référentiel"
		case start == "1900":
			c.Period = "jusqu'en " + end
		case end == "2100":
			c.Period = "depuis " + start
		case start != "" && end != "":
			c.Period = "de " + start + " à " + end
		case start != "":
			c.Period = "depuis " + start
		default:
			c.Period = "vague 2024"
		}
		out[name] = append(out[name], c)
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
