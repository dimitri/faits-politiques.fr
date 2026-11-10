package fiscalite

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// Le jeu DECP consolidé est passé de decp.csv à decp.csv.gz : le même lecteur
// doit lire l'un et l'autre, sans dépendre de l'extension du fichier scellé.
func TestReadCSVStreamGzipOuNon(t *testing.T) {
	const csvTexte = "uid,montant\nA,10\nB,20\n"
	dir := t.TempDir()

	brut := filepath.Join(dir, "brut.csv")
	if err := os.WriteFile(brut, []byte(csvTexte), 0o644); err != nil {
		t.Fatal(err)
	}
	comprime := filepath.Join(dir, "sans-extension")
	fh, err := os.Create(comprime)
	if err != nil {
		t.Fatal(err)
	}
	zw := gzip.NewWriter(fh)
	zw.Write([]byte(csvTexte))
	zw.Close()
	fh.Close()

	for _, chemin := range []string{brut, comprime} {
		var uids []string
		err := readCSVStream(chemin, func(col map[string]int, rec []string) error {
			uids = append(uids, rec[col["uid"]])
			return nil
		})
		if err != nil {
			t.Fatalf("%s : %v", filepath.Base(chemin), err)
		}
		if len(uids) != 2 || uids[0] != "A" || uids[1] != "B" {
			t.Errorf("%s : lignes lues %v", filepath.Base(chemin), uids)
		}
	}
}
