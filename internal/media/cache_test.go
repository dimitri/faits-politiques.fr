package media

import (
	"path/filepath"
	"testing"
)

func TestCacheLoadAbsent(t *testing.T) {
	c, err := loadCache(filepath.Join(t.TempDir(), "n-existe-pas.json"))
	if err != nil {
		t.Fatalf("loadCache sur un fichier absent : %v", err)
	}
	if c.Entries == nil || len(c.Entries) != 0 {
		t.Fatalf("attendu un cache vide, obtenu %#v", c)
	}
}

func TestCacheRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "media-cache.json")
	c := cache{Entries: map[string]cacheEntry{
		cacheKey("Jean Dupont", "PORTRAIT"): {
			Name: "Jean_Dupont.jpg", Licence: "CC BY-SA 4.0", Local: "jean-dupont-portrait.jpg",
			Width: 400, Height: 533,
		},
		cacheKey("Parti Imaginaire", "LOGO"): {
			Rejected: true, Reason: "licence non libre : fair use",
		},
	}}
	if err := c.save(path); err != nil {
		t.Fatalf("save : %v", err)
	}
	reread, err := loadCache(path)
	if err != nil {
		t.Fatalf("loadCache : %v", err)
	}
	if len(reread.Entries) != 2 {
		t.Fatalf("attendu 2 entrées, obtenu %d", len(reread.Entries))
	}
	pos, ok := reread.Entries[cacheKey("Jean Dupont", "PORTRAIT")]
	if !ok || pos.Local != "jean-dupont-portrait.jpg" || pos.Rejected {
		t.Fatalf("entrée positive mal relue : %#v", pos)
	}
	neg, ok := reread.Entries[cacheKey("Parti Imaginaire", "LOGO")]
	if !ok || !neg.Rejected || neg.Reason == "" {
		t.Fatalf("entrée négative mal relue : %#v", neg)
	}
}

func TestCacheKeyDistinguishesKind(t *testing.T) {
	// Même PageFR, kinds différents : deux clés distinctes, jamais la même
	// entrée de cache utilisée pour un portrait et un logo.
	if cacheKey("Même Page", "PORTRAIT") == cacheKey("Même Page", "LOGO") {
		t.Fatal("cacheKey ne distingue pas PORTRAIT de LOGO pour la même page")
	}
}
