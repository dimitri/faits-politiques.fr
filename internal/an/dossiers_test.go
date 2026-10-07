package an

import "testing"

// TestSlugUniqueRepetitions reproduit le cas réel qui a cassé
// texte_slug_key en CI : le même texte de loi traverse plusieurs étapes
// (dépôt, adoption, Assemblée puis Sénat...), chacune avec son propre UID
// mais un titre identique au caractère près — jusqu'à neuf occurrences
// observées pour un seul texte (voir le commentaire de slugUnique).
func TestSlugUniqueRepetitions(t *testing.T) {
	seenSlug := map[string]bool{}
	uids := []string{
		"PRJLANR5L17B1470", "PRJLANR5L17BTA0154", "PRJLANR5L17BTA0164",
		"PRJLANR5L17TAP0154", "PRJLANR5L17TAP0164", "PRJLSNR5S459B0544",
		"PRJLSNR5S459B0810", "PRJLSNR5S459BTA0128", "PRJLSNR5S459BTA0172",
	}
	title := "projet de loi de programmation pour la refondation de Mayotte"
	seen := map[string]bool{}
	for _, uid := range uids {
		slug := slugUnique(seenSlug, "t-", title, uid)
		if seen[slug] {
			t.Fatalf("slug %q réattribué pour uid=%s", slug, uid)
		}
		seen[slug] = true
	}
	if len(seen) != len(uids) {
		t.Fatalf("attendu %d slugs distincts, obtenu %d", len(uids), len(seen))
	}
}

// TestSlugUniqueFallbackCollision : le repli titre-uid peut lui-même
// recouper le slug qu'une AUTRE ligne a déjà pris tel quel — l'ancien essai
// unique ne revérifiait jamais ce cas. Construit délibérément : le titre B
// slugifie en "x", pris par A ; le repli de B est alors "x-uidb", pris
// exactement par le titre (fictif) de C.
func TestSlugUniqueFallbackCollision(t *testing.T) {
	seenSlug := map[string]bool{}
	a := slugUnique(seenSlug, "t-", "x", "uidA")
	if a != "x" {
		t.Fatalf("attendu x, obtenu %q", a)
	}
	c := slugUnique(seenSlug, "t-", "x-uidb", "uidC")
	if c != "x-uidb" {
		t.Fatalf("attendu x-uidb, obtenu %q", c)
	}
	// B : slugify("x") == "x", déjà pris par A -> repli "x-uidb" — mais
	// c'est EXACTEMENT le slug que C vient de prendre tel quel. L'ancien
	// code s'arrêtait là et laissait la base en doublon jusqu'au niveau SQL ;
	// slugUnique doit boucler jusqu'à une troisième variante.
	b := slugUnique(seenSlug, "t-", "x", "uidB")
	if b == a || b == c {
		t.Fatalf("slug %q réutilisé (a=%q c=%q)", b, a, c)
	}
}

func TestSlugUniqueEmptyTitle(t *testing.T) {
	seenSlug := map[string]bool{}
	slug := slugUnique(seenSlug, "", "", "ABC123")
	if slug != "abc123" {
		t.Fatalf("attendu le repli sur l'uid en minuscules, obtenu %q", slug)
	}
}
