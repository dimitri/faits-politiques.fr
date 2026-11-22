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
		if c.Status != "DECLARE" && c.Status != "PRIMAIRE" && c.Status != "RETIRE" {
			t.Errorf("%s : statut inattendu %q (colonnes décalées ?)", c.Slug, c.Status)
		}
		// Un retrait est un fait daté et sourcé, comme une déclaration ; sans
		// date ni source il ne se distingue pas d'une ligne mal remplie.
		if c.Withdrawn() && (c.DateWithdrawal == "" || c.WithdrawalSourceURL == "") {
			t.Errorf("%s : candidature retirée sans date ou sans source du retrait", c.Slug)
		}
		if !c.Withdrawn() && (c.DateWithdrawal != "" || c.WithdrawalSourceURL != "") {
			t.Errorf("%s : date ou source de retrait sur une candidature non retirée", c.Slug)
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
	if ruffin.Note == "" || !ruffin.Withdrawn() || ruffin.DateDeclaration != "2026-05-12" ||
		ruffin.DateWithdrawal != "2026-11-21" {
		t.Errorf("Ruffin mal lu : %+v", ruffin)
	}
}
