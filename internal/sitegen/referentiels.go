package sitegen

import (
	"encoding/csv"
	"os"
	"strings"
)

type Tag struct {
	SetSlug, Key, LabelFr, TextFr string
}

type Reference struct {
	Slug, Title, Method string
	Tags                []Tag
}

// loadReferences lit les explications françaises des catégories employées par
// les référentiels tiers.
//
// Ces textes sont LES NÔTRES : ils expliquent une méthode, ils ne la reprennent
// pas mot pour mot et ne l'endossent pas. La catégorie reste toujours affichée
// dans le vocabulaire d'origine — « far right » n'est jamais traduit en
// « extrême droite », parce que ce ne sont pas les mêmes objets.
func loadReferences(path string) (map[string]*Reference, map[string]Tag, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comment = '#'
	r.FieldsPerRecord = -1
	recs, err := r.ReadAll()
	if err != nil {
		return nil, nil, err
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

	refs := map[string]*Reference{}
	tags := map[string]Tag{}
	for _, rec := range recs[1:] {
		set := get(rec, "set_slug")
		if set == "" {
			continue
		}
		ref, ok := refs[set]
		if !ok {
			ref = &Reference{Slug: set}
			refs[set] = ref
		}
		switch get(rec, "type") {
		case "METHODE":
			ref.Title = get(rec, "libelle_fr")
			ref.Method = get(rec, "texte_fr")
		case "TAG":
			t := Tag{SetSlug: set, Key: get(rec, "cle"),
				LabelFr: get(rec, "libelle_fr"), TextFr: get(rec, "texte_fr")}
			ref.Tags = append(ref.Tags, t)
			tags[set+"/"+t.Key] = t
		}
	}
	return refs, tags, nil
}
