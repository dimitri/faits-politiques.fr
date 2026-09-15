package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Une section n'est recopiée depuis la construction précédente, au lieu
// d'être refaite, que si DEUX empreintes concordent toutes les deux avec la
// dernière construction réussie :
//
//   - data  : core.section_checksum, calculée par cmd/ingest (voir
//     internal/checksum) — a-t-on chargé de nouvelles données depuis ?
//   - code  : le hachage des .go et .gohtml dont dépend la section — a-t-on
//     changé la façon de la construire depuis ?
//
// Les deux sont nécessaires : la donnée seule ignore un gabarit modifié à
// données inchangées ; le code seul ignore une nouvelle ingestion. Se tromper
// dans un sens (oublier un fichier ci-dessous) ne fait jamais servir une page
// fausse — au pire une section reconstruite pour rien ; se tromper dans l'autre
// sens n'existe pas, il n'y a pas de mécanisme qui accepterait une empreinte
// qu'on n'aurait pas vérifiée.
var fichiersSection = map[string][]string{
	"communes": {
		"web/templates/base.gohtml", "cmd/build/main.go", "cmd/build/format.go",
		"cmd/build/assets.go", "cmd/build/typographie.go",
		"cmd/build/lieux.go", "cmd/build/lieux_pages.go", "cmd/build/carte_situation.go",
		"web/templates/commune.gohtml", "web/templates/epci.gohtml",
	},
	"scrutin": {
		"web/templates/base.gohtml", "cmd/build/main.go", "cmd/build/format.go",
		"cmd/build/assets.go", "cmd/build/typographie.go",
		"cmd/build/scrutins.go", "cmd/build/dossiers.go",
		"web/templates/scrutin.gohtml",
	},
}

// manifesteCache : ce que la construction précédente a réellement produit,
// pour chaque section — écrit dans le site publié, donc transmis d'une
// construction à l'autre par le même renommage que le reste (mettreEnPlace).
type manifesteCache struct {
	Sections map[string]etatSection `json:"sections"`
}
type etatSection struct {
	DataHash string `json:"data_hash"`
	CodeHash string `json:"code_hash"`
	// N : le compte à afficher dans le journal quand la section est recopiée
	// plutôt que reconstruite — pour ne pas avoir à relire le site précédent
	// juste pour ce chiffre.
	N int `json:"n,omitempty"`
}

const nomManifeste = ".build-cache.json"

func chargerManifeste(siteExistant string) manifesteCache {
	var m manifesteCache
	b, err := os.ReadFile(filepath.Join(siteExistant, nomManifeste))
	if err != nil {
		return manifesteCache{Sections: map[string]etatSection{}}
	}
	if err := json.Unmarshal(b, &m); err != nil || m.Sections == nil {
		return manifesteCache{Sections: map[string]etatSection{}}
	}
	return m
}

func (m manifesteCache) ecrire(chantier string) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(chantier, nomManifeste), b, 0o644)
}

// hashFichiers hache le contenu concaténé des fichiers listés, dans un ordre
// fixe (celui de la liste, pas celui du système de fichiers) : peu importe,
// seul compte que le même ensemble de fichiers produise toujours le même
// résultat. Un fichier absent change le résultat plutôt que d'échouer : un
// renommage de fichier doit invalider le cache, pas le paralyser.
func hashFichiers(chemins []string) (string, error) {
	h := sha256.New()
	for _, chemin := range chemins {
		fmt.Fprintf(h, "%s:", chemin)
		f, err := os.Open(chemin)
		if err != nil {
			fmt.Fprintf(h, "absent;")
			continue
		}
		if _, err := io.Copy(h, f); err != nil {
			f.Close()
			return "", err
		}
		f.Close()
		fmt.Fprint(h, ";")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// sectionInchangee compare l'état enregistré au dernier build réussi à l'état
// actuel (données + code). ctx/pool ne servent qu'à lire core.section_checksum
// — une seule ligne, jamais le contenu des tables : le calcul coûteux a déjà
// été fait par cmd/ingest.
func sectionInchangee(ctx context.Context, pool *pgxpool.Pool, ancien manifesteCache, section string) (bool, etatSection, error) {
	var actuel etatSection
	err := pool.QueryRow(ctx, `SELECT data_hash FROM core.section_checksum WHERE section=$1`, section).
		Scan(&actuel.DataHash)
	if err != nil {
		// Pas encore d'empreinte pour cette section (migration toute
		// fraîche, ou cmd/ingest -only=checksums pas encore lancé) : on ne
		// sait pas si les données ont changé, donc on ne recopie pas.
		return false, actuel, nil
	}
	actuel.CodeHash, err = hashFichiers(fichiersSection[section])
	if err != nil {
		return false, actuel, err
	}
	prec, ok := ancien.Sections[section]
	inchangee := ok && prec.DataHash == actuel.DataHash && prec.CodeHash == actuel.CodeHash
	return inchangee, actuel, nil
}

// copierRepertoire recopie un sous-répertoire du site précédent dans le
// chantier en cours — utilisé quand une section est recopiée plutôt que
// reconstruite. Une copie physique, pas un lien : le site précédent est
// détruit par mettreEnPlace juste après l'échange, un lien pendrait dans le vide.
func copierRepertoire(src, dst string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil // rien à recopier (première construction, ou section vide)
	}
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		cible := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(cible, 0o755)
		}
		s, err := os.Open(p)
		if err != nil {
			return err
		}
		defer s.Close()
		d, err := os.Create(cible)
		if err != nil {
			return err
		}
		defer d.Close()
		_, err = io.Copy(d, s)
		return err
	})
}
