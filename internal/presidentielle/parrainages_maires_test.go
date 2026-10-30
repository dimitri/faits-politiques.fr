package presidentielle

import (
	"strings"
	"testing"
)

func TestParseMairesSeulsLesMairesNommes(t *testing.T) {
	fichier := "Civilité;Nom;Prénom;Mandat;Circonscription;Département;Candidat;\"Date de publication\"\n" +
		"M.;CORDIVAL;Gilles;Maire;Mont-Saint-Père;Aisne;\"ARTHAUD Nathalie\";03/02/2022\n" +
		"M.;CORDIVAL;Gilles;Maire;Mont-Saint-Père;Aisne;\"ARTHAUD Nathalie\";01/02/2022\n" +
		"M.;PICARD;Laurent;\"Maire délégué d'une commune associée ou d'une commune déléguée\";Chemillé-en-Anjou;Maine-et-Loire;\"ZEMMOUR Éric\";03/03/2022\n" +
		"M.;E;F;\"Conseiller départemental\";;Jura;\"ARTHAUD Nathalie\";03/02/2022\n"
	got, err := parseMaires(strings.NewReader(fichier))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("un seul maire attendu (ni délégué ni conseiller), obtenu %d : %v", len(got), got)
	}
	m := got[0]
	if m.nom != "CORDIVAL" || m.commune != "Mont-Saint-Père" || m.candidat != "ARTHAUD Nathalie" ||
		m.date.Format("2006-01-02") != "2022-02-01" {
		t.Errorf("maire inattendu (la date la plus ancienne doit l'emporter) : %+v", m)
	}
}

func TestParseMairesColonneNomAbsente(t *testing.T) {
	_, err := parseMaires(strings.NewReader("Mandat;Département;Candidat;Date de publication\nMaire;Aisne;X;01/02/2022\n"))
	if err == nil || !strings.Contains(err.Error(), "nom") {
		t.Errorf("une colonne nom absente doit échouer nommément, obtenu : %v", err)
	}
}
