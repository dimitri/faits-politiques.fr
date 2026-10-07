package sources

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Connector : une fonction Ingest* trouvée dans internal/ — le « type de
// connecteur » de faits-politiques.fr n'est pas déclaré dans un registre à
// part, c'est le code lui-même. Chercher directement dedans ne peut pas
// dériver d'une liste qu'on aurait oublié de tenir à jour.
type Connector struct {
	Package  string
	Function string
	File     string
}

var reIngestFunc = regexp.MustCompile(`(?m)^func (Ingest\w+)\(`)

// ListConnectors cherche, sous root/internal, toute fonction exportée
// dont le nom commence par Ingest — la convention de nommage effectivement
// suivie par les connecteurs de ce dépôt (140 fonctions, à ce jour, dans une
// quarantaine de paquets). Le paquet rapporté est le nom du répertoire, qui
// correspond au nom du paquet Go dans tout ce dépôt.
func ListConnectors(root string) ([]Connector, error) {
	var out []Connector
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range reIngestFunc.FindAllStringSubmatch(string(b), -1) {
			out = append(out, Connector{
				Package:  filepath.Base(filepath.Dir(path)),
				Function: m[1],
				File:     path,
			})
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool {
		if out[i].Package != out[j].Package {
			return out[i].Package < out[j].Package
		}
		return out[i].Function < out[j].Function
	})
	return out, err
}
