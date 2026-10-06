package media

import (
	"path/filepath"
	"testing"
)

func TestCacheChargerAbsent(t *testing.T) {
	c, err := chargerCache(filepath.Join(t.TempDir(), "n-existe-pas.json"))
	if err != nil {
		t.Fatalf("chargerCache sur un fichier absent : %v", err)
	}
	if c.Entries == nil || len(c.Entries) != 0 {
		t.Fatalf("attendu un cache vide, obtenu %#v", c)
	}
}

func TestCacheRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "media-cache.json")
	c := cache{Entries: map[string]entreeCache{
		cleCache("Jean Dupont", "PORTRAIT"): {
			Nom: "Jean_Dupont.jpg", Licence: "CC BY-SA 4.0", Local: "jean-dupont-portrait.jpg",
			Largeur: 400, Hauteur: 533,
		},
		cleCache("Parti Imaginaire", "LOGO"): {
			Rejete: true, Raison: "licence non libre : fair use",
		},
	}}
	if err := c.sauvegarder(path); err != nil {
		t.Fatalf("sauvegarder : %v", err)
	}
	relu, err := chargerCache(path)
	if err != nil {
		t.Fatalf("chargerCache : %v", err)
	}
	if len(relu.Entries) != 2 {
		t.Fatalf("attendu 2 entrées, obtenu %d", len(relu.Entries))
	}
	pos, ok := relu.Entries[cleCache("Jean Dupont", "PORTRAIT")]
	if !ok || pos.Local != "jean-dupont-portrait.jpg" || pos.Rejete {
		t.Fatalf("entrée positive mal relue : %#v", pos)
	}
	neg, ok := relu.Entries[cleCache("Parti Imaginaire", "LOGO")]
	if !ok || !neg.Rejete || neg.Raison == "" {
		t.Fatalf("entrée négative mal relue : %#v", neg)
	}
}

func TestCleCacheDistingueLeKind(t *testing.T) {
	// Même PageFR, kinds différents : deux clés distinctes, jamais la même
	// entrée de cache utilisée pour un portrait et un logo.
	if cleCache("Même Page", "PORTRAIT") == cleCache("Même Page", "LOGO") {
		t.Fatal("cleCache ne distingue pas PORTRAIT de LOGO pour la même page")
	}
}
