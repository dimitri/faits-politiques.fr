package main

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	U string `json:"u"`           // URL
}

func ecrireIndex(out, root string, persons map[string]*Person, candidats []*Candidat,
	orgs map[string]*Organisation, groupes map[string]*Groupe,
	refs map[string]*Referentiel) error {

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
			T: "Candidat 2027", U: root + "/candidat/" + c.Slug + "/"})
	}
	for _, p := range persons {
		add(Entree{N: p.Prenom + " " + p.Nom, S: p.Groupe,
			T: "Député", U: root + "/depute/" + p.Slug + "/"})
	}
	for _, o := range orgs {
		add(Entree{N: o.Libelle, T: "Parti", U: root + "/organisation/" + o.Slug + "/"})
	}
	for _, g := range groupes {
		add(Entree{N: g.Nom, S: g.NomCourt, T: "Groupe", U: root + "/groupe/" + g.Slug + "/"})
	}
	for _, r := range refs {
		add(Entree{N: r.Titre, T: "Référentiel", U: root + "/referentiel/" + r.Slug + "/"})
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
