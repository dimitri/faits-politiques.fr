package sources

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/faits-politiques/faits-politiques/internal/objectstore"
)

// EmpreintesStockage : les clés effectivement présentes dans le support de
// stockage (disque local ou object store), avec leur taille — construites
// en UNE passe (un parcours de répertoire, ou un listing de bucket), jamais
// un accès par document : sur des milliers de documents, ce serait des
// milliers d'appels système ou de requêtes S3 pour la même réponse.
type EmpreintesStockage map[string]int64

// StockageDisque parcourt racine (l'archive scellée locale, voir
// internal/archive) et renvoie la taille de chaque fichier, sous une clé
// égale à son chemin relatif — la même convention que raw.document.storage_key.
func StockageDisque(racine string) (EmpreintesStockage, error) {
	out := EmpreintesStockage{}
	err := filepath.WalkDir(racine, func(chemin string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil // archive locale absente : rien à vérifier, pas une erreur
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(racine, chemin)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = info.Size()
		return nil
	})
	return out, err
}

// StockageS3 liste un bucket compatible S3 (voir internal/objectstore) et
// renvoie la même structure que StockageDisque — pour que le reste du
// paquet compare une source à sa liste de présence sans savoir laquelle des
// deux l'a produite.
func StockageS3(ctx context.Context, bucket string) (EmpreintesStockage, error) {
	c, err := objectstore.Client(objectstore.DepuisEnv())
	if err != nil {
		return nil, err
	}
	objets, err := objectstore.ListerPrefixe(ctx, c, bucket, "")
	if err != nil {
		return nil, fmt.Errorf("bucket %s : %w", bucket, err)
	}
	out := make(EmpreintesStockage, len(objets))
	for _, o := range objets {
		out[o.Cle] = o.Taille
	}
	return out, nil
}

// Manquants compte, parmi chemins (les storage_key attendus par la base),
// ceux qui sont absents de empreintes ou dont la taille ne correspond pas —
// un document tronqué compte comme manquant, pas comme présent.
func (e EmpreintesStockage) Manquants(chemins []string, taillesAttendues map[string]int64) int {
	n := 0
	for _, c := range chemins {
		taille, ok := e[c]
		if !ok || taille != taillesAttendues[c] {
			n++
		}
	}
	return n
}
