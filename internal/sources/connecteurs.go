package sources

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Connecteur : une fonction Ingest* trouvée dans internal/ — le « type de
// connecteur » de faits-politiques.fr n'est pas déclaré dans un registre à
// part, c'est le code lui-même. Chercher directement dedans ne peut pas
// dériver d'une liste qu'on aurait oublié de tenir à jour.
type Connecteur struct {
	Paquet   string
	Fonction string
	Fichier  string
}

var reFonctionIngest = regexp.MustCompile(`(?m)^func (Ingest\w+)\(`)

// ListerConnecteurs cherche, sous racine/internal, toute fonction exportée
// dont le nom commence par Ingest — la convention de nommage effectivement
// suivie par les connecteurs de ce dépôt (140 fonctions, à ce jour, dans une
// quarantaine de paquets). Le paquet rapporté est le nom du répertoire, qui
// correspond au nom du paquet Go dans tout ce dépôt.
func ListerConnecteurs(racine string) ([]Connecteur, error) {
	var out []Connecteur
	err := filepath.WalkDir(filepath.Join(racine, "internal"), func(chemin string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(chemin, ".go") || strings.HasSuffix(chemin, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(chemin)
		if err != nil {
			return err
		}
		for _, m := range reFonctionIngest.FindAllStringSubmatch(string(b), -1) {
			out = append(out, Connecteur{
				Paquet:   filepath.Base(filepath.Dir(chemin)),
				Fonction: m[1],
				Fichier:  chemin,
			})
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool {
		if out[i].Paquet != out[j].Paquet {
			return out[i].Paquet < out[j].Paquet
		}
		return out[i].Fonction < out[j].Fonction
	})
	return out, err
}
