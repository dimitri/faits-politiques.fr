package sources

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/faits-politiques/faits-politiques/internal/objectstore"
)

// StorageFootprint : les clés effectivement présentes dans le support de
// stockage (disque local ou object store), avec leur taille — construites
// en UNE passe (un parcours de répertoire, ou un listing de bucket), jamais
// un accès par document : sur des milliers de documents, ce serait des
// milliers d'appels système ou de requêtes S3 pour la même réponse.
type StorageFootprint map[string]int64

// DiskFootprint parcourt root (l'archive scellée locale, voir
// internal/archive) et renvoie la taille de chaque fichier, sous une clé
// égale à son chemin relatif — la même convention que raw.document.storage_key.
func DiskFootprint(root string) (StorageFootprint, error) {
	out := StorageFootprint{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil // archive locale absente : rien à vérifier, pas une erreur
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
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

// S3Footprint liste un bucket compatible S3 (voir internal/objectstore) et
// renvoie la même structure que DiskFootprint — pour que le reste du
// paquet compare une source à sa liste de présence sans savoir laquelle des
// deux l'a produite.
func S3Footprint(ctx context.Context, bucket string) (StorageFootprint, error) {
	c, err := objectstore.Client(objectstore.FromEnv())
	if err != nil {
		return nil, err
	}
	objects, err := objectstore.ListPrefix(ctx, c, bucket, "")
	if err != nil {
		return nil, fmt.Errorf("bucket %s : %w", bucket, err)
	}
	out := make(StorageFootprint, len(objects))
	for _, o := range objects {
		out[o.Key] = o.Size
	}
	return out, nil
}

// Missing compte, parmi paths (les storage_key attendus par la base),
// ceux qui sont absents de footprint ou dont la taille ne correspond pas —
// un document tronqué compte comme manquant, pas comme présent.
func (e StorageFootprint) Missing(paths []string, expectedSizes map[string]int64) int {
	n := 0
	for _, p := range paths {
		size, ok := e[p]
		if !ok || size != expectedSizes[p] {
			n++
		}
	}
	return n
}
