package media

import (
	"encoding/json"
	"os"
)

// Le cache évite de refaire, à CHAQUE ingestion, les deux appels Wikimédia
// (rate-limités à 1,2 s, jusqu'à 4 essais chacun) pour une cible dont la
// résolution n'a aucune raison d'avoir changé depuis la dernière fois :
// data/candidats.csv et data/organisations.csv ne bougent qu'à la décision
// éditoriale d'un humain, jamais au fil d'un ingest automatique — contrairement
// à presque tout le reste de ce dépôt, où la source change en continu et où
// une resélection systématique est le comportement correct. La licence et
// l'auteur d'un fichier Commons ne changent pour ainsi dire jamais après coup
// non plus : ce que la dernière résolution réussie a vu reste vrai.
//
// Fichier JSON commité (voir docs/perimetre.md sur le choix de committer
// web/media/*), relu/réécrit à chaque Ingest — jamais la source de vérité
// (Wikimédia l'est), seulement ce qui évite de la réinterroger pour rien.
// Une cible absente du cache, ou dont le fichier local a disparu de mediaDir,
// est toujours résolue en direct : le cache ne fait QUE sauter un appel
// réseau déjà fait avec succès, il ne invente jamais un résultat.
type entreeCache struct {
	// Positif : un fichier a été retenu.
	Nom, SourceURL, Licence, LicenceCode, Auteur string
	URL                                          string
	Largeur, Hauteur                             int
	Local                                        string
	// Négatif : écarté faute de licence libre (ou aucune image). Mis en
	// cache aussi — reposer la même question à Wikimédia à chaque run pour
	// une réponse qui ne change pour ainsi dire jamais coûterait le même
	// aller-retour rate-limité qu'un cas positif, pour rien.
	Rejete bool
	Raison string
}

type cache struct {
	// Entries, exporté : encoding/json a besoin d'un champ exporté pour
	// sérialiser ; le reste du paquet continue de passer par cache, jamais
	// directement par ce champ.
	Entries map[string]entreeCache
}

func cleCache(pageFR, kind string) string { return kind + "|" + pageFR }

// chargerCache : un cache absent (premier run, ou fichier jamais créé) n'est
// pas une erreur — juste un cache vide, qui résout tout en direct une
// première fois.
func chargerCache(path string) (cache, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cache{Entries: map[string]entreeCache{}}, nil
	}
	if err != nil {
		return cache{}, err
	}
	var c cache
	if err := json.Unmarshal(b, &c); err != nil {
		return cache{}, err
	}
	if c.Entries == nil {
		c.Entries = map[string]entreeCache{}
	}
	return c, nil
}

// sauvegarder : encoding/json trie déjà les clés d'une map par ordre
// alphabétique — un diff git lisible quand une seule cible change, plutôt
// qu'un fichier réécrit dans un ordre non déterministe à chaque run, sans
// qu'il soit besoin de le refaire à la main ici.
func (c cache) sauvegarder(path string) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o644)
}
