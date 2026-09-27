package sitegen

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

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
		"web/templates/base.gohtml", "internal/sitegen/main.go", "internal/sitegen/format.go",
		"internal/sitegen/assets.go", "internal/sitegen/typographie.go",
		"internal/sitegen/lieux.go", "internal/sitegen/lieux_pages.go", "internal/sitegen/carte_situation.go",
		"web/templates/commune.gohtml", "web/templates/epci.gohtml",
	},
	"scrutin": {
		"web/templates/base.gohtml", "internal/sitegen/main.go", "internal/sitegen/format.go",
		"internal/sitegen/assets.go", "internal/sitegen/typographie.go",
		"internal/sitegen/scrutins.go", "internal/sitegen/dossiers.go",
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

// errRienAFaire signale à main() que run() s'est arrêté avant même de créer
// le chantier : rien à mettre en place, ce n'est pas une erreur.
var errRienAFaire = errors.New("rien n'a changé depuis la dernière construction")

// hashGlobs élargit hashFichiers à des motifs (filepath.Glob) plutôt qu'une
// liste tenue à la main — utilisé pour « reste », la section fourre-tout qui
// couvre tout ce que communes/scrutin ne couvrent pas (une cinquantaine de
// pages indépendantes : accueil, dossiers, 2027, gouvernement, thèmes...).
// Une liste à la main serait aussi longue que risquée à tenir à jour pour un
// périmètre aussi large ; un glob ne peut pas oublier un fichier qui existe
// au moment de la construction — il peut seulement, par construction,
// changer de résultat dès qu'un fichier apparaît, disparaît ou change,
// exactement ce qu'on veut détecter.
func hashGlobs(motifs ...string) (string, error) {
	var fichiers []string
	for _, motif := range motifs {
		trouves, err := filepath.Glob(motif)
		if err != nil {
			return "", err
		}
		fichiers = append(fichiers, trouves...)
	}
	sort.Strings(fichiers)
	return hashFichiers(fichiers)
}

// empreinteIngestion : la date de la dernière exécution de cmd/ingest
// terminée avec succès, tables source par tables source, section par
// section. Sert de signal de fraîcheur des données pour « reste » — pas un
// hachage de tables lues (la liste serait celle de tout core/ref/geo,
// intenable), mais une question plus simple et tout aussi sûre : cmd/ingest
// a-t-il tourné à bien depuis la dernière construction ? Un ingest qui
// n'aurait touché aucune des tables de « reste » ferait reconstruire pour
// rien — jamais servir une page périmée, qui serait le sens dangereux.
func empreinteIngestion(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	var derniere *time.Time
	if err := pool.QueryRow(ctx,
		`SELECT max(finished_at) FROM raw.fetch_run WHERE status = 'SUCCESS'`).
		Scan(&derniere); err != nil {
		return "", err
	}
	if derniere == nil {
		return "jamais", nil
	}
	return derniere.UTC().Format(time.RFC3339Nano), nil
}

// resteInchange : rien de ce que communes/scrutin ne couvrent pas n'a
// changé — ni le code, ni les gabarits, ni les CSV éditoriaux (data/), ni les
// dossiers documentaires (docs/*.md, voir loadDocs), ni les données
// core/ref/geo depuis la dernière ingestion réussie.
func resteInchange(ctx context.Context, pool *pgxpool.Pool, ancien manifesteCache) (bool, etatSection, error) {
	var actuel etatSection
	var err error
	actuel.DataHash, err = empreinteIngestion(ctx, pool)
	if err != nil {
		return false, actuel, err
	}
	actuel.CodeHash, err = hashGlobs("internal/sitegen/*.go", "web/templates/*.gohtml", "web/assets/*", "docs/*.md", "data/*.csv")
	if err != nil {
		return false, actuel, err
	}
	prec, ok := ancien.Sections["reste"]
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
