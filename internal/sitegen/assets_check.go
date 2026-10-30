package sitegen

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	reStyleRef  = regexp.MustCompile(`style\.[0-9a-f]{10}\.css`)
	reScriptRef = regexp.MustCompile(`site\.[0-9a-f]{10}\.js`)
)

// checkAssetReferences vérifie que chaque page nomme la feuille de style et le
// script réellement publiés.
//
// Le nom d'une ressource porte l'empreinte de son contenu (voir assets.go) : il
// change dès que le CSS change, et il est écrit dans CHAQUE page. Une page
// recopiée depuis le site précédent (communes, scrutins : voir cache.go) garde
// donc l'ancien nom si on a oublié d'invalider sa section — et le site publié
// perd son CSS sur des dizaines de milliers de pages, sans aucune erreur. Ce
// garde-fou est la ceinture : même si une liste de fichiers du cache est
// incomplète, une construction qui laisserait un nom périmé ne sera jamais mise
// en place.
//
// Seul le début de chaque page est lu : les deux références sont dans le <head>.
func checkAssetReferences(out string, a Assets) error {
	var stale []string
	pages := 0
	buf := make([]byte, 16<<10)
	err := filepath.WalkDir(out, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		n, err := io.ReadFull(f, buf)
		f.Close()
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return err
		}
		head := buf[:n]
		css, js := reStyleRef.Find(head), reScriptRef.Find(head)
		if css == nil && js == nil {
			return nil // pas une page du site (une carte autonome, par exemple)
		}
		pages++
		if (css != nil && string(css) != a.CSS) || (js != nil && string(js) != a.JS) {
			rel, _ := filepath.Rel(out, path)
			stale = append(stale, rel)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(stale) == 0 {
		return nil
	}
	exemples := stale
	if len(exemples) > 5 {
		exemples = exemples[:5]
	}
	return fmt.Errorf("%d pages sur %d nomment une feuille de style ou un script qui n'est pas celui publié (%s, %s), par exemple %s — "+
		"une section recopiée du site précédent n'a pas été invalidée par le changement des ressources (voir filesSection dans cache.go)",
		len(stale), pages, a.CSS, a.JS, strings.Join(exemples, ", "))
}
