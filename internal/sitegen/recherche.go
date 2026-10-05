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
type Entree struct {
	N string `json:"n"`           // nom affiché
	S string `json:"s,omitempty"` // complément, entre aussi dans la recherche
	T string `json:"t"`           // nature, affichée à droite
	U string `json:"u"`           // chemin RELATIF à la racine du site
}

func ecrireIndex(out string, persons map[string]*Person, candidats []*Candidat,
	orgs map[string]*Organisation, groupes map[string]*Groupe,
	refs map[string]*Referentiel, themes []*Theme, docs []*Doc,
	senateurs map[string]bool) error {

	var idx []Entree
	vus := map[string]bool{}
	add := func(e Entree) {
		if e.N == "" || vus[e.U] {
			return
		}
		vus[e.U] = true
		idx = append(idx, e)
	}

	for _, c := range candidats {
		add(Entree{N: c.Prenom + " " + c.Nom, S: c.Organisation,
			T: "Candidat 2027", U: "candidat/" + c.Slug + "/"})
	}
	for _, p := range persons {
		// La nature vient du mandat que la SOURCE publie, jamais d'une
		// supposition. Depuis que le Sénat est chargé, core.person contient des
		// personnes sans mandat ingéré : les étiqueter « Député » serait une
		// affirmation fausse sur 971 d'entre elles.
		nature := "Personne"
		switch {
		case strings.HasPrefix(p.Mandat, "depute"):
			nature = "Député"
		case senateurs[p.Slug]:
			nature = "Sénateur"
		}
		add(Entree{N: p.Prenom + " " + p.Nom, S: p.Groupe,
			T: nature, U: "depute/" + p.Slug + "/"})
	}
	for _, o := range orgs {
		add(Entree{N: o.Libelle, T: "Parti", U: "organisation/" + o.Slug + "/"})
	}
	for _, g := range groupes {
		add(Entree{N: g.Nom, S: g.NomCourt, T: "Groupe", U: "groupe/" + g.Slug + "/"})
	}
	for _, r := range refs {
		add(Entree{N: r.Titre, T: "Référentiel", U: "referentiel/" + r.Slug + "/"})
	}
	for _, t := range themes {
		add(Entree{N: t.Label, T: "Thème", U: "theme/" + t.Slug + "/"})
	}
	for _, d := range docs {
		add(Entree{N: d.Titre, S: d.Fichier, T: "Méthode", U: "comprendre/" + d.Slug + "/"})
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
