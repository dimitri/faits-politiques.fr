package sitegen

import (
	"encoding/csv"
	"os"
	"strings"
)

type Coalition struct {
	Slug, Libelle, Annee, Scrutin, Source, SourceConsultee string
	Composantes                                            []*Organisation
}

// loadCoalitions lit les coalitions électorales.
//
// Une coalition n'est ni un parti ni un groupe : elle ne dépose pas de comptes,
// n'a pas d'adhérents, et ses composantes gardent leur structure. Un groupe
// parlementaire peut porter son nom sans la rassembler — les autres composantes
// constituent leurs propres groupes.
func loadCoalitions(path string, orgs map[string]*Organisation) ([]*Coalition, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comment = '#'
	r.FieldsPerRecord = -1
	recs, err := r.ReadAll()
	if err != nil || len(recs) < 2 {
		return nil, err
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
	var out []*Coalition
	for _, rec := range recs[1:] {
		c := &Coalition{Slug: get(rec, "slug"), Libelle: get(rec, "libelle"),
			Annee: get(rec, "annee"), Scrutin: get(rec, "scrutin"),
			Source: get(rec, "source"), SourceConsultee: get(rec, "source_consultee")}
		if c.Slug == "" {
			continue
		}
		for _, s := range strings.Split(get(rec, "composantes"), "|") {
			if o, ok := orgs[strings.TrimSpace(s)]; ok {
				c.Composantes = append(c.Composantes, o)
			}
		}
		out = append(out, c)
	}
	return out, nil
}
