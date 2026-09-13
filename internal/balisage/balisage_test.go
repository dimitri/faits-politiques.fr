package balisage

import "testing"

func TestTexte(t *testing.T) {
	cas := []struct{ nom, entree, attendu string }{
		{
			// Le bug publié : toutes les balises remplacées par un espace.
			"balise en ligne soudée",
			"<p><span>M</span>esdames, Messieurs,</p>",
			"Mesdames, Messieurs,",
		},
		{
			// Celui que l'expression régulière ne peut pas voir : un `>` dans
			// un attribut arrête `<[^>]+>` au mauvais endroit.
			"chevron dans un attribut",
			`<p title="a>b">Texte</p>`,
			"Texte",
		},
		{"blocs séparés", "<p>Un</p><p>Deux</p>", "Un\nDeux"},
		{"saut de ligne", "Un<br/>Deux", "Un\nDeux"},
		{"br non fermé", "Un<br>Deux", "Un\nDeux"},
		{"entité html", "Les &laquo;&nbsp;apports&nbsp;&raquo; du texte", "Les « apports » du texte"},
		{"script ignoré", "<p>Vu</p><script>var x = 1 < 2;</script>", "Vu"},
		{"balise non fermée", "<p>Vu le code<p>Arrête :", "Vu le code\nArrête :"},
		{"esperluette nue", "Vu l'article 5 & suivants", "Vu l'article 5 & suivants"},
		{"sans balise", "Texte nu", "Texte nu"},
		{"vide", "", ""},
	}
	for _, c := range cas {
		if got := Texte(c.entree); got != c.attendu {
			t.Errorf("%s : Texte(%q) = %q, attendu %q", c.nom, c.entree, got, c.attendu)
		}
	}
}

func TestLigne(t *testing.T) {
	if got := Ligne("<p>Un</p><p>Deux</p>"); got != "Un Deux" {
		t.Errorf("Ligne = %q, attendu %q", got, "Un Deux")
	}
}
