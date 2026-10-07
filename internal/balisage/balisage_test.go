package balisage

import "testing"

func TestText(t *testing.T) {
	cases := []struct{ name, input, want string }{
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
	for _, c := range cases {
		if got := Text(c.input); got != c.want {
			t.Errorf("%s : Text(%q) = %q, attendu %q", c.name, c.input, got, c.want)
		}
	}
}

func TestLine(t *testing.T) {
	if got := Line("<p>Un</p><p>Deux</p>"); got != "Un Deux" {
		t.Errorf("Line = %q, attendu %q", got, "Un Deux")
	}
}
