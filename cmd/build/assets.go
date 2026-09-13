package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// Ressources partagées : une seule feuille de style et un seul script pour tout
// le site, au lieu d'être recopiés dans chaque page.
//
// La raison est mesurée, pas théorique : 35,4 Ko de CSS et de JS répétés sur
// 40 263 pages faisaient 1,52 Go — quarante pour cent du poids du site — et
// aucun de ces octets n'était mis en cache d'une page à l'autre. Extraits, ils
// sont téléchargés une fois et servis depuis le cache pour toutes les suivantes.
//
// Le nom porte l'empreinte du contenu : une modification change le nom, donc le
// cache ne peut pas servir une version périmée, et le fichier peut être mis en
// cache indéfiniment.
type Assets struct{ CSS, JS string }

func copierAssets(src, out string) (Assets, error) {
	var a Assets
	for _, f := range []struct {
		nom  string
		dest *string
	}{{"style.css", &a.CSS}, {"site.js", &a.JS}} {
		b, err := os.ReadFile(filepath.Join(src, f.nom))
		if err != nil {
			return a, err
		}
		sum := sha256.Sum256(b)
		ext := filepath.Ext(f.nom)
		nom := fmt.Sprintf("%s.%s%s", f.nom[:len(f.nom)-len(ext)],
			hex.EncodeToString(sum[:])[:10], ext)
		if err := os.WriteFile(filepath.Join(out, nom), b, 0o644); err != nil {
			return a, err
		}
		*f.dest = nom
	}
	return a, nil
}
