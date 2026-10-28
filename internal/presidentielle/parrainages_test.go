package presidentielle

import (
	"strings"
	"testing"
)

func TestCategorieMandat(t *testing.T) {
	cas := map[string]string{
		"Maire":                          "MAIRE",
		"Conseillère départementale":     "CONSEILLER_DEPARTEMENTAL",
		"Conseiller/ère départemental-e": "CONSEILLER_DEPARTEMENTAL",
		"Conseiller régional":            "CONSEILLER_REGIONAL",
		"Maire déléguée d'une commune associée ou d'une commune déléguée": "MAIRE_DELEGUE",
		"Maire délégué-e":        "MAIRE_DELEGUE",
		"Maire d'arrondissement": "MAIRE_ARRONDISSEMENT",
		"Députée":                "DEPUTE",
		"Sénatrice":              "SENATEUR",
		"Sénateur/trice":         "SENATEUR",
		"Représentante française au Parlement européen":          "EURODEPUTE",
		"Président d'un EPCI à fiscalité propre":                 "PRESIDENT_INTERCO",
		"Président-e d'un conseil de communauté d'agglomération": "PRESIDENT_INTERCO",
		"Président de la Polynésie française":                    "AUTRE",
		"Conseillère de Paris":                                   "AUTRE",
	}
	for in, want := range cas {
		if got := categorieMandat(in); got != want {
			t.Errorf("categorieMandat(%q) = %s, attendu %s", in, got, want)
		}
	}
}

func TestParseParrainagesLesDeuxFormats(t *testing.T) {
	en2022 := "Civilité;Nom;Prénom;Mandat;Circonscription;Département;Candidat;\"Date de publication\"\n" +
		"M.;A;B;Maire;X;Aisne;\"ARTHAUD Nathalie\";01/02/2022\n" +
		"Mme;C;D;Maire;Y;Aisne;\"ARTHAUD Nathalie\";01/02/2022\n" +
		"M.;E;F;\"Conseiller départemental\";;Jura;\"ARTHAUD Nathalie\";03/02/2022\n"
	agg, n, err := parseParrainages(strings.NewReader(en2022))
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 || agg[clef{"ARTHAUD Nathalie", "MAIRE", "Aisne", "2022-02-01"}] != 2 ||
		agg[clef{"ARTHAUD Nathalie", "CONSEILLER_DEPARTEMENTAL", "Jura", "2022-02-03"}] != 1 {
		t.Errorf("agrégat 2022 inattendu : %d lignes, %v", n, agg)
	}

	en2017 := string(rune(0xFEFF)) + "Civilité;Nom;Prénom;Mandat;Circonscription;Département;Candidat-e parrainé-e;Date de publication\r\n" +
		"M;BARIOD;Maurice;Maire;Saint-Maurice-Crillat;Jura;ALLIOT-MARIE Michèle;01/03/2017\r\n"
	agg, n, err = parseParrainages(strings.NewReader(en2017))
	if err != nil || n != 1 || agg[clef{"ALLIOT-MARIE Michèle", "MAIRE", "Jura", "2017-03-01"}] != 1 {
		t.Errorf("format 2017 : %v %d %v", err, n, agg)
	}
}

func TestParseParrainagesColonneAbsente(t *testing.T) {
	_, _, err := parseParrainages(strings.NewReader("Civilité;Nom;Mandat;Département;Date de publication\nM.;A;Maire;Aisne;01/02/2022\n"))
	if err == nil || !strings.Contains(err.Error(), "candidat") {
		t.Errorf("une colonne candidat absente doit échouer nommément, obtenu : %v", err)
	}
}

func TestDerniereCSVChoisitLHorodatageLePlusRecent(t *testing.T) {
	r, ok := derniereCSV([]ressourceJeu{
		{Title: "au 3 février", Format: "csv", URL: "https://x/res/20220203-171845/parrainagestotal.csv"},
		{Title: "au 7 mars", Format: "csv", URL: "https://x/res/20220307-183308/parrainagestotal.csv"},
		{Title: "au 7 mars xlsx", Format: "xlsx", URL: "https://x/res/20220307-183333/parrainagestotal.xlsx"},
		{Title: "au 1 mars", Format: "csv", URL: "https://x/res/20220301-180835/parrainagestotal.csv"},
	})
	if !ok || r.Title != "au 7 mars" {
		t.Errorf("attendu le CSV du 7 mars, obtenu %+v (%v)", r, ok)
	}
}
