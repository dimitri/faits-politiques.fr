package sitegen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePage(t *testing.T, dir, rel, css, js string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	html := `<!doctype html><head><link rel="stylesheet" href="/` + css + `"><script src="/` + js + `"></script></head><body>x</body>`
	if err := os.WriteFile(p, []byte(html), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCheckAssetReferences(t *testing.T) {
	a := Assets{CSS: "style.0123456789.css", JS: "site.abcdef0123.js"}
	dir := t.TempDir()
	writePage(t, dir, "index.html", a.CSS, a.JS)
	writePage(t, dir, "collectivites/commune/01013/index.html", a.CSS, a.JS)
	if err := checkAssetReferences(dir, a); err != nil {
		t.Fatalf("des pages à jour doivent passer : %v", err)
	}

	// Une page recopiée du site précédent, avec l'ancien nom de feuille de style.
	writePage(t, dir, "collectivites/epci/200059392/index.html", "style.ffffffffff.css", a.JS)
	err := checkAssetReferences(dir, a)
	if err == nil || !strings.Contains(err.Error(), "epci/200059392") {
		t.Fatalf("une page à l'ancienne feuille de style doit être nommée, obtenu : %v", err)
	}

	// Un fichier .html sans aucune référence (une carte autonome) n'est pas une page du site.
	dir2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir2, "carte.html"), []byte("<svg></svg>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkAssetReferences(dir2, a); err != nil {
		t.Errorf("un fichier sans référence est ignoré : %v", err)
	}
}

// Les sections recopiées d'un site à l'autre ne peuvent pas ignorer les ressources
// partagées : leur nom, empreinte comprise, est écrit dans chaque page.
func TestFilesSectionIncludeSharedAssets(t *testing.T) {
	for section, files := range filesSection {
		for _, want := range []string{"web/assets/style.css", "web/assets/site.js", "internal/sitegen/icones.go", "internal/sitegen/assets.go"} {
			found := false
			for _, f := range files {
				found = found || f == want
			}
			if !found {
				t.Errorf("la section %q n'est pas invalidée par %s", section, want)
			}
		}
	}
}
