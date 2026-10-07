package sitegen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Index de recherche locale. Un fichier statique, chargé au premier usage :
// aucun serveur, aucune requête sortante, aucun traceur, et le site reste
// consultable hors ligne.
//
// Le cas d'usage de ce site — contrôler une affirmation en sortant d'un débat —
// est une interrogation urgente. Sans porte d'entrée, il n'existe pas.
//
// PÉRIMÈTRE : personnes, partis, groupes et référentiels. Les 38 000 libellés
// de scrutins ne sont PAS indexés — 4,5 Mo de texte brut, dont le découpage et
// le classement demandent une décision qui n'est pas prise. L'absence est
// affichée dans le panneau de recherche, avec son code.
type Entry struct {
	N string `json:"n"`           // nom affiché
	S string `json:"s,omitempty"` // complément, entre aussi dans la recherche
	T string `json:"t"`           // nature, affichée à droite
	U string `json:"u"`           // chemin RELATIF à la racine du site
}

func writeIndex(out string, persons map[string]*Person, candidates []*Candidate,
	orgs map[string]*Organization, groups map[string]*Group,
	refs map[string]*Reference, themes []*Theme, docs []*Doc,
	senators map[string]bool) error {

	var idx []Entry
	seen := map[string]bool{}
	add := func(e Entry) {
		if e.N == "" || seen[e.U] {
			return
		}
		seen[e.U] = true
		idx = append(idx, e)
	}

	for _, c := range candidates {
		add(Entry{N: c.FirstName + " " + c.Name, S: c.Organization,
			T: "Candidat 2027", U: "candidat/" + c.Slug + "/"})
	}
	for _, p := range persons {
		// La nature vient du mandat que la SOURCE publie, jamais d'une
		// supposition. Depuis que le Sénat est chargé, core.person contient des
		// personnes sans mandat ingéré : les étiqueter « Député » serait une
		// affirmation fausse sur 971 d'entre elles.
		nature := "Personne"
		switch {
		case strings.HasPrefix(p.Term, "depute"):
			nature = "Député"
		case senators[p.Slug]:
			nature = "Sénateur"
		}
		add(Entry{N: p.FirstName + " " + p.Name, S: p.Group,
			T: nature, U: "depute/" + p.Slug + "/"})
	}
	for _, o := range orgs {
		add(Entry{N: o.Label, T: "Parti", U: "organisation/" + o.Slug + "/"})
	}
	for _, g := range groups {
		add(Entry{N: g.Name, S: g.NameShort, T: "Groupe", U: "groupe/" + g.Slug + "/"})
	}
	for _, r := range refs {
		add(Entry{N: r.Title, T: "Référentiel", U: "referentiel/" + r.Slug + "/"})
	}
	for _, t := range themes {
		add(Entry{N: t.Label, T: "Thème", U: "theme/" + t.Slug + "/"})
	}
	for _, d := range docs {
		add(Entry{N: d.Title, S: d.File, T: "Méthode", U: "comprendre/" + d.Slug + "/"})
	}

	b, err := json.Marshal(idx)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "recherche-index.json"), b, 0o644)
}
