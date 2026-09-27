package sitegen

import (
	"encoding/csv"
	"os"
	"strings"
)

type Presidence struct {
	Debut, Fin, Nom, Qualite, Source string
}

// loadPresidences lit la chronologie des présidences.
//
// Elle sert uniquement de REPÈRE : situer un mandat ministériel dans le temps.
// Ce n'est pas une imputation — un ministre n'est pas responsable des actes du
// président, ni l'inverse.
func loadPresidences(path string) ([]Presidence, error) {
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
	var out []Presidence
	for _, rec := range recs[1:] {
		p := Presidence{Debut: get(rec, "debut"), Fin: get(rec, "fin"),
			Nom: get(rec, "nom"), Qualite: get(rec, "qualite"), Source: get(rec, "source")}
		if p.Debut != "" {
			out = append(out, p)
		}
	}
	return out, nil
}

// presidencesDe retourne les présidences qui recouvrent une période, dans
// l'ordre. Un mandat à cheval sur deux présidences en cite deux : tronquer
// donnerait une chronologie fausse.
func presidencesDe(ps []Presidence, debutISO, finISO string) []string {
	if debutISO == "" {
		return nil
	}
	if finISO == "" {
		finISO = "9999-12-31"
	}
	var out []string
	for _, p := range ps {
		fin := p.Fin
		if fin == "" {
			fin = "9999-12-31"
		}
		if p.Debut < finISO && debutISO < fin {
			out = append(out, p.Nom)
		}
	}
	return out
}
