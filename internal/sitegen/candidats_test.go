package sitegen

import "testing"

// Le fichier éditorial réel doit rester lisible : une ligne mal quotée (une
// note avec des virgules, par exemple) décalerait les colonnes sans erreur.
func TestLoadCandidatesRealFile(t *testing.T) {
	cands, err := loadCandidates("../../data/candidats.csv", map[string]*Person{})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	var ruffin *Candidate
	for _, c := range cands {
		if seen[c.Slug] {
			t.Errorf("slug en double : %s", c.Slug)
		}
		seen[c.Slug] = true
		if c.Status != "DECLARE" && c.Status != "PRIMAIRE" {
			t.Errorf("%s : statut inattendu %q (colonnes décalées ?)", c.Slug, c.Status)
		}
		if c.SourceURL == "" || c.SourceConsulted == "" {
			t.Errorf("%s : déclaration sans source ou sans date de consultation", c.Slug)
		}
		if c.Slug == "francois-ruffin" {
			ruffin = c
		}
	}
	if ruffin == nil {
		t.Fatal("François Ruffin absent")
	}
	if ruffin.Note == "" || ruffin.Status != "DECLARE" || ruffin.DateDeclaration != "2026-05-12" {
		t.Errorf("Ruffin mal lu : %+v", ruffin)
	}
}
