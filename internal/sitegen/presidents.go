package sitegen

import (
	"encoding/csv"
	"os"
	"strings"
)

type Presidency struct {
	Start, End, Name, Quality, Source string
}

// loadPresidencies lit la chronologie des présidences.
//
// Elle sert uniquement de REPÈRE : situer un mandat ministériel dans le temps.
// Ce n'est pas une imputation — un ministre n'est pas responsable des actes du
// président, ni l'inverse.
func loadPresidencies(path string) ([]Presidency, error) {
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
	var out []Presidency
	for _, rec := range recs[1:] {
		p := Presidency{Start: get(rec, "debut"), End: get(rec, "fin"),
			Name: get(rec, "nom"), Quality: get(rec, "qualite"), Source: get(rec, "source")}
		if p.Start != "" {
			out = append(out, p)
		}
	}
	return out, nil
}

// presidenciesOf retourne les présidences qui recouvrent une période, dans
// l'ordre. Un mandat à cheval sur deux présidences en cite deux : tronquer
// donnerait une chronologie fausse.
func presidenciesOf(ps []Presidency, startISO, endISO string) []string {
	if startISO == "" {
		return nil
	}
	if endISO == "" {
		endISO = "9999-12-31"
	}
	var out []string
	for _, p := range ps {
		end := p.End
		if end == "" {
			end = "9999-12-31"
		}
		if p.Start < endISO && startISO < end {
			out = append(out, p.Name)
		}
	}
	return out
}
